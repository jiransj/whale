package tui

import (
	"strings"

	tuirender "github.com/usewhale/whale/internal/tui/render"
)

func (m *model) append(role, text string) {
	if m.assembler == nil {
		m.assembler = tuirender.NewAssembler()
	}
	m.assembler.AppendDelta(role, text)
	m.refreshLiveViewportContent()
}

func (m *model) appendNotice(text string) {
	if m.assembler == nil {
		m.assembler = tuirender.NewAssembler()
	}
	m.assembler.AddNotice(text)
	m.refreshLiveViewportContent()
}

func (m *model) appendLiveToolResult(text, role string) {
	if m.assembler == nil {
		m.assembler = tuirender.NewAssembler()
	}
	m.assembler.AddToolResultWithRole("", text, role)
	m.refreshLiveViewportContent()
}

func (m *model) beginTurnTranscript() {
	m.turnTranscriptStart = len(m.transcript)
	m.visibleAssistantThisTurn = ""
}

func (m *model) recordAssistantDelta(text string) {
	m.visibleAssistantThisTurn += text
}

func (m *model) reconcileFinalAssistant(lastResponse string) bool {
	final := strings.TrimRight(lastResponse, "\n")
	if strings.TrimSpace(final) == "" {
		return false
	}
	visible := strings.TrimRight(m.visibleAssistantThisTurn, "\n")
	if visible == final {
		return false
	}
	if visible != "" && strings.HasPrefix(final, visible) {
		m.append("assistant", strings.TrimPrefix(final, visible))
		m.sawAssistantThisTurn = true
		return true
	}
	m.replaceCurrentTurnAssistant(final)
	m.sawAssistantThisTurn = true
	return true
}

func (m *model) replaceCurrentTurnAssistant(text string) {
	start := m.turnTranscriptStart
	if start < 0 || start > len(m.transcript) {
		start = len(m.transcript)
	}
	out := m.transcript[:start]
	for _, msg := range m.transcript[start:] {
		if msg.Role == "assistant" && msg.Kind == tuirender.KindText {
			continue
		}
		out = append(out, msg)
	}
	m.transcript = out
	if m.nativeScrollbackPrinted > start {
		m.nativeScrollbackPrinted = start
	}
	if m.assembler == nil {
		m.assembler = tuirender.NewAssembler()
	}
	m.assembler.RemoveAssistantMessages()
	m.assembler.AppendDelta("assistant", text)
	m.refreshLiveViewportContent()
}

func (m *model) markNoFinalAnswerIfNeeded() bool {
	if !m.sawReasoningThisTurn || m.sawAssistantThisTurn || m.sawPlanThisTurn || m.sawTerminalToolOutcomeThisTurn {
		return false
	}
	if m.chatMode == "plan" {
		m.appendNotice("No plan was produced. Ask the model to propose the plan again.")
	} else {
		m.appendNotice("No final answer was produced. Ask the model to answer directly or retry the last step.")
	}
	m.addLog(logEntry{
		Kind:    "no_final_answer",
		Source:  "assistant",
		Summary: "reasoning-only turn completed without final answer",
		Raw:     "The model produced reasoning content but no assistant content.",
	})
	return true
}

func suppressesNoFinalAnswer(role string) bool {
	switch strings.TrimSpace(role) {
	case "result_denied", "result_canceled", "result_timeout":
		return true
	default:
		return false
	}
}

func (m *model) appendPlanDelta(text string) {
	if m.assembler == nil {
		m.assembler = tuirender.NewAssembler()
	}
	m.assembler.AddPlanDelta(text)
	m.refreshLiveViewportContent()
}
