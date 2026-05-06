package app

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/usewhale/whale/internal/core"
	"github.com/usewhale/whale/internal/policy"
	"github.com/usewhale/whale/internal/session"
)

func (a *App) HandleSlash(line string) (handled bool, output string, synthetic string, shouldExit bool, clearScreen bool, err error) {
	cmdResult, cmdErr := handleCommand(line, a.sessionID, time.Now())
	if cmdErr != nil {
		return true, "", "", false, false, cmdErr
	}
	if !cmdResult.Handled {
		return false, "", "", false, false, nil
	}
	if cmdResult.ClearScreen {
		return true, "▸ terminal cleared — context is intact — use /new to start fresh", "", false, true, nil
	}
	if cmdResult.ShowStatus {
		return true, a.buildStatus(), "", false, false, nil
	}
	if cmdResult.ShowContext {
		ctxLine, err := a.buildContext()
		return true, ctxLine, "", false, false, err
	}
	if cmdResult.InitMemory {
		line, err := a.initMemory()
		if err != nil || line != "" {
			return true, line, "", false, false, err
		}
		return true, "Initializing AGENTS.md from repository context...", buildInitSyntheticPrompt(), false, false, nil
	}
	if cmdResult.ShowMemory {
		line := a.showMemory()
		return true, line, "", false, false, nil
	}
	if cmdResult.Mode != "" {
		mode, err := session.ParseMode(cmdResult.Mode)
		if err != nil {
			return true, "", "", false, false, err
		}
		msg, err := a.SetMode(mode)
		if err != nil {
			return true, "", "", false, false, err
		}
		if cmdResult.Output == "" {
			cmdResult.Output = msg
		}
	}
	if cmdResult.Output != "" {
		output = cmdResult.Output
	}
	if cmdResult.PlanPrompt != "" {
		synthetic = cmdResult.PlanPrompt
	}
	// For /new: capture old session info before switching.
	oldID := a.sessionID
	oldMsgCount := 0
	if strings.HasPrefix(strings.TrimSpace(line), "/new") {
		if msgs, err := a.msgStore.List(a.ctx, a.sessionID); err == nil {
			oldMsgCount = len(msgs)
		}
	}
	a.sessionID = cmdResult.SessionID
	if strings.HasPrefix(strings.TrimSpace(line), "/new") {
		modeState, err := session.LoadModeState(a.sessionsDir, a.sessionID)
		if err != nil {
			return true, "", "", false, false, err
		}
		a.currentMode = modeState.Mode
		a.a = nil
		output = fmt.Sprintf("new session: %s", cmdResult.SessionID)
		if oldMsgCount > 0 {
			output += fmt.Sprintf("\n▸ dropped %d message(s) from session %s", oldMsgCount, oldID)
		} else {
			output += fmt.Sprintf("\n▸ previous session: %s", oldID)
		}
		output += fmt.Sprintf("\n▸ to resume the previous session, run: whale resume %s", oldID)
		output += fmt.Sprintf("\nmode: %s", a.currentMode)
		if _, err := session.PatchSessionMeta(a.sessionsDir, a.sessionID, session.SessionMeta{Workspace: a.workspaceRoot, Branch: a.branch}); err != nil {
			return true, "", "", false, false, err
		}
	}
	return true, output, synthetic, cmdResult.ShouldExit, false, nil
}

func (a *App) HandleLocalCommand(line string) (handled bool, output string, err error) {
	if strings.TrimSpace(line) == "/tools" {
		specs := a.toolRegistry.Specs()
		if len(specs) == 0 {
			return true, "no tools registered", nil
		}
		parts := make([]string, 0, len(specs)*2)
		for i, s := range specs {
			ro := "write"
			if s.ReadOnly {
				ro = "read-only"
			}
			parts = append(parts, fmt.Sprintf("%d. %s [%s]", i+1, s.Name, ro))
			if s.Description != "" {
				parts = append(parts, "   "+s.Description)
			}
		}
		return true, strings.Join(parts, "\n"), nil
	}
	if strings.HasPrefix(line, "/tool") {
		n := 5
		fields := strings.Fields(line)
		if len(fields) == 2 {
			v, err := strconv.Atoi(fields[1])
			if err != nil || v < 1 {
				return true, "", errors.New("usage: /tool [N]")
			}
			n = v
		}
		msgs, err := a.msgStore.List(a.ctx, a.sessionID)
		if err != nil {
			return true, "", err
		}
		hits := make([]core.ToolResult, 0, n)
		for i := len(msgs) - 1; i >= 0 && len(hits) < n; i-- {
			if msgs[i].Role != core.RoleTool {
				continue
			}
			for j := len(msgs[i].ToolResults) - 1; j >= 0 && len(hits) < n; j-- {
				hits = append(hits, msgs[i].ToolResults[j])
			}
		}
		if len(hits) == 0 {
			return true, "no tool results in this session", nil
		}
		rows := make([]string, 0, len(hits)*2)
		for i, r := range hits {
			status := "ok"
			if r.IsError {
				status = "error"
			}
			content := r.Content
			if len(content) > 240 {
				content = content[:240] + "..."
			}
			rows = append(rows, fmt.Sprintf("#%d %s [%s]\n%s", i+1, r.Name, status, content))
		}
		return true, strings.Join(rows, "\n"), nil
	}
	if strings.HasPrefix(line, "/compact") {
		fields := strings.Fields(line)
		if len(fields) != 1 || fields[0] != "/compact" {
			return true, "", errors.New("usage: /compact")
		}
		ag, err := a.ensureAgent()
		if err != nil {
			return true, "", err
		}
		info, err := ag.CompactSession(a.ctx, a.sessionID)
		if err != nil {
			return true, "", err
		}
		a.a = nil
		if !info.Compacted {
			return true, "nothing to compact", nil
		}
		return true, fmt.Sprintf("compacted conversation: %d -> %d messages; ~%d -> ~%d tokens", info.MessagesBefore, info.MessagesAfter, info.BeforeEstimate, info.AfterEstimate), nil
	}
	if strings.HasPrefix(line, "/budget") {
		fields := strings.Fields(line)
		if len(fields) == 1 || (len(fields) == 2 && strings.EqualFold(fields[1], "show")) {
			if a.budgetWarningUSD > 0 {
				return true, fmt.Sprintf("budget warning cap: $%.4f", a.budgetWarningUSD), nil
			}
			return true, "budget warning cap: disabled", nil
		}
		if len(fields) != 2 {
			return true, "", errors.New("usage: /budget [off|show|USD]")
		}
		arg := strings.TrimSpace(fields[1])
		if strings.EqualFold(arg, "off") {
			a.budgetWarningUSD = 0
			a.a = nil
			return true, "budget warning disabled", nil
		}
		v, err := strconv.ParseFloat(arg, 64)
		if err != nil || v <= 0 {
			return true, "", errors.New("usage: /budget [off|show|USD]")
		}
		a.budgetWarningUSD = v
		a.a = nil
		return true, fmt.Sprintf("budget warning cap set: $%.4f", a.budgetWarningUSD), nil
	}
	if strings.HasPrefix(line, "/approval ") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return true, "", errors.New("usage: /approval <on-request|never-ask>")
		}
		mode, err := policy.ParseApprovalMode(fields[1])
		if err != nil {
			return true, "", err
		}
		a.SetApprovalMode(mode)
		return true, fmt.Sprintf("approval mode set: %s", approvalModeDisplay(a.approvalMode)), nil
	}
	if strings.HasPrefix(line, "/thinking") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return true, "", errors.New("usage: /thinking [on|off]")
		}
		switch strings.ToLower(strings.TrimSpace(fields[1])) {
		case "on":
			a.SetThinkingEnabled(true)
			return true, "thinking: on", nil
		case "off":
			a.SetThinkingEnabled(false)
			return true, "thinking: off", nil
		default:
			return true, "", errors.New("usage: /thinking [on|off]")
		}
	}
	if strings.HasPrefix(line, "/key ") {
		a.apiKey = strings.TrimSpace(strings.TrimPrefix(line, "/key "))
		if a.apiKey == "" {
			return true, "", errors.New("empty key")
		}
		a.a = nil
		return true, "api key set for current session", nil
	}
	if strings.HasPrefix(line, "sk-") && a.apiKey == "" {
		a.apiKey = line
		a.a = nil
		return true, "api key set for current session", nil
	}
	return false, "", nil
}
