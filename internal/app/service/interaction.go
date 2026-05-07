package service

import (
	"github.com/usewhale/whale/internal/agent"
	"github.com/usewhale/whale/internal/core"
	"github.com/usewhale/whale/internal/policy"
)

func (s *Service) awaitApproval(req policy.ApprovalRequest) bool {
	toolCallID := req.ToolCall.ID
	s.approveMu.Lock()
	if s.sessionGrantLocked(req.SessionID, req.Key) {
		s.approveMu.Unlock()
		return true
	}
	ch := make(chan approvalDecision, 1)
	s.approvals[toolCallID] = ch
	s.approveMu.Unlock()
	s.emit(Event{Kind: EventApprovalRequired, ToolCallID: toolCallID, ToolName: req.ToolCall.Name, Text: policy.ApprovalSummary(req.ToolCall), Metadata: req.Metadata})
	decision := <-ch
	s.approveMu.Lock()
	delete(s.approvals, toolCallID)
	if decision == approvalAllowSession {
		s.grantSessionLocked(req.SessionID, req.Key)
	}
	s.approveMu.Unlock()
	return decision != approvalDeny
}

func (s *Service) resolveApproval(toolCallID string, decision approvalDecision) {
	s.approveMu.Lock()
	ch, ok := s.approvals[toolCallID]
	s.approveMu.Unlock()
	if !ok {
		s.emit(Event{Kind: EventError, Text: "no pending approval for tool call"})
		return
	}
	ch <- decision
}

func (s *Service) sessionGrantLocked(sessionID, key string) bool {
	bySession, ok := s.sessionGrants[sessionID]
	if !ok {
		return false
	}
	return bySession[key]
}

func (s *Service) grantSessionLocked(sessionID, key string) {
	bySession, ok := s.sessionGrants[sessionID]
	if !ok {
		bySession = map[string]bool{}
		s.sessionGrants[sessionID] = bySession
	}
	bySession[key] = true
}

func (s *Service) awaitUserInput(req agent.UserInputRequest) (core.UserInputResponse, bool) {
	toolCallID := req.ToolCall.ID
	ch := make(chan userInputDecision, 1)
	s.inputMu.Lock()
	s.inputs[toolCallID] = ch
	s.inputMu.Unlock()
	s.emit(Event{Kind: EventUserInputRequired, ToolCallID: toolCallID, ToolName: req.ToolCall.Name, Questions: req.Questions})
	decision := <-ch
	s.inputMu.Lock()
	delete(s.inputs, toolCallID)
	s.inputMu.Unlock()
	return decision.response, decision.ok
}

func (s *Service) resolveUserInput(toolCallID string, resp core.UserInputResponse, ok bool) {
	s.inputMu.Lock()
	ch, exists := s.inputs[toolCallID]
	s.inputMu.Unlock()
	if !exists {
		s.emit(Event{Kind: EventError, Text: "no pending user input"})
		return
	}
	ch <- userInputDecision{response: resp, ok: ok}
}
