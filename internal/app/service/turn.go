package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/usewhale/whale/internal/agent"
	"github.com/usewhale/whale/internal/core"
)

func (s *Service) runTurn(line string, hiddenInput bool) {
	turnCtx, cancel := context.WithCancel(s.ctx)
	s.cancelMu.Lock()
	if s.active {
		s.cancelMu.Unlock()
		cancel()
		s.emit(Event{Kind: EventError, Text: agent.ErrSessionBusy.Error()})
		s.emit(Event{Kind: EventTurnDone})
		return
	}
	s.active = true
	s.cancel = cancel
	s.cancelMu.Unlock()
	defer func() {
		s.cancelMu.Lock()
		s.cancel = nil
		s.active = false
		s.cancelMu.Unlock()
		cancel()
	}()
	events, err := s.app.RunTurn(turnCtx, line, hiddenInput)
	if err != nil {
		if shouldSuppressCancelledTurnError(turnCtx, err) {
			s.emit(Event{Kind: EventTurnDone})
			return
		}
		s.emit(Event{Kind: EventError, Text: err.Error()})
		s.emit(Event{Kind: EventTurnDone})
		return
	}
	last := ""
	deltas := newTurnDeltaCoalescers(s)
	for ev := range events {
		switch ev.Type {
		case agent.AgentEventTypeAssistantDelta:
			if ev.Content != "" {
				last += ev.Content
				deltas.add(EventAssistantDelta, ev.Content)
			}
		case agent.AgentEventTypeReasoningDelta:
			deltas.add(EventReasoningDelta, ev.ReasoningDelta)
		case agent.AgentEventTypePlanDelta:
			if ev.Content != "" {
				deltas.add(EventPlanDelta, ev.Content)
			}
		case agent.AgentEventTypePlanCompleted:
			deltas.flushReliable()
			s.emit(Event{Kind: EventPlanCompleted, Text: ev.Content})
		case agent.AgentEventTypeToolCall:
			if ev.ToolCall != nil {
				deltas.flushReliable()
				s.emit(Event{
					Kind:       EventToolCall,
					ToolCallID: ev.ToolCall.ID,
					ToolName:   ev.ToolCall.Name,
					Text:       summarizeToolCall(*ev.ToolCall),
				})
			}
		case agent.AgentEventTypeToolResult:
			if ev.Result != nil {
				deltas.flushReliable()
				s.emit(Event{Kind: EventToolResult, ToolCallID: ev.Result.ToolCallID, ToolName: ev.Result.Name, Text: ev.Result.Content})
			}
		case agent.AgentEventTypeUserInputRequired:
			if ev.ToolCall != nil && ev.UserInputReq != nil {
				deltas.flushReliable()
				s.emit(Event{Kind: EventUserInputRequired, ToolCallID: ev.ToolCall.ID, ToolName: ev.ToolCall.Name, Questions: ev.UserInputReq.Questions})
			}
		case agent.AgentEventTypeUserInputSubmitted, agent.AgentEventTypeUserInputCancelled:
			deltas.flushReliable()
			s.emit(Event{Kind: EventUserInputDone})
		case agent.AgentEventTypeError:
			if ev.Err != nil {
				if shouldSuppressCancelledTurnError(turnCtx, ev.Err) {
					continue
				}
				deltas.flushReliable()
				s.emit(Event{Kind: EventError, Text: ev.Err.Error()})
			}
		}
	}
	deltas.flushReliable()
	_ = s.app.FinalizeTurn(last)
	if out := s.app.RunStopHook(last, 0); out != "" {
		s.emit(Event{Kind: EventInfo, Text: out})
	}
	s.emit(Event{Kind: EventTurnDone, LastResponse: last})
}

func shouldSuppressCancelledTurnError(ctx context.Context, err error) bool {
	return ctx != nil && ctx.Err() != nil && errors.Is(err, context.Canceled)
}

func summarizeToolCall(call core.ToolCall) string {
	body := map[string]any{}
	_ = json.Unmarshal([]byte(call.Input), &body)
	name := strings.TrimSpace(call.Name)
	switch name {
	case "exec_shell":
		if cmd, _ := body["command"].(string); strings.TrimSpace(cmd) != "" {
			return fmt.Sprintf("exec_shell: %s", strings.TrimSpace(cmd))
		}
	case "exec_shell_wait":
		if taskID, _ := body["task_id"].(string); strings.TrimSpace(taskID) != "" {
			return fmt.Sprintf("exec_shell_wait: %s", strings.TrimSpace(taskID))
		}
	case "write", "edit":
		if path, _ := body["file_path"].(string); strings.TrimSpace(path) != "" {
			return fmt.Sprintf("%s: %s", name, strings.TrimSpace(path))
		}
	case "list_dir", "grep", "search_files":
		if path, _ := body["path"].(string); strings.TrimSpace(path) != "" {
			return fmt.Sprintf("%s: %s", name, strings.TrimSpace(path))
		}
	case "read_file":
		if path, _ := body["file_path"].(string); strings.TrimSpace(path) != "" {
			return fmt.Sprintf("%s: %s", name, strings.TrimSpace(path))
		}
	case "web_search":
		if q, _ := body["query"].(string); strings.TrimSpace(q) != "" {
			return fmt.Sprintf("web_search: %s", strings.TrimSpace(q))
		}
	case "fetch", "web_fetch":
		if u, _ := body["url"].(string); strings.TrimSpace(u) != "" {
			return fmt.Sprintf("%s: %s", name, strings.TrimSpace(u))
		}
	case "apply_patch":
		return "apply_patch: patch payload"
	case "request_user_input":
		if qs := body["questions"]; qs != nil {
			return fmt.Sprintf("request_user_input: %d question(s)", len(asAnySlice(qs)))
		}
	}
	if strings.TrimSpace(call.Input) != "" {
		return fmt.Sprintf("%s: %s", name, strings.TrimSpace(call.Input))
	}
	return name
}

func asAnySlice(v any) []any {
	if v == nil {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	return arr
}
