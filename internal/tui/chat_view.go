package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	tuirender "github.com/usewhale/whale/internal/tui/render"
	tuitheme "github.com/usewhale/whale/internal/tui/theme"
)

func (m *model) append(role, text string) {
	if m.assembler == nil {
		m.assembler = tuirender.NewAssembler()
	}
	m.assembler.AppendDelta(role, text)
}

func (m *model) appendNotice(text string) {
	if m.assembler == nil {
		m.assembler = tuirender.NewAssembler()
	}
	m.assembler.AddNotice(text)
}

func (m *model) markNoFinalAnswerIfNeeded() bool {
	if !m.sawReasoningThisTurn || m.sawAssistantThisTurn || m.sawPlanThisTurn {
		return false
	}
	m.addLog(logEntry{
		Kind:    "no_final_answer",
		Source:  "assistant",
		Summary: "reasoning-only turn completed without final answer",
		Raw:     "The model produced reasoning content but no assistant content.",
	})
	return true
}

func (m *model) appendPlanDelta(text string) {
	if m.assembler == nil {
		m.assembler = tuirender.NewAssembler()
	}
	m.assembler.AddPlanDelta(text)
}

func (m *model) appendToolCall(toolCallID, toolName, text string) {
	if m.assembler == nil {
		m.assembler = tuirender.NewAssembler()
	}
	m.assembler.AddToolCall(toolCallID, summarizeToolCallForChat(toolName, text))
}

func (m *model) updateToolCallFromResult(toolCallID, toolName, result, role, summary string) bool {
	if toolCallID == "" {
		return false
	}
	title := completedToolTitle(toolName, result)
	if summary != "" && summary != "✓" {
		title += "\n" + summary
	}
	return m.assembler.UpdateToolCall(toolCallID, title, role)
}

func summarizeToolCallForChat(toolName, text string) string {
	detail := toolCallDetail(text)
	switch toolDisplayKind(toolName) {
	case "shell":
		if detail == "" {
			detail = "shell command"
		}
		return "Running " + detail
	case "explore":
		line := explorationLine(toolName, detail, toolResultEnvelope{})
		return "Exploring\n" + line
	case "edit":
		line := editLine(toolName, detail, toolResultEnvelope{})
		return line
	default:
		if detail == "" {
			detail = toolName
		}
		return "Running " + detail
	}
}

func completedToolTitle(toolName, raw string) string {
	env := parseToolEnvelope(raw)
	switch toolDisplayKind(toolName) {
	case "shell":
		cmd := strings.TrimSpace(asString(env.payload["command"]))
		if cmd == "" {
			cmd = "shell command"
		}
		return "Ran " + cmd
	case "explore":
		return "Explored\n" + explorationLine(toolName, "", env)
	case "edit":
		return editLine(toolName, "", env)
	default:
		label := toolName
		if label == "" {
			label = "tool"
		}
		return "Ran " + label
	}
}

func toolDisplayKind(toolName string) string {
	switch strings.TrimSpace(toolName) {
	case "exec_shell", "exec_shell_wait":
		return "shell"
	case "read_file", "list_dir", "search_files", "grep", "search_content", "fetch", "web_fetch", "web_search":
		return "explore"
	case "write_file", "edit_file", "apply_patch", "write", "edit":
		return "edit"
	default:
		return "unknown"
	}
}

func toolCallDetail(text string) string {
	t := strings.TrimSpace(text)
	if t == "" {
		return ""
	}
	if idx := strings.Index(t, ":"); idx >= 0 {
		t = strings.TrimSpace(t[idx+1:])
	}
	if strings.HasPrefix(t, "{") {
		var body map[string]any
		if err := json.Unmarshal([]byte(t), &body); err == nil {
			detail := firstNonEmpty(
				asString(body["command"]),
				asString(body["file_path"]),
				asString(body["path"]),
				asString(body["pattern"]),
				asString(body["query"]),
				asString(body["url"]),
				asString(body["task_id"]),
			)
			if detail != "" {
				return detail
			}
			// Fall back to showing what we can: first non-empty value
			for _, v := range body {
				if s := asString(v); strings.TrimSpace(s) != "" {
					return truncateDisplayText(s, 80)
				}
			}
			return ""
		}
	}
	return t
}

func explorationLine(toolName, fallback string, env toolResultEnvelope) string {
	payload := env.payload
	data := env.data
	switch toolName {
	case "read_file":
		path := firstNonEmpty(asString(payload["file_path"]), asString(data["file_path"]), fallback, "file")
		return "Read " + path
	case "list_dir":
		path := firstNonEmpty(asString(payload["path"]), asString(data["path"]), fallback, ".")
		return "List " + path
	case "search_files":
		return "Search " + firstNonEmpty(fallback, "files")
	case "grep", "search_content":
		return "Search " + firstNonEmpty(fallback, "content")
	case "fetch", "web_fetch":
		return "Fetch " + firstNonEmpty(asString(payload["url"]), asString(data["url"]), fallback, "url")
	case "web_search":
		return "Search web"
	default:
		return "Run " + firstNonEmpty(fallback, toolName)
	}
}

func editLine(toolName, fallback string, env toolResultEnvelope) string {
	payload := env.payload
	data := env.data
	switch toolName {
	case "write_file", "write":
		return "Edited " + firstNonEmpty(asString(payload["file_path"]), asString(data["file_path"]), fallback, "file")
	case "edit_file", "edit":
		return "Edited " + firstNonEmpty(asString(payload["file_path"]), asString(data["file_path"]), fallback, "file")
	case "apply_patch":
		files := stringSlice(firstNonEmptyAny(payload["files_changed"], data["files_changed"]))
		if len(files) == 1 {
			return "Edited " + files[0]
		}
		if len(files) > 1 {
			return fmt.Sprintf("Edited %d files", len(files))
		}
		return "Edited files"
	default:
		return "Edited " + firstNonEmpty(fallback, "files")
	}
}

func (m *model) syncModelEffortFromInfo(text string) {
	if strings.HasPrefix(text, "model: ") {
		m.model = strings.TrimSpace(strings.TrimPrefix(text, "model: "))
	}
	if strings.HasPrefix(text, "mode: ") {
		m.chatMode = chatModeDisplay(strings.TrimSpace(strings.TrimPrefix(text, "mode: ")))
	}
	if strings.HasPrefix(text, "effort: ") {
		m.effort = strings.TrimSpace(strings.TrimPrefix(text, "effort: "))
	}
	if strings.HasPrefix(text, "thinking: ") {
		m.thinking = strings.TrimSpace(strings.TrimPrefix(text, "thinking: "))
	}
	if strings.HasPrefix(text, "model set: ") {
		// format: model set: <m>  effort: <e>  thinking: <on|off>
		rest := strings.TrimSpace(strings.TrimPrefix(text, "model set: "))
		parts := strings.Split(rest, "  effort: ")
		if len(parts) == 2 {
			m.model = strings.TrimSpace(parts[0])
			right := strings.Split(parts[1], "  thinking: ")
			m.effort = strings.TrimSpace(right[0])
			if len(right) == 2 {
				m.thinking = strings.TrimSpace(right[1])
			}
		}
	}
	if strings.HasPrefix(text, "status: ") && strings.Contains(text, " thinking=") {
		parts := strings.Split(text, " thinking=")
		if len(parts) == 2 {
			m.thinking = strings.TrimSpace(parts[1])
		}
	}
	switch strings.TrimSpace(text) {
	case "Plan mode enabled":
		m.chatMode = "plan"
	case "Chat mode enabled":
		m.chatMode = "chat"
	}
}

func chatModeDisplay(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "plan":
		return "plan"
	default:
		return "chat"
	}
}

func visibleSubmittedText(value string) string {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "/plan ") {
		return value
	}
	payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "/plan"))
	if payload == "" || payload == "show" || payload == "on" || payload == "off" {
		return value
	}
	return payload
}

func (m *model) refreshViewportContent() {
	mainWidth, bodyHeight := m.layoutDims()
	m.viewport.Width = max(10, mainWidth-2)
	m.viewport.Height = max(1, bodyHeight-2)
	content := ""
	if m.page == pageLogs {
		content = strings.Join(m.filteredLogs(), "\n")
	}
	if m.page == pageDiff {
		content = strings.Join(m.renderDiffs(), "\n")
	}
	m.viewport.SetContent(content)
}

func (m model) renderChatLines(width int) []string {
	if m.assembler == nil {
		return nil
	}
	return tuirender.ChatLines(m.assembler.Snapshot(), width)
}

func (m model) scrollbackText(messages []tuirender.UIMessage) string {
	lines := tuirender.ChatLines(messages, m.chatRenderWidth())
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

func (m model) commitMessagesScrollbackCmd(messages []tuirender.UIMessage) tea.Cmd {
	text := m.scrollbackText(messages)
	if strings.TrimSpace(text) == "" {
		return nil
	}
	return tea.Println(text + "\n")
}

func (m model) commitMessageScrollbackCmd(role, text string) tea.Cmd {
	return m.commitMessagesScrollbackCmd([]tuirender.UIMessage{{
		Role: role,
		Kind: tuirender.KindText,
		Text: text,
	}})
}

func (m model) approvalNoticeText(decision string) string {
	action := approvalNoticeAction(m.approval.reason)
	product := strings.ToLower(strings.TrimSpace(m.product))
	if product == "" {
		product = "whale"
	}
	switch decision {
	case "allow":
		return lipgloss.NewStyle().
			Foreground(tuitheme.Default.Success).
			Render(fmt.Sprintf("✔ You approved %s to %s this time", product, action))
	case "allow_session":
		return lipgloss.NewStyle().
			Foreground(tuitheme.Default.Success).
			Render(fmt.Sprintf("✔ You approved %s to %s for this session", product, action))
	default:
		return lipgloss.NewStyle().
			Foreground(tuitheme.Default.Error).
			Render(fmt.Sprintf("✗ You canceled the request to %s", action))
	}
}

func approvalNoticeAction(summary string) string {
	summary = truncateLine(strings.TrimSpace(summary), 140)
	if summary == "" {
		return "use the requested tool"
	}
	if cmd, ok := strings.CutPrefix(summary, "exec_shell:"); ok {
		cmd = strings.TrimSpace(cmd)
		if cmd != "" {
			return "run " + cmd
		}
	}
	return "use " + summary
}

func (m model) turnInterruptedNoticeText() string {
	return lipgloss.NewStyle().
		Foreground(tuitheme.Default.Error).
		Bold(true).
		Render("■ Conversation interrupted - tell the model what to do differently.")
}

func (m *model) commitLiveScrollbackCmd() tea.Cmd {
	if m.assembler == nil {
		return nil
	}
	cmd := m.commitMessagesScrollbackCmd(m.assembler.Snapshot())
	m.assembler.Reset()
	return cmd
}
