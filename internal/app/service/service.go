package service

import (
	"context"
	"sync"

	"github.com/usewhale/whale/internal/app"
	"github.com/usewhale/whale/internal/core"
	"github.com/usewhale/whale/internal/policy"
)

type IntentKind string

const (
	IntentSubmit              IntentKind = "submit"
	IntentAllowTool           IntentKind = "allow_tool"
	IntentAllowToolForSession IntentKind = "allow_tool_for_session"
	IntentDenyTool            IntentKind = "deny_tool"
	IntentSubmitUserInput     IntentKind = "submit_user_input"
	IntentCancelUserInput     IntentKind = "cancel_user_input"
	IntentSelectSession       IntentKind = "select_session"
	IntentRequestSessions     IntentKind = "request_sessions"
	IntentShutdown            IntentKind = "shutdown"
	IntentSetModelAndEffort   IntentKind = "set_model_and_effort"
	IntentSetApprovalMode     IntentKind = "set_approval_mode"
	IntentToggleMode          IntentKind = "toggle_mode"
	IntentImplementPlan       IntentKind = "implement_plan"
)

type Intent struct {
	Kind         IntentKind
	Input        string
	HiddenInput  bool
	ToolCallID   string
	UserInput    *core.UserInputResponse
	SessionInput string
	Model        string
	Effort       string
	Thinking     string
	ApprovalMode string
}

type EventKind string

const (
	EventInfo              EventKind = "info"
	EventError             EventKind = "error"
	EventAssistantDelta    EventKind = "assistant_delta"
	EventReasoningDelta    EventKind = "reasoning_delta"
	EventPlanDelta         EventKind = "plan_delta"
	EventPlanCompleted     EventKind = "plan_completed"
	EventToolCall          EventKind = "tool_call"
	EventToolResult        EventKind = "tool_result"
	EventTaskStarted       EventKind = "task_started"
	EventTaskProgress      EventKind = "task_progress"
	EventTaskCompleted     EventKind = "task_completed"
	EventMCPStatus         EventKind = "mcp_status"
	EventMCPComplete       EventKind = "mcp_complete"
	EventApprovalRequired  EventKind = "approval_required"
	EventUserInputRequired EventKind = "user_input_required"
	EventUserInputDone     EventKind = "user_input_done"
	EventSessionsListed    EventKind = "sessions_listed"
	EventTurnDone          EventKind = "turn_done"
	EventModelPicker       EventKind = "model_picker"
	EventPermissionsPicker EventKind = "permissions_picker"
	EventExitRequested     EventKind = "exit_requested"
	EventClearScreen       EventKind = "clear_screen"
	EventSessionHydrated   EventKind = "session_hydrated"
)

type Event struct {
	Kind            EventKind
	Text            string
	ToolCallID      string
	ToolName        string
	Metadata        map[string]any
	Status          string
	Count           int
	DurationMS      int64
	Questions       []core.UserInputQuestion
	Choices         []string
	Approval        *policy.ApprovalRequest
	LastResponse    string
	ModelChoices    []string
	EffortChoices   []string
	CurrentModel    string
	CurrentEffort   string
	ThinkingChoices []string
	CurrentThinking string
	ApprovalChoices []string
	CurrentApproval string
	SessionID       string
	Messages        []core.Message
}

type Service struct {
	ctx              context.Context
	serviceCtxCancel context.CancelFunc
	app              *app.App
	events           chan Event
	cancelMu         sync.Mutex
	cancel           context.CancelFunc
	active           bool

	approveMu     sync.Mutex
	approvals     map[string]chan approvalDecision
	sessionGrants map[string]map[string]bool

	inputMu sync.Mutex
	inputs  map[string]chan userInputDecision
}

type userInputDecision struct {
	response core.UserInputResponse
	ok       bool
}

type approvalDecision int

const (
	approvalDeny approvalDecision = iota
	approvalAllow
	approvalAllowSession
)

func New(ctx context.Context, cfg app.Config, start app.StartOptions) (*Service, error) {
	ctx, cancel := context.WithCancel(ctx)
	a, err := app.New(ctx, cfg, start)
	if err != nil {
		cancel()
		return nil, err
	}
	s := &Service{
		ctx:              ctx,
		serviceCtxCancel: cancel,
		app:              a,
		events:           make(chan Event, 512),
		approvals:        map[string]chan approvalDecision{},
		sessionGrants:    map[string]map[string]bool{},
		inputs:           map[string]chan userInputDecision{},
	}
	a.SetApprovalFunc(s.awaitApproval)
	a.SetUserInputFunc(s.awaitUserInput)
	for _, line := range a.StartupLines() {
		s.emit(Event{Kind: EventInfo, Text: line})
	}
	s.emitSessionHydrated()
	s.startMCPStartup()
	return s, nil
}

func (s *Service) Events() <-chan Event { return s.events }
func (s *Service) SessionID() string    { return s.app.SessionID() }
func (s *Service) WorkspaceRoot() string {
	return s.app.WorkspaceRoot()
}
func (s *Service) Model() string           { return s.app.Model() }
func (s *Service) ReasoningEffort() string { return s.app.ReasoningEffort() }
func (s *Service) ThinkingEnabled() bool   { return s.app.ThinkingEnabled() }

func (s *Service) Close() error {
	if s == nil || s.app == nil {
		return nil
	}
	if s.serviceCtxCancel != nil {
		s.serviceCtxCancel()
	}
	return s.app.Close()
}
