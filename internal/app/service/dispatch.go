package service

import (
	"fmt"
	"strings"

	"github.com/usewhale/whale/internal/app"
	appcommands "github.com/usewhale/whale/internal/app/commands"
	"github.com/usewhale/whale/internal/core"
	"github.com/usewhale/whale/internal/policy"
	"github.com/usewhale/whale/internal/session"
)

func (s *Service) Dispatch(in Intent) {
	switch in.Kind {
	case IntentSubmit:
		go s.handleSubmit(in.Input, in.HiddenInput, in.SkillBinding)
	case IntentAllowTool:
		s.resolveApproval(in.ToolCallID, policy.ApprovalAllow)
	case IntentAllowToolForSession:
		s.resolveApproval(in.ToolCallID, policy.ApprovalAllowForSession)
	case IntentDenyTool:
		s.resolveApproval(in.ToolCallID, policy.ApprovalDeny)
	case IntentSubmitUserInput:
		if in.UserInput != nil {
			s.resolveUserInput(in.ToolCallID, *in.UserInput, true)
		}
	case IntentCancelUserInput:
		s.resolveUserInput(in.ToolCallID, core.UserInputResponse{}, false)
	case IntentRequestSessions:
		s.emitSessionChoices()
	case IntentSelectSession:
		msg, err := s.app.ApplyResumeChoice(in.SessionInput)
		if err != nil {
			s.emit(Event{Kind: EventError, Text: err.Error()})
			return
		}
		s.emit(Event{Kind: EventInfo, Text: msg})
		s.emitSessionHydrated()
	case IntentShutdown:
		s.cancelMu.Lock()
		if s.cancel != nil {
			s.cancel()
		}
		s.cancelMu.Unlock()
	case IntentSetModelAndEffort:
		if err := s.app.SetModelAndEffort(in.Model, in.Effort); err != nil {
			s.emit(Event{Kind: EventError, Text: err.Error()})
			return
		}
		if strings.EqualFold(strings.TrimSpace(in.Thinking), "on") {
			s.app.SetThinkingEnabled(true)
		}
		if strings.EqualFold(strings.TrimSpace(in.Thinking), "off") {
			s.app.SetThinkingEnabled(false)
		}
		s.emit(Event{Kind: EventInfo, Text: fmt.Sprintf("model set: %s  effort: %s  thinking: %s", s.app.Model(), s.app.ReasoningEffort(), onOff(s.app.ThinkingEnabled()))})
		s.emit(Event{Kind: EventTurnDone})
	case IntentSetApprovalMode:
		mode, err := policy.ParseApprovalMode(in.ApprovalMode)
		if err != nil {
			s.emit(Event{Kind: EventError, Text: err.Error()})
			return
		}
		s.app.SetApprovalMode(mode)
		s.emit(Event{Kind: EventInfo, Text: fmt.Sprintf("approval set: %s", approvalModeDisplay(s.app.ApprovalMode()))})
		s.emit(Event{Kind: EventTurnDone})
	case IntentToggleMode:
		msg, err := s.app.ToggleMode()
		if err != nil {
			s.emit(Event{Kind: EventError, Text: err.Error()})
			return
		}
		s.emit(Event{Kind: EventInfo, Text: msg})
		s.emit(Event{Kind: EventTurnDone, LastResponse: msg})
	case IntentImplementPlan:
		out, err := s.app.SetMode(session.ModeAgent)
		if err != nil {
			s.emit(Event{Kind: EventError, Text: err.Error()})
			s.emit(Event{Kind: EventTurnDone})
			return
		}
		s.emit(Event{Kind: EventInfo, Text: out})
		go s.runTurn("Implement the plan.", false)
	case IntentRequestSkillsManage:
		s.emit(Event{Kind: EventSkillsManager, Skills: s.SkillsForManager()})
	case IntentSetSkillEnabled:
		if _, err := s.app.SetSkillEnabled(in.SkillName, in.SkillEnabled); err != nil {
			s.emit(Event{Kind: EventError, Text: err.Error()})
			return
		}
		s.emit(Event{Kind: EventSkillsManager, Skills: s.SkillsForManager()})
	}
}

func (s *Service) handleSubmit(line string, hiddenInput bool, skillBinding *app.SkillBinding) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	line = appcommands.ExpandUniqueSlashPrefix(line, app.CommandsHelp, "/mcp")
	prevSessionID := s.app.SessionID()
	if line == "/model" {
		s.emit(Event{
			Kind:            EventModelPicker,
			ModelChoices:    s.app.SupportedModels(),
			EffortChoices:   s.app.SupportedEfforts(),
			CurrentModel:    s.app.Model(),
			CurrentEffort:   s.app.ReasoningEffort(),
			ThinkingChoices: []string{"on", "off"},
			CurrentThinking: onOff(s.app.ThinkingEnabled()),
		})
		return
	}
	if line == "/permissions" {
		s.emit(Event{
			Kind:            EventPermissionsPicker,
			ApprovalChoices: []string{"Ask first", "Auto approve"},
			CurrentApproval: approvalModeDisplay(s.app.ApprovalMode()),
		})
		return
	}
	if line == "/skills" {
		s.emit(Event{Kind: EventSkillsMenu})
		return
	}
	if prompt, ok := appcommands.PlanPromptFromSlash(line); ok {
		out, err := s.app.SetMode(session.ModePlan)
		if err != nil {
			s.emit(Event{Kind: EventError, Text: err.Error()})
			s.emit(Event{Kind: EventTurnDone})
			return
		}
		s.emit(Event{Kind: EventInfo, Text: out})
		line = prompt
		hiddenInput = false
	}
	if prompt, ok := appcommands.AskPromptFromSlash(line); ok {
		out, err := s.app.SetMode(session.ModeAsk)
		if err != nil {
			s.emit(Event{Kind: EventError, Text: err.Error()})
			s.emit(Event{Kind: EventTurnDone})
			return
		}
		s.emit(Event{Kind: EventInfo, Text: out})
		line = prompt
		hiddenInput = false
	}
	if s.app.IsResumeMenu(line) {
		s.emitSessionChoices()
		s.emit(Event{Kind: EventTurnDone})
		return
	}
	if strings.HasPrefix(line, "/model ") {
		s.emit(Event{Kind: EventError, Text: "usage: /model"})
		s.emit(Event{Kind: EventTurnDone})
		return
	}
	handled, out, synthetic, shouldExit, clearScreen, err := s.app.HandleSlash(line)
	if err != nil {
		s.emit(Event{Kind: EventError, Text: err.Error()})
		s.emit(Event{Kind: EventTurnDone})
		return
	}
	if handled {
		if clearScreen {
			s.emit(Event{Kind: EventClearScreen})
		}
		if shouldExit {
			s.emit(Event{Kind: EventExitRequested})
		}
		if s.app.SessionID() != prevSessionID {
			s.emitSessionHydrated()
		}
		// Emit Info after session hydration so the text isn't
		// wiped by the hydration's assembler reset.
		if out != "" {
			s.emit(Event{Kind: EventInfo, Text: out})
		}
		if synthetic == "" {
			s.emit(Event{Kind: EventTurnDone, LastResponse: out})
			return
		}
		line = synthetic
		hiddenInput = true
	}
	handled, out, err = s.app.HandleLocalCommand(line)
	if err != nil {
		s.emit(Event{Kind: EventError, Text: err.Error()})
		s.emit(Event{Kind: EventTurnDone})
		return
	}
	if handled {
		if out != "" {
			s.emit(Event{Kind: EventInfo, Text: out})
		}
		s.emit(Event{Kind: EventTurnDone, LastResponse: out})
		return
	}
	if appcommands.LooksLikeSlashCommand(line) {
		s.emit(Event{Kind: EventError, Text: fmt.Sprintf("• Unrecognized command %q. Type \"/\" for a list of supported commands.", line)})
		s.emit(Event{Kind: EventTurnDone})
		return
	}
	skillMention, skillOut, skillSynthetic, err := s.app.BuildSkillMentionSyntheticPromptWithBinding(line, skillBinding)
	if err != nil {
		s.emit(Event{Kind: EventError, Text: err.Error()})
		s.emit(Event{Kind: EventTurnDone})
		return
	}
	if blocked, out := s.app.RunUserPromptSubmitHook(line); out != "" {
		s.emit(Event{Kind: EventInfo, Text: out})
		if blocked {
			s.emit(Event{Kind: EventTurnDone, LastResponse: out})
			return
		}
	}
	if skillMention {
		if skillOut != "" {
			s.emit(Event{Kind: EventSkillLoaded, Text: skillOut})
		}
		go s.runInjectedTurn(line, skillSynthetic)
		return
	}
	go s.runTurn(line, hiddenInput)
}

func (s *Service) emitSessionHydrated() {
	msgs, err := s.app.ListMessages()
	if err != nil {
		s.emit(Event{Kind: EventError, Text: err.Error()})
		return
	}
	s.emit(Event{Kind: EventSessionHydrated, SessionID: s.app.SessionID(), Messages: msgs})
}

func (s *Service) emitSessionChoices() bool {
	choices, err := s.app.ListResumeChoices(20)
	if err != nil {
		s.emit(Event{Kind: EventError, Text: err.Error()})
		return false
	}
	if len(choices) == 0 {
		s.emit(Event{Kind: EventInfo, Text: "no saved sessions"})
		return false
	}
	s.emit(Event{Kind: EventSessionsListed, Choices: choices})
	return true
}
