package tui

import (
	"fmt"
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
	lastNonEnterKeyTime            time.Time
	lastPasteEnterTime             time.Time
	quitArmedUntil                 time.Time
	promptHistory                  []string
	historyIndex                   int
	historyDraft                   string
	lastHistoryText                string
	inHistoryNav                   bool
	queuedPrompts                  []queuedPrompt
	nativeScrollbackPrinted        int
	pasteBuffer                    []rune
	pasteBufferTime                time.Time
}

type queuedPrompt struct {
	Text string
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
type pasteFlushMsg struct{}

// pasteBufferThreshold defines the maximum inter-character gap that distinguishes
// a paste stream from normal typing. Characters arriving within this window are
// accumulated and flushed as a single paste operation. 50ms easily covers paste
// events (microsecond gaps) while staying well below human typing speed (~200ms).
const pasteBufferThreshold = 50 * time.Millisecond

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
		eventCmd, quit, direct := m.handleServiceEvent(service.Event(msg))
		if quit {
			return m, tea.Quit
		}
		if direct {
			return m, eventCmd
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
	case pasteFlushMsg:
		m.flushPasteBuffer()
		m.refreshViewportContent()
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
		if msg.Type != tea.KeyEnter {
			m.lastNonEnterKeyTime = time.Now()
		}
		cmd, quit, handled := m.handleKeyMsg(msg)
		if quit {
			m.flushPasteBuffer()
			return m, tea.Quit
		}
		if handled {
			m.flushPasteBuffer()
			return m, cmd
		}
		// Unhandled key: detect paste streams by measuring inter-character gaps.
		// The first character always falls through to normal textarea processing.
		// Subsequent characters arriving within the threshold are accumulated and
		// flushed atomically, avoiding the "one character at a time" rendering.
		if m.mode == modeChat && msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
			now := time.Now()
			if !m.pasteBufferTime.IsZero() && now.Sub(m.pasteBufferTime) <= pasteBufferThreshold {
				// Rapid character stream: buffer it as part of a paste.
				m.pasteBuffer = append(m.pasteBuffer, msg.Runes...)
				m.pasteBufferTime = now
				m.updateSlashMatches()
				m.refreshViewportContent()
				// Arm a timer to flush this batch if no more chars arrive.
				return m, tea.Tick(pasteBufferThreshold, func(time.Time) tea.Msg {
					return pasteFlushMsg{}
				})
			}
			// First character, or gap too large: flush any pending buffer,
			// then fall through so this character is processed normally.
			m.flushPasteBuffer()
		} else {
			// Non-character event: flush accumulated paste buffer.
			m.flushPasteBuffer()
		}
	}
	prevInput := m.input.Value()
	cmd := m.input.Update(msg)
	m.updateSlashMatches()
	if m.inHistoryNav && m.input.Value() != prevInput {
		m.resetHistoryNavigation()
	}
	m.refreshViewportContent()
	return m, cmd
}

func (m *model) flushPasteBuffer() {
	if len(m.pasteBuffer) == 0 {
		return
	}
	text := string(m.pasteBuffer)
	m.pasteBuffer = nil
	m.pasteBufferTime = time.Time{}

	m.input.HandlePaste(text)
	m.resetHistoryNavigation()
	m.updateSlashMatches()
	m.refreshViewportContent()
}
