package tui

import (
	"strings"

	tuirender "github.com/usewhale/whale/internal/tui/render"
)

func (m *model) refreshViewportContent() {
	mainWidth, bodyHeight := m.layoutDims()
	m.refreshViewportContentForSize(mainWidth, bodyHeight, false)
}

func (m *model) refreshViewportContentFollow(forceBottom bool) {
	mainWidth, bodyHeight := m.layoutDims()
	m.refreshViewportContentForSize(mainWidth, bodyHeight, forceBottom)
}

func (m *model) refreshViewportContentForSize(mainWidth, bodyHeight int, forceBottom bool) {
	wasAtBottom := m.viewport.AtBottom()
	content := ""
	if m.page == pageChat {
		m.viewport.Width = max(10, mainWidth)
		m.viewport.Height = max(1, bodyHeight)
		content = m.chatContent(mainWidth)
	} else {
		m.viewport.Width = max(10, mainWidth-2)
		m.viewport.Height = max(1, bodyHeight-2)
	}
	if m.page == pageLogs {
		content = strings.Join(m.filteredLogs(), "\n")
	}
	if m.page == pageDiff {
		content = strings.Join(m.renderDiffs(), "\n")
	}
	m.viewport.SetContent(content)
	if m.page == pageChat && (forceBottom || wasAtBottom) {
		m.viewport.GotoBottom()
	}
}

func (m model) renderChatLines(width int) []string {
	messages := m.chatMessages()
	if len(messages) == 0 {
		return nil
	}
	return tuirender.ChatLines(messages, width)
}

func (m model) scrollbackText(messages []tuirender.UIMessage) string {
	lines := tuirender.ChatLines(messages, m.chatRenderWidth())
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

func (m model) chatMessages() []tuirender.UIMessage {
	live := []tuirender.UIMessage(nil)
	if m.assembler != nil {
		live = m.assembler.Snapshot()
	}
	if len(m.transcript) == 0 && len(live) == 0 {
		return nil
	}
	out := make([]tuirender.UIMessage, 0, len(m.transcript)+len(live))
	out = append(out, m.transcript...)
	out = append(out, live...)
	return out
}

func (m model) chatContent(width int) string {
	lines := tuirender.ChatLines(m.chatMessages(), max(20, width-2))
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}
