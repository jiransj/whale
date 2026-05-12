package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/usewhale/whale/internal/app"
	"github.com/usewhale/whale/internal/app/service"
	tuirender "github.com/usewhale/whale/internal/tui/render"
)

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

func (m *model) submitPrompt(value string) tea.Cmd {
	return m.submitPromptWithBinding(value, m.currentSkillBinding(value))
}

func (m *model) submitPromptWithBinding(value string, binding *app.SkillBinding) tea.Cmd {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	m.recordPromptHistory(value)
	m.resetHistoryNavigation()
	m.appendTranscript("you", tuirender.KindText, visibleSubmittedText(value))
	m.input.SetValue("")
	m.skillBinding = nil
	m.slash.matches = nil
	m.slash.selected = 0
	m.startBusy()
	m.status = "running"
	m.dispatchIntent(service.Intent{Kind: service.IntentSubmit, Input: value, SkillBinding: binding})
	m.refreshViewportContentFollow(true)
	return busyTickCmd()
}

func (m *model) enqueuePrompt(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	m.queuedPrompts = append(m.queuedPrompts, queuedPrompt{Text: value, SkillBinding: m.currentSkillBinding(value)})
	m.input.SetValue("")
	m.skillBinding = nil
	m.resetHistoryNavigation()
	m.slash.matches = nil
	m.slash.selected = 0
	m.status = fmt.Sprintf("queued (%d)", len(m.queuedPrompts))
	m.refreshViewportContent()
	return true
}

func (m *model) popQueuedPrompt() (queuedPrompt, bool) {
	if len(m.queuedPrompts) == 0 {
		return queuedPrompt{}, false
	}
	next := m.queuedPrompts[0]
	copy(m.queuedPrompts, m.queuedPrompts[1:])
	m.queuedPrompts = m.queuedPrompts[:len(m.queuedPrompts)-1]
	return next, true
}

func (m *model) restoreQueuedPromptsToComposer() bool {
	if len(m.queuedPrompts) == 0 {
		return false
	}
	parts := make([]string, 0, len(m.queuedPrompts)+1)
	for _, prompt := range m.queuedPrompts {
		if text := strings.TrimSpace(prompt.Text); text != "" {
			parts = append(parts, text)
		}
	}
	if current := strings.TrimSpace(m.input.Value()); current != "" {
		parts = append(parts, current)
	}
	m.queuedPrompts = nil
	m.skillBinding = nil
	m.input.SetValue(strings.Join(parts, "\n"))
	m.resetHistoryNavigation()
	m.updateSlashMatches()
	m.refreshViewportContent()
	return true
}
