package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/usewhale/whale/internal/agent"
	"github.com/usewhale/whale/internal/app"
)

func Run(cfg app.Config, start app.StartOptions) error {
	ctx := context.Background()
	start.ApprovalFunc = promptApprovalCLI
	start.UserInputFunc = promptUserInputCLI
	coreApp, err := app.New(ctx, cfg, start)
	if err != nil {
		return err
	}
	defer coreApp.Close()
	coreApp.InitializeMCP(ctx, nil)
	for _, line := range coreApp.StartupLines() {
		fmt.Println(line)
	}

	if start.ResumeMenu {
		if err := promptResumeChoice(coreApp); err != nil {
			return err
		}
	}
	turn := 0
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	var turnCancelMu sync.Mutex
	var turnCancel context.CancelFunc
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("> ")
		raw, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		raw = strings.TrimRight(raw, "\r\n")
		if raw == "" {
			continue
		}
		// On Windows, pasted multi-line text may arrive as a single stdin block.
		// Read all buffered lines and join them as one prompt instead of
		// treating each line as a separate turn.
		lines := []string{raw}
		for reader.Buffered() > 0 {
			nextRaw, err := reader.ReadString('\n')
			if err != nil {
				break
			}
			lines = append(lines, strings.TrimRight(nextRaw, "\r\n"))
		}
		line := strings.Join(lines, "\n")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if coreApp.IsResumeMenu(line) {
			choices, err := coreApp.ListResumeChoices(20)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				continue
			}
			if len(choices) == 0 {
				fmt.Println("no saved sessions")
				continue
			}
			for _, c := range choices {
				fmt.Println(c)
			}
			{
				fmt.Print("resume> choose number or session id (blank to cancel): ")
				resumeRaw, resumeErr := reader.ReadString('\n')
				if resumeErr != nil {
					break
				}
				resumeRaw = strings.TrimSpace(resumeRaw)
				if resumeRaw == "" {
					continue
				}
				msg, err := coreApp.ApplyResumeChoice(resumeRaw)
				if err != nil {
					fmt.Fprintf(os.Stderr, "error: %v\n", err)
					continue
				}
				fmt.Println(msg)
			}
			continue
		}

		handled, out, synthetic, shouldExit, _, err := coreApp.HandleSlash(line)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			continue
		}
		if handled {
			if out != "" {
				fmt.Println(out)
			}
			if shouldExit {
				break
			}
			if synthetic == "" {
				continue
			}
			line = synthetic
		}
		handled, out, err = coreApp.HandleLocalCommand(line)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			continue
		}
		if handled {
			if out != "" {
				fmt.Println(out)
			}
			continue
		}
		if strings.HasPrefix(line, "/") {
			fmt.Fprintf(os.Stderr, "error: unknown command\n")
			continue
		}
		if hookOutBlocked, hookOut := coreApp.RunUserPromptSubmitHook(line); hookOut != "" {
			fmt.Println(hookOut)
			if hookOutBlocked {
				continue
			}
		}

		turnCtx, cancelTurn := context.WithCancel(ctx)
		turnCancelMu.Lock()
		turnCancel = cancelTurn
		turnCancelMu.Unlock()
		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				select {
				case <-turnCtx.Done():
					return
				case <-sigCh:
					turnCancelMu.Lock()
					if turnCancel != nil {
						turnCancel()
					}
					turnCancelMu.Unlock()
				}
			}
		}()
		events, runErr := coreApp.RunTurn(turnCtx, line, false)
		if runErr != nil {
			cancelTurn()
			turnCancelMu.Lock()
			turnCancel = nil
			turnCancelMu.Unlock()
			<-done
			fmt.Fprintf(os.Stderr, "error: %v\n", runErr)
			continue
		}

		fmt.Print("assistant> ")
		printedText := false
		lastAssistantText := ""
		for ev := range events {
			renderEvent(ev, &printedText, &lastAssistantText)
		}
		cancelTurn()
		turnCancelMu.Lock()
		turnCancel = nil
		turnCancelMu.Unlock()
		<-done
		turn++
		if err := coreApp.FinalizeTurn(lastAssistantText); err != nil {
			fmt.Fprintf(os.Stderr, "patch session meta failed: %v\n", err)
		}
		if hookOut := coreApp.RunStopHook(lastAssistantText, turn); hookOut != "" {
			fmt.Println(hookOut)
		}
	}
	return nil
}

func promptResumeChoice(app *app.App) error {
	scanner := bufio.NewScanner(os.Stdin)
	choices, err := app.ListResumeChoices(20)
	if err != nil {
		return err
	}
	if len(choices) == 0 {
		fmt.Println("no saved sessions")
		return nil
	}
	for _, c := range choices {
		fmt.Println(c)
	}
	fmt.Print("resume> choose number or session id (blank to cancel): ")
	if !scanner.Scan() {
		return scanner.Err()
	}
	msg, err := app.ApplyResumeChoice(scanner.Text())
	if err != nil {
		return err
	}
	fmt.Println(msg)
	return nil
}

func renderEvent(ev agent.AgentEvent, printedText *bool, lastAssistantText *string) {
	switch ev.Type {
	case agent.AgentEventTypeAssistantDelta:
		if ev.Content != "" {
			*printedText = true
		}
		fmt.Print(ev.Content)
	case agent.AgentEventTypeReasoningDelta:
		if ev.ReasoningDelta != "" {
			fmt.Printf("\n[think] %s\n", ev.ReasoningDelta)
		}
	case agent.AgentEventTypeToolArgsDelta:
		if ev.ToolArgs != nil {
			fmt.Printf("\n[tool-args] %s#%d %d chars ready:%d\n", ev.ToolArgs.ToolName, ev.ToolArgs.ToolCallIndex, ev.ToolArgs.ArgsChars, ev.ToolArgs.ReadyCount)
		}
	case agent.AgentEventTypeToolArgsRepaired:
		if ev.ToolArgsRepair != nil {
			fmt.Printf("\n[tool-repair] %s#%d repaired\n", ev.ToolArgsRepair.ToolName, ev.ToolArgsRepair.ToolCallIndex)
		}
	case agent.AgentEventTypeToolCallBlocked, agent.AgentEventTypeToolModeBlocked:
		if ev.ToolBlocked != nil {
			tag := "tool-blocked"
			if ev.Type == agent.AgentEventTypeToolModeBlocked {
				tag = "tool-mode-blocked"
			}
			fmt.Printf("\n[%s] %s(%s) code:%s\n", tag, ev.ToolBlocked.ToolName, ev.ToolBlocked.ToolCallID, ev.ToolBlocked.ReasonCode)
		}
	case agent.AgentEventTypeToolApprovalRequired:
		if ev.Approval != nil {
			fmt.Printf("\n[tool-approval-required] %s(%s) code:%s scope:%s\n", ev.Approval.ToolName, ev.Approval.ToolCallID, ev.Approval.Code, ev.Approval.Scope)
			if ev.Approval.Summary != "" {
				fmt.Printf("[tool-approval-summary] %s\n", ev.Approval.Summary)
			}
		}
	case agent.AgentEventTypeToolCallScavenged:
		if ev.Scavenged != nil {
			fmt.Printf("\n[tool-scavenge] recovered:%d\n", ev.Scavenged.Count)
		}
	case agent.AgentEventTypeToolPolicyDecision:
		if ev.Policy != nil {
			fmt.Printf("\n[tool-policy] %s(%s) phase:%s code:%s allow:%v need_approval:%v rule:%s\n", ev.Policy.ToolName, ev.Policy.ToolCallID, ev.Policy.Phase, ev.Policy.Code, ev.Policy.Allow, ev.Policy.NeedsApproval, ev.Policy.MatchedRule)
		}
	case agent.AgentEventTypeToolCall:
		if ev.ToolCall != nil {
			fmt.Printf("\n[tool call] %s(%s)\n", ev.ToolCall.Name, ev.ToolCall.ID)
		}
	case agent.AgentEventTypeUserInputRequired:
		if ev.ToolCall != nil && ev.UserInputReq != nil {
			fmt.Printf("\n[user-input-required] %s(%s) questions:%d\n", ev.ToolCall.Name, ev.ToolCall.ID, len(ev.UserInputReq.Questions))
		}
	case agent.AgentEventTypeUserInputSubmitted:
		if ev.ToolCall != nil && ev.UserInputResp != nil {
			fmt.Printf("\n[user-input-submitted] %s(%s) answers:%d\n", ev.ToolCall.Name, ev.ToolCall.ID, len(ev.UserInputResp.Answers))
		}
	case agent.AgentEventTypeUserInputCancelled:
		if ev.ToolCall != nil {
			fmt.Printf("\n[user-input-cancelled] %s(%s)\n", ev.ToolCall.Name, ev.ToolCall.ID)
		}
	case agent.AgentEventTypeToolResult:
		if ev.Result != nil {
			fmt.Printf("[tool result] %s\n", ev.Result.Content)
		}
	case agent.AgentEventTypeDone:
		if !*printedText && ev.Message != nil && ev.Message.Text != "" {
			fmt.Print(ev.Message.Text)
		}
		if ev.Message != nil {
			*lastAssistantText = ev.Message.Text
		}
		fmt.Print("\n")
	case agent.AgentEventTypeError:
		if ev.Err != nil {
			fmt.Fprintf(os.Stderr, "\nerror: %v\n", ev.Err)
		}
	default:
		if ev.Content != "" {
			fmt.Printf("\n[%s] %s\n", ev.Type, strings.TrimSpace(ev.Content))
		}
	}
}
