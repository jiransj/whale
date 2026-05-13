package tui

import (
	"strings"

	tuirender "github.com/usewhale/whale/internal/tui/render"
)

const (
	chatTailRenderMessageLimit = 80
	chatTailRenderLineFloor    = 80
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
	content := ""
	if m.page == pageChat {
		if forceBottom {
			m.unfreezeChatViewport()
			m.followTail = true
		}
		m.viewport.Width = max(10, mainWidth)
		m.viewport.Height = max(1, bodyHeight)
		if m.viewportFrozen {
			content = m.frozenChatContent
		} else if m.shouldRenderChatTailOnly(forceBottom) {
			content = m.chatTailContent(mainWidth, bodyHeight)
		} else {
			content = m.chatContent(mainWidth)
		}
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
	if m.page == pageChat && (forceBottom || m.followTail) {
		m.viewport.GotoBottom()
	}
}

func (m *model) shouldRenderChatTailOnly(forceBottom bool) bool {
	return m.page == pageChat && m.busy && m.followTail && !m.viewportFrozen && !forceBottom
}

func (m *model) freezeChatViewport() {
	if m.page != pageChat || m.viewportFrozen {
		return
	}
	mainWidth, bodyHeight := m.layoutDims()
	m.viewport.Width = max(10, mainWidth)
	m.viewport.Height = max(1, bodyHeight)
	m.frozenChatContent = m.chatContent(mainWidth)
	m.viewport.SetContent(m.frozenChatContent)
	if m.followTail {
		m.viewport.GotoBottom()
	}
	m.viewportFrozen = true
}

func (m *model) unfreezeChatViewport() {
	m.viewportFrozen = false
	m.frozenChatContent = ""
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

func (m model) chatTailContent(width, height int) string {
	messages := m.chatMessages()
	if len(messages) > chatTailRenderMessageLimit {
		messages = messages[len(messages)-chatTailRenderMessageLimit:]
	}
	lines := tuirender.ChatLines(messages, max(20, width-2))
	lineLimit := max(chatTailRenderLineFloor, max(1, height)*4)
	if len(lines) > lineLimit {
		lines = lines[len(lines)-lineLimit:]
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}
