package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	tuirender "github.com/usewhale/whale/internal/tui/render"
)

type liveStream struct {
	role         string
	kind         tuirender.MessageKind
	buffer       string
	hasCommitted bool
}

type streamCommit struct {
	message  *tuirender.UIMessage
	gapAfter bool
}

func newLiveStream(role string, kind tuirender.MessageKind) liveStream {
	return liveStream{role: role, kind: kind}
}

func (s *liveStream) reset() {
	s.buffer = ""
	s.hasCommitted = false
}

func (s *liveStream) push(delta string) *streamCommit {
	if delta == "" {
		return nil
	}
	s.buffer += strings.ReplaceAll(delta, "\r\n", "\n")
	idx := strings.LastIndexByte(s.buffer, '\n')
	if idx < 0 {
		return nil
	}
	text := s.buffer[:idx]
	s.buffer = s.buffer[idx+1:]
	return s.commit(text, false)
}

func (s *liveStream) finalize() *streamCommit {
	if strings.TrimSpace(s.buffer) == "" {
		hadCommitted := s.hasCommitted
		s.reset()
		if hadCommitted {
			return &streamCommit{gapAfter: true}
		}
		return nil
	}
	text := s.buffer
	s.reset()
	return &streamCommit{
		message: &tuirender.UIMessage{
			Role: s.role,
			Kind: s.kind,
			Text: strings.TrimRight(text, "\n"),
		},
		gapAfter: true,
	}
}

func (s *liveStream) commit(text string, gapAfter bool) *streamCommit {
	text = strings.TrimRight(text, "\n")
	if strings.TrimSpace(text) == "" {
		return nil
	}
	s.hasCommitted = true
	return &streamCommit{
		message: &tuirender.UIMessage{
			Role: s.role,
			Kind: s.kind,
			Text: text,
		},
		gapAfter: gapAfter,
	}
}

func (s liveStream) tailMessage() *tuirender.UIMessage {
	if strings.TrimSpace(s.buffer) == "" {
		return nil
	}
	return &tuirender.UIMessage{
		Role: s.role,
		Kind: s.kind,
		Text: strings.TrimRight(s.buffer, "\n"),
	}
}

func (s liveStream) active() bool {
	return strings.TrimSpace(s.buffer) != "" || s.hasCommitted
}

func (m *model) initLiveStreams() {
	if m.assistantStream.role == "" {
		m.assistantStream = newLiveStream("assistant", tuirender.KindText)
	}
	if m.reasoningStream.role == "" {
		m.reasoningStream = newLiveStream("think", tuirender.KindThinking)
	}
	if m.planStream.role == "" {
		m.planStream = newLiveStream("plan", tuirender.KindPlan)
	}
}

func (m *model) streamForRole(role string) *liveStream {
	m.initLiveStreams()
	switch role {
	case "think":
		return &m.reasoningStream
	case "plan":
		return &m.planStream
	default:
		return &m.assistantStream
	}
}

func (m *model) pushStreamDelta(role, text string) tea.Cmd {
	cmds := []tea.Cmd{m.flushOtherStreamsCmd(role)}
	if commit := m.streamForRole(role).push(text); commit != nil {
		cmds = append(cmds, m.commitStreamScrollbackCmd(*commit))
	}
	return sequenceCmds(cmds...)
}

func (m *model) flushOtherStreamsCmd(activeRole string) tea.Cmd {
	cmds := make([]tea.Cmd, 0, 2)
	for _, role := range []string{"think", "plan", "assistant"} {
		if role == activeRole {
			continue
		}
		cmds = append(cmds, m.flushStreamCmd(role))
	}
	return sequenceCmds(cmds...)
}

func (m *model) flushAllStreamsCmd() tea.Cmd {
	return sequenceCmds(
		m.flushStreamCmd("think"),
		m.flushStreamCmd("plan"),
		m.flushStreamCmd("assistant"),
	)
}

func (m *model) flushStreamCmd(role string) tea.Cmd {
	commit := m.streamForRole(role).finalize()
	if commit == nil {
		return nil
	}
	return m.commitStreamScrollbackCmd(*commit)
}

func (m *model) resetLiveStreams() {
	m.initLiveStreams()
	m.assistantStream.reset()
	m.reasoningStream.reset()
	m.planStream.reset()
}

func (m model) liveStreamMessages() []tuirender.UIMessage {
	m.initLiveStreams()
	messages := make([]tuirender.UIMessage, 0, 3)
	for _, stream := range []liveStream{m.reasoningStream, m.planStream, m.assistantStream} {
		if msg := stream.tailMessage(); msg != nil {
			messages = append(messages, *msg)
		}
	}
	return messages
}

func (m model) commitStreamScrollbackCmd(commit streamCommit) tea.Cmd {
	if commit.message == nil {
		if commit.gapAfter {
			return tea.Println("")
		}
		return nil
	}
	text := m.scrollbackText([]tuirender.UIMessage{*commit.message})
	if strings.TrimSpace(text) == "" {
		return nil
	}
	if commit.gapAfter {
		return tea.Println(text + "\n")
	}
	return tea.Println(text)
}

func sequenceCmds(cmds ...tea.Cmd) tea.Cmd {
	compact := make([]tea.Cmd, 0, len(cmds))
	for _, cmd := range cmds {
		if cmd != nil {
			compact = append(compact, cmd)
		}
	}
	if len(compact) == 0 {
		return nil
	}
	if len(compact) == 1 {
		return compact[0]
	}
	return tea.Sequence(compact...)
}
