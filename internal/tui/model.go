package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/usewhale/whale/internal/app"
	"github.com/usewhale/whale/internal/app/service"
	"github.com/usewhale/whale/internal/core"
	"github.com/usewhale/whale/internal/defaults"
	"github.com/usewhale/whale/internal/tui/composer"
	tuirender "github.com/usewhale/whale/internal/tui/render"
)

type mode int

const (
	modeChat mode = iota
	modeApproval
	modeSessionPicker
	modeUserInput
	modeModelPicker
	modePermissionsPicker
	modePlanImplementation
)

type page int

const (
	pageChat page = iota
	pageLogs
	pageDiff
)

type model struct {
	svc        *service.Service
	dispatch   func(service.Intent)
	input      composer.Composer
	viewport   viewport.Model
	assembler  *tuirender.Assembler
	transcript []tuirender.UIMessage
	logs       []logEntry
	diffs      []diffEntry
	width      int
	height     int
	mode       mode
	page       page
	status     string
	busy       bool
	busySince  time.Time
	stopping   bool
	sidebar    bool
	model      string
	effort     string
	thinking   string
	chatMode   string
	product    string
	version    string
	cwd        string
	approval   struct {
		toolCallID string
		toolName   string
		reason     string
		metadata   map[string]any
		selected   int
	}
	sessionChoices []string
	sessionIndex   int
	userInput      struct {
		toolCallID     string
		toolName       string
		questions      []core.UserInputQuestion
		index          int
		selectedOption int
		answers        []core.UserInputAnswer
	}
	palette struct {
		actions  []paletteAction
		selected int
	}
	logFilterInput textinput.Model
	logFilter      string
	slash          struct {
		all      []string
		autoRun  map[string]bool
		matches  []string
		selected int
	}
	modelPicker struct {
		stage     int // 0 model, 1 effort, 2 thinking
		models    []string
		efforts   []string
		thinkings []string
		modelIx   int
		effIx     int
		thinkIx   int
	}
	permissionsPicker struct {
		choices []string
		index   int
	}
	planImplementation struct {
		index int
	}
	sawPlanThisTurn                bool
	sawAssistantThisTurn           bool
	sawReasoningThisTurn           bool
	sawTerminalToolOutcomeThisTurn bool
	quitArmedUntil                 time.Time
	promptHistory                  []string
	historyIndex                   int
	historyDraft                   string
	lastHistoryText                string
	inHistoryNav                   bool
	nativeScrollbackPrinted        int
}

type paletteAction struct {
	Label string
	Run   func(*model)
}

type logEntry struct {
	Kind    string
	Source  string
	Summary string
	Raw     string
}

type diffEntry struct {
	Source string
	Line   string
}

type svcMsg service.Event

type errMsg struct{ err error }
type quitTimeoutMsg struct{}
type busyTickMsg struct{}

func newModel(svc *service.Service, modelName, effort, thinking string) model {
	filter := textinput.New()
	filter.Placeholder = "filter logs (press /)"
	filter.Prompt = "/"
	filter.CharLimit = 200
	vp := viewport.New(80, 20)
	if modelName == "" {
		modelName = defaults.DefaultModel
	}
	if effort == "" {
		effort = defaults.DefaultReasoningEffort
	}
	if thinking == "" {
		thinking = "on"
	}
	m := model{
		svc:            svc,
		input:          composer.New(),
		viewport:       vp,
		assembler:      tuirender.NewAssembler(),
		status:         "ready",
		page:           pageChat,
		sidebar:        false,
		logFilterInput: filter,
		model:          modelName,
		effort:         effort,
		thinking:       thinking,
		chatMode:       "agent",
		product:        "Whale",
		version:        resolveVersion(),
		cwd:            resolveWorkingDirectory(),
		historyIndex:   -1,
	}
	if svc != nil {
		m.dispatch = svc.Dispatch
	}
	m.slash.all = parseSlashCommands(app.CommandsHelp)
	m.slash.autoRun = buildSlashAutoRunMap(app.CommandsHelp)
	m.resetTranscriptWithHeader()
	return m
}

func (m *model) dispatchIntent(in service.Intent) {
	if m.dispatch != nil {
		m.dispatch(in)
	}
}

func waitEventCmd(svc *service.Service) tea.Cmd {
	return func() tea.Msg {
		ev := <-svc.Events()
		return svcMsg(ev)
	}
}

func armQuitCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return quitTimeoutMsg{} })
}

func busyTickCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return busyTickMsg{} })
}

func (m *model) startBusy() {
	if m.busySince.IsZero() {
		m.busySince = time.Now()
	}
	m.busy = true
}

func (m *model) stopBusy() {
	m.busy = false
	m.busySince = time.Time{}
}

// clearScreenCmd clears the visible terminal and scrollback buffer,
// then forces a full TUI redraw. Uses ANSI \033[3J to clear scrollback
// in addition to \033[H\033[2J (visible area).
func clearScreenCmd() tea.Cmd {
	return func() tea.Msg {
		fmt.Print("\033[H\033[2J\033[3J")
		return nil
	}
}

func (m model) Init() tea.Cmd { return waitEventCmd(m.svc) }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.SetWidth(max(20, m.width-4))
		mainWidth, bodyHeight := m.layoutDims()
		m.viewport.Width = max(10, mainWidth-2)
		m.viewport.Height = bodyHeight - 2
		m.refreshViewportContent()
		return m, nil
	case svcMsg:
		var eventCmd tea.Cmd
		ev := service.Event(msg)
		switch ev.Kind {
		case service.EventAssistantDelta:
			m.append("assistant", ev.Text)
			m.addLog(logEntry{Kind: "assistant_delta", Source: "assistant", Summary: ev.Text, Raw: ev.Text})
			if strings.TrimSpace(ev.Text) != "" {
				m.sawAssistantThisTurn = true
			}
			m.startBusy()
		case service.EventReasoningDelta:
			m.append("think", ev.Text)
			m.addLog(logEntry{Kind: "reasoning_delta", Source: "reasoning", Summary: ev.Text, Raw: ev.Text})
			if strings.TrimSpace(ev.Text) != "" {
				m.sawReasoningThisTurn = true
			}
		case service.EventPlanDelta:
			m.appendPlanDelta(ev.Text)
			m.addLog(logEntry{Kind: "plan_delta", Source: "plan", Summary: ev.Text, Raw: ev.Text})
			if strings.TrimSpace(ev.Text) != "" {
				m.sawPlanThisTurn = true
			}
		case service.EventPlanCompleted:
			if strings.TrimSpace(ev.Text) != "" {
				if m.assembler == nil {
					m.assembler = tuirender.NewAssembler()
				}
				m.assembler.SetPlan(ev.Text)
				m.commitLiveTranscript(false)
				m.sawPlanThisTurn = true
			}
			m.addLog(logEntry{Kind: "plan_completed", Source: "plan", Summary: truncateLine(ev.Text, 120), Raw: ev.Text})
		case service.EventInfo:
			if !isEnvironmentInventoryBlock(ev.Text) {
				m.append("info", ev.Text)
			} else {
				m.addLog(logEntry{
					Kind:    "env_summary",
					Source:  "system",
					Summary: "environment summary captured",
					Raw:     ev.Text,
				})
			}
			m.addLog(logEntry{Kind: "info", Source: "system", Summary: ev.Text, Raw: ev.Text})
			m.status = "ready"
			m.syncModelEffortFromInfo(ev.Text)
		case service.EventError:
			m.append("error", ev.Text)
			m.addLog(logEntry{Kind: "error", Source: "system", Summary: ev.Text, Raw: ev.Text})
			m.status = "error"
		case service.EventToolCall:
			m.appendToolCall(ev.ToolCallID, ev.ToolName, ev.Text)
			m.addLog(logEntry{
				Kind:    "tool_call",
				Source:  ev.ToolName,
				Summary: fmt.Sprintf("%s (id=%s)", ev.Text, ev.ToolCallID),
				Raw:     fmt.Sprintf("id=%s\ninput=%s", ev.ToolCallID, ev.Text),
			})
		case service.EventToolResult:
			role, text := summarizeToolResultForChat(ev.ToolName, ev.Text)
			if suppressesNoFinalAnswer(role) {
				m.sawTerminalToolOutcomeThisTurn = true
			}
			if !m.updateToolCallFromResult(ev.ToolCallID, ev.ToolName, ev.Text, role, text, ev.Metadata) {
				m.assembler.AddToolResultWithRole("", text, role)
			}
			m.addLog(logEntry{Kind: "tool_result", Source: ev.ToolName, Summary: truncateLine(ev.Text, 120), Raw: ev.Text})
			m.captureDiffMetadata(ev.ToolName, ev.Metadata)
			m.captureDiff(ev.ToolName, ev.Text)
			m.commitLiveTranscript(false)
		case service.EventTaskStarted:
			m.status = ev.Text
			m.addLog(logEntry{Kind: "task_started", Source: ev.ToolName, Summary: ev.Text, Raw: fmt.Sprintf("%+v", ev.Metadata)})
		case service.EventTaskProgress:
			m.status = ev.Text
			m.updateTaskProgress(ev.ToolCallID, ev.ToolName, ev.Text)
			m.addLog(logEntry{Kind: "task_progress", Source: ev.ToolName, Summary: ev.Text, Raw: fmt.Sprintf("%+v", ev.Metadata)})
		case service.EventTaskCompleted:
			m.status = ev.Text
			m.addLog(logEntry{Kind: "task_completed", Source: ev.ToolName, Summary: ev.Text, Raw: fmt.Sprintf("%+v", ev.Metadata)})
		case service.EventMCPStatus:
			m.status = ev.Text
			if ev.Status == "failed" || ev.Status == "cancelled" {
				m.append("error", ev.Text)
			}
			m.addLog(logEntry{Kind: "mcp_status", Source: "mcp", Summary: ev.Text, Raw: fmt.Sprintf("%+v", ev.Metadata)})
		case service.EventMCPComplete:
			m.status = ev.Text
			m.addLog(logEntry{Kind: "mcp_complete", Source: "mcp", Summary: ev.Text, Raw: fmt.Sprintf("%+v", ev.Metadata)})
		case service.EventApprovalRequired:
			m.mode = modeApproval
			m.approval.toolCallID = ev.ToolCallID
			m.approval.toolName = ev.ToolName
			m.approval.reason = ev.Text
			m.approval.metadata = ev.Metadata
			m.approval.selected = 0
			m.addLog(logEntry{Kind: "approval_required", Source: ev.ToolName, Summary: ev.Text, Raw: ev.Text})
			m.status = "approval required"
		case service.EventUserInputRequired:
			m.mode = modeUserInput
			m.userInput.toolCallID = ev.ToolCallID
			m.userInput.toolName = ev.ToolName
			m.userInput.questions = ev.Questions
			m.userInput.index = 0
			m.userInput.selectedOption = 0
			m.userInput.answers = nil
			m.addLog(logEntry{Kind: "user_input_required", Source: ev.ToolName, Summary: fmt.Sprintf("%d questions", len(ev.Questions)), Raw: fmt.Sprintf("%+v", ev.Questions)})
			m.status = "user input required"
		case service.EventSessionsListed:
			m.mode = modeSessionPicker
			m.sessionChoices = ev.Choices
			m.sessionIndex = firstSessionChoiceIndex(ev.Choices)
			m.addLog(logEntry{Kind: "sessions_listed", Source: "session", Summary: fmt.Sprintf("%d sessions", len(ev.Choices)), Raw: strings.Join(ev.Choices, "\n")})
			m.status = "session picker"
		case service.EventTurnDone:
			wasBusy := m.busy
			m.stopBusy()
			m.stopping = false
			m.markNoFinalAnswerIfNeeded()
			m.commitLiveTranscript(false)
			m.addLog(logEntry{Kind: "turn_done", Source: "assistant", Summary: truncateLine(ev.LastResponse, 120), Raw: ev.LastResponse})
			m.status = "ready"
			if wasBusy && m.chatMode == "plan" && m.sawPlanThisTurn && m.mode == modeChat {
				m.mode = modePlanImplementation
				m.planImplementation.index = 0
			}
			m.sawPlanThisTurn = false
			m.sawAssistantThisTurn = false
			m.sawReasoningThisTurn = false
			m.sawTerminalToolOutcomeThisTurn = false
		case service.EventModelPicker:
			m.stopBusy()
			m.stopping = false
			m.mode = modeModelPicker
			m.modelPicker.stage = 0
			m.modelPicker.models = ev.ModelChoices
			m.modelPicker.efforts = ev.EffortChoices
			m.modelPicker.thinkings = ev.ThinkingChoices
			m.modelPicker.modelIx = indexOf(ev.ModelChoices, ev.CurrentModel)
			m.modelPicker.effIx = indexOf(ev.EffortChoices, ev.CurrentEffort)
			m.modelPicker.thinkIx = indexOf(ev.ThinkingChoices, ev.CurrentThinking)
		case service.EventPermissionsPicker:
			m.stopBusy()
			m.stopping = false
			m.mode = modePermissionsPicker
			m.permissionsPicker.choices = ev.ApprovalChoices
			m.permissionsPicker.index = indexOf(ev.ApprovalChoices, ev.CurrentApproval)
		case service.EventClearScreen:
			m.assembler.Reset()
			m.resetTranscriptWithHeader()
			m.sawPlanThisTurn = false
			m.sawAssistantThisTurn = false
			m.sawReasoningThisTurn = false
			m.sawTerminalToolOutcomeThisTurn = false
			m.logs = nil
			m.diffs = nil
			m.status = "terminal cleared"
			return m, tea.Sequence(clearScreenCmd(), waitEventCmd(m.svc))
		case service.EventSessionHydrated:
			m.assembler.Reset()
			m.resetTranscriptWithHeader()
			m.sawPlanThisTurn = false
			m.sawAssistantThisTurn = false
			m.sawReasoningThisTurn = false
			m.sawTerminalToolOutcomeThisTurn = false
			m.logs = nil
			m.diffs = nil
			m.hydrateSessionMessages(ev.Messages)
			m.commitLiveTranscript(true)
			m.trimHydratedTranscriptForDisplay(maxHydratedTranscriptLines)
			m.status = "ready"
		case service.EventExitRequested:
			m.dispatchIntent(service.Intent{Kind: service.IntentShutdown})
			return m, tea.Quit
		}
		return m, tea.Sequence(eventCmd, m.flushNativeScrollbackCmd(), waitEventCmd(m.svc))
	case quitTimeoutMsg:
		if !m.quitArmedUntil.IsZero() && time.Now().After(m.quitArmedUntil) {
			m.quitArmedUntil = time.Time{}
			if m.status == "Press Ctrl+C again to quit" {
				m.status = "ready"
			}
		}
		return m, nil
	case busyTickMsg:
		if m.busy {
			return m, busyTickCmd()
		}
		return m, nil
	case tea.MouseMsg:
		m.refreshViewportContent()
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.viewport.LineUp(3)
		case tea.MouseButtonWheelDown:
			m.viewport.LineDown(3)
		}
		return m, nil
	case tea.KeyMsg:
		if !m.quitArmedUntil.IsZero() && time.Now().After(m.quitArmedUntil) {
			m.quitArmedUntil = time.Time{}
			if m.status == "Press Ctrl+C again to quit" {
				m.status = "ready"
			}
		}
		m.updateSlashMatches()
		if m.mode == modeChat && msg.Paste {
			m.input.HandlePaste(string(msg.Runes))
			m.resetHistoryNavigation()
			m.updateSlashMatches()
			m.refreshViewportContent()
			return m, nil
		}
		if m.mode == modeChat {
			switch msg.String() {
			case "shift+tab", "backtab":
				if !m.busy && !m.hasSlashSuggestions() {
					m.startBusy()
					m.status = "switching mode"
					m.dispatchIntent(service.Intent{Kind: service.IntentToggleMode})
					return m, busyTickCmd()
				}
			case "up":
				if m.hasSlashSuggestions() {
					if m.slash.selected > 0 {
						m.slash.selected--
					}
					return m, nil
				}
				if m.shouldHandleHistoryNavigation() {
					if m.historyPrev() {
						return m, nil
					}
				}
			case "down":
				if m.hasSlashSuggestions() {
					if m.slash.selected < len(m.slash.matches)-1 {
						m.slash.selected++
					}
					return m, nil
				}
				if m.shouldHandleHistoryNavigation() {
					if m.historyNext() {
						return m, nil
					}
				}
			case "tab":
				if m.hasSlashSuggestions() {
					if cmd := safeChoice(m.slash.matches, m.slash.selected); cmd != "" {
						m.input.SetValue(cmd)
						m.updateSlashMatches()
					}
					return m, nil
				}
			case "esc":
				if m.busy {
					if !m.stopping {
						if m.svc != nil {
							m.dispatchIntent(service.Intent{Kind: service.IntentShutdown})
						}
						m.status = "stopping"
						m.stopping = true
						m.appendNotice(m.turnInterruptedNoticeText())
					}
					m.commitLiveTranscript(false)
					return m, m.flushNativeScrollbackCmd()
				}
				if m.hasSlashSuggestions() {
					m.slash.matches = nil
					m.slash.selected = 0
					return m, nil
				}
			case "pgup", "pgdown", "ctrl+d", "home", "end":
				m.handleViewportScrollKey(msg.String())
				return m, nil
			}
		}
		switch m.mode {
		case modeApproval:
			switch msg.String() {
			case "left", "h":
				m.approval.selected = (m.approval.selected + 2) % 3
				return m, nil
			case "right", "l", "tab":
				m.approval.selected = (m.approval.selected + 1) % 3
				return m, nil
			case "enter":
				switch m.approval.selected {
				case 0:
					m.dispatchIntent(service.Intent{Kind: service.IntentAllowTool, ToolCallID: m.approval.toolCallID})
					m.addLog(logEntry{Kind: "approval_allow", Source: m.approval.toolName, Summary: "allow", Raw: "allow"})
					m.status = "approved"
					m.appendNotice(m.approvalNoticeText("allow"))
					m.mode = modeChat
					return m, m.flushNativeScrollbackCmd()
				case 1:
					m.dispatchIntent(service.Intent{Kind: service.IntentAllowToolForSession, ToolCallID: m.approval.toolCallID})
					m.addLog(logEntry{Kind: "approval_allow_session", Source: m.approval.toolName, Summary: "allow for session", Raw: "allow_session"})
					m.status = "approved for session"
					m.appendNotice(m.approvalNoticeText("allow_session"))
					m.mode = modeChat
					return m, m.flushNativeScrollbackCmd()
				default:
					m.dispatchIntent(service.Intent{Kind: service.IntentDenyTool, ToolCallID: m.approval.toolCallID})
					m.addLog(logEntry{Kind: "approval_deny", Source: m.approval.toolName, Summary: "deny", Raw: "deny"})
					m.status = "rejected"
					m.appendNotice(m.approvalNoticeText("deny"))
					m.mode = modeChat
					return m, m.flushNativeScrollbackCmd()
				}
			case "a":
				m.dispatchIntent(service.Intent{Kind: service.IntentAllowTool, ToolCallID: m.approval.toolCallID})
				m.addLog(logEntry{Kind: "approval_allow", Source: m.approval.toolName, Summary: "allow", Raw: "allow"})
				m.mode = modeChat
				m.status = "approved"
				m.appendNotice(m.approvalNoticeText("allow"))
				return m, m.flushNativeScrollbackCmd()
			case "s":
				m.dispatchIntent(service.Intent{Kind: service.IntentAllowToolForSession, ToolCallID: m.approval.toolCallID})
				m.addLog(logEntry{Kind: "approval_allow_session", Source: m.approval.toolName, Summary: "allow for session", Raw: "allow_session"})
				m.mode = modeChat
				m.status = "approved for session"
				m.appendNotice(m.approvalNoticeText("allow_session"))
				return m, m.flushNativeScrollbackCmd()
			case "d", "esc", "ctrl+c":
				m.dispatchIntent(service.Intent{Kind: service.IntentDenyTool, ToolCallID: m.approval.toolCallID})
				m.addLog(logEntry{Kind: "approval_deny", Source: m.approval.toolName, Summary: "deny", Raw: "deny"})
				m.mode = modeChat
				m.status = "rejected"
				m.appendNotice(m.approvalNoticeText("deny"))
				return m, m.flushNativeScrollbackCmd()
			}
			return m, nil
		case modeSessionPicker:
			switch msg.String() {
			case "esc":
				m.mode = modeChat
				return m, nil
			case "up", "k":
				m.sessionIndex = prevSessionChoiceIndex(m.sessionChoices, m.sessionIndex)
				return m, nil
			case "down", "j":
				m.sessionIndex = nextSessionChoiceIndex(m.sessionChoices, m.sessionIndex)
				return m, nil
			case "enter":
				selected := sessionChoiceNumberAt(m.sessionChoices, m.sessionIndex)
				if selected > 0 {
					m.dispatchIntent(service.Intent{Kind: service.IntentSelectSession, SessionInput: strconv.Itoa(selected)})
				}
				m.mode = modeChat
				return m, nil
			}
			return m, nil
		case modeUserInput:
			if len(m.userInput.questions) == 0 {
				m.dispatchIntent(service.Intent{Kind: service.IntentCancelUserInput, ToolCallID: m.userInput.toolCallID})
				m.mode = modeChat
				return m, nil
			}
			q := m.userInput.questions[m.userInput.index]
			switch msg.String() {
			case "esc":
				m.dispatchIntent(service.Intent{Kind: service.IntentCancelUserInput, ToolCallID: m.userInput.toolCallID})
				m.mode = modeChat
				return m, nil
			case "up", "k":
				if m.userInput.selectedOption > 0 {
					m.userInput.selectedOption--
				}
			case "down", "j":
				if m.userInput.selectedOption < len(q.Options)-1 {
					m.userInput.selectedOption++
				}
			case "enter":
				opt := q.Options[m.userInput.selectedOption]
				m.userInput.answers = append(m.userInput.answers, core.UserInputAnswer{ID: q.ID, Label: opt.Label, Value: opt.Label})
				m.userInput.index++
				m.userInput.selectedOption = 0
				if m.userInput.index >= len(m.userInput.questions) {
					resp := core.UserInputResponse{Answers: m.userInput.answers}
					m.dispatchIntent(service.Intent{Kind: service.IntentSubmitUserInput, ToolCallID: m.userInput.toolCallID, UserInput: &resp})
					m.mode = modeChat
				}
			}
			return m, nil
		case modeModelPicker:
			switch msg.String() {
			case "esc":
				if m.modelPicker.stage > 0 {
					m.modelPicker.stage--
				} else {
					m.mode = modeChat
				}
			case "up", "k":
				if m.modelPicker.stage == 0 && m.modelPicker.modelIx > 0 {
					m.modelPicker.modelIx--
				}
				if m.modelPicker.stage == 1 && m.modelPicker.effIx > 0 {
					m.modelPicker.effIx--
				}
				if m.modelPicker.stage == 2 && m.modelPicker.thinkIx > 0 {
					m.modelPicker.thinkIx--
				}
			case "down", "j":
				if m.modelPicker.stage == 0 && m.modelPicker.modelIx < len(m.modelPicker.models)-1 {
					m.modelPicker.modelIx++
				}
				if m.modelPicker.stage == 1 && m.modelPicker.effIx < len(m.modelPicker.efforts)-1 {
					m.modelPicker.effIx++
				}
				if m.modelPicker.stage == 2 && m.modelPicker.thinkIx < len(m.modelPicker.thinkings)-1 {
					m.modelPicker.thinkIx++
				}
			case "enter":
				if m.modelPicker.stage == 0 {
					m.modelPicker.stage = 1
				} else if m.modelPicker.stage == 1 {
					m.modelPicker.stage = 2
				} else {
					modelName := safeChoice(m.modelPicker.models, m.modelPicker.modelIx)
					effort := safeChoice(m.modelPicker.efforts, m.modelPicker.effIx)
					thinking := safeChoice(m.modelPicker.thinkings, m.modelPicker.thinkIx)
					if modelName != "" && effort != "" && thinking != "" {
						m.dispatchIntent(service.Intent{Kind: service.IntentSetModelAndEffort, Model: modelName, Effort: effort, Thinking: thinking})
						m.model = modelName
						m.effort = effort
						m.thinking = thinking
					}
					m.mode = modeChat
				}
			}
			return m, nil
		case modePermissionsPicker:
			switch msg.String() {
			case "esc":
				m.mode = modeChat
			case "up", "k":
				if m.permissionsPicker.index > 0 {
					m.permissionsPicker.index--
				}
			case "down", "j":
				if m.permissionsPicker.index < len(m.permissionsPicker.choices)-1 {
					m.permissionsPicker.index++
				}
			case "enter":
				choice := safeChoice(m.permissionsPicker.choices, m.permissionsPicker.index)
				mode := approvalChoiceMode(choice)
				if mode != "" {
					m.dispatchIntent(service.Intent{Kind: service.IntentSetApprovalMode, ApprovalMode: mode})
				}
				m.mode = modeChat
			}
			return m, nil
		case modePlanImplementation:
			switch msg.String() {
			case "esc":
				m.mode = modeChat
			case "up", "k", "left", "h":
				if m.planImplementation.index > 0 {
					m.planImplementation.index--
				}
			case "down", "j", "right", "l", "tab":
				if m.planImplementation.index < 1 {
					m.planImplementation.index++
				}
			case "enter":
				if m.planImplementation.index == 0 {
					m.appendTranscript("you", tuirender.KindText, "Implement the plan.")
					m.startBusy()
					m.status = "running"
					m.chatMode = "agent"
					m.dispatchIntent(service.Intent{Kind: service.IntentImplementPlan})
					m.mode = modeChat
					m.refreshViewportContentFollow(true)
					return m, tea.Sequence(m.flushNativeScrollbackCmd(), busyTickCmd())
				}
				m.mode = modeChat
			}
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c":
			if strings.TrimSpace(m.input.Value()) != "" {
				m.input.Reset()
				m.resetHistoryNavigation()
				m.updateSlashMatches()
				m.status = "input cleared"
				return m, nil
			}
			now := time.Now()
			if !m.quitArmedUntil.IsZero() && now.Before(m.quitArmedUntil) {
				m.dispatchIntent(service.Intent{Kind: service.IntentShutdown})
				return m, tea.Quit
			}
			m.quitArmedUntil = now.Add(2 * time.Second)
			m.status = "Press Ctrl+C again to quit"
			return m, armQuitCmd(2 * time.Second)
		case "enter":
			if m.busy {
				if m.stopping {
					m.status = "stopping"
				}
				m.appendNotice(m.busySubmitNoticeText())
				return m, m.flushNativeScrollbackCmd()
			}
			if m.hasSlashSuggestions() {
				if cmd := safeChoice(m.slash.matches, m.slash.selected); cmd != "" {
					m.input.SetValue(cmd)
					m.updateSlashMatches()
					if m.shouldAutoRunSlash(cmd) {
						m.appendTranscript("you", tuirender.KindText, cmd)
						m.input.SetValue("")
						m.slash.matches = nil
						m.slash.selected = 0
						m.startBusy()
						m.status = "running"
						m.dispatchIntent(service.Intent{Kind: service.IntentSubmit, Input: cmd})
						m.refreshViewportContentFollow(true)
						return m, tea.Sequence(m.flushNativeScrollbackCmd(), busyTickCmd())
					}
				}
				return m, nil
			}
			if m.page == pageLogs && m.logFilterInput.Focused() {
				m.logFilter = strings.TrimSpace(m.logFilterInput.Value())
				m.logFilterInput.Blur()
				return m, nil
			}
			if raw := m.input.Value(); strings.HasSuffix(raw, "\\") {
				m.input.SetValue(strings.TrimSuffix(raw, "\\") + "\n")
				m.resetHistoryNavigation()
				m.updateSlashMatches()
				return m, nil
			}
			value := strings.TrimSpace(m.input.Value())
			if value == "" {
				return m, nil
			}
			m.recordPromptHistory(value)
			m.resetHistoryNavigation()
			m.appendTranscript("you", tuirender.KindText, visibleSubmittedText(value))
			m.input.SetValue("")
			m.startBusy()
			m.status = "running"
			m.dispatchIntent(service.Intent{Kind: service.IntentSubmit, Input: value})
			m.refreshViewportContentFollow(true)
			return m, tea.Sequence(m.flushNativeScrollbackCmd(), busyTickCmd())
		}
	}
	var cmd tea.Cmd
	prevInput := m.input.Value()
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "shift+enter", "ctrl+j":
			m.input.InsertNewline()
			m.resetHistoryNavigation()
			m.updateSlashMatches()
			m.refreshViewportContent()
			return m, nil
		case "ctrl+p":
			if m.historyPrev() {
				return m, nil
			}
			return m, nil
		case "ctrl+n":
			if m.historyNext() {
				return m, nil
			}
			return m, nil
		}
		if m.input.HandleKey(keyMsg) {
			m.resetHistoryNavigation()
			m.updateSlashMatches()
			m.refreshViewportContent()
			return m, nil
		}
	}
	cmd = m.input.Update(msg)
	m.updateSlashMatches()
	if m.inHistoryNav && m.input.Value() != prevInput {
		m.resetHistoryNavigation()
	}
	m.refreshViewportContent()
	return m, cmd
}

func (m *model) applyPalette() {
	if m.palette.selected < 0 || m.palette.selected >= len(m.palette.actions) {
		return
	}
	m.palette.actions[m.palette.selected].Run(m)
}
