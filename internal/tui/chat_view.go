package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	tuirender "github.com/usewhale/whale/internal/tui/render"
	tuitheme "github.com/usewhale/whale/internal/tui/theme"
)

func (m *model) append(role, text string) {
	if m.assembler == nil {
		m.assembler = tuirender.NewAssembler()
	}
	m.assembler.AppendDelta(role, text)
	m.refreshViewportContentFollow(false)
}

func (m *model) appendNotice(text string) {
	if m.assembler == nil {
		m.assembler = tuirender.NewAssembler()
	}
	m.assembler.AddNotice(text)
	m.refreshViewportContentFollow(false)
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
	m.refreshViewportContentFollow(false)
}

func (m *model) appendToolCall(toolCallID, toolName, text string) {
	if m.assembler == nil {
		m.assembler = tuirender.NewAssembler()
	}
	m.assembler.AddToolCall(toolCallID, summarizeToolCallForChat(toolName, text))
	m.refreshViewportContentFollow(false)
}

func (m *model) updateToolCallFromResult(toolCallID, toolName, result, role, summary string, metadata map[string]any) bool {
	if toolCallID == "" {
		return false
	}
	previous := ""
	if m.assembler != nil {
		previous = m.assembler.ToolCallText(toolCallID)
	}
	title := completedToolTitle(toolName, result, previous)
	if summary != "" && summary != "✓" {
		title += "\n" + summary
	}
	if diff := renderFileDiffMetadataMarkdown(metadata, 80); diff != "" && role == "result_ok" {
		title += "\n\n" + diff
	}
	ok := m.assembler.UpdateToolCall(toolCallID, title, role)
	if ok {
		m.refreshViewportContentFollow(false)
	}
	return ok
}

func (m *model) updateTaskProgress(toolCallID, toolName, text string) bool {
	if toolCallID == "" || m.assembler == nil {
		return false
	}
	title := summarizeTaskProgressForChat(toolName, text)
	ok := m.assembler.UpdateToolCall(toolCallID, title, "result_running")
	if ok {
		m.refreshViewportContentFollow(false)
	}
	return ok
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
	case "task":
		if strings.TrimSpace(toolName) == "parallel_reason" {
			return "Parallel reasoning\n" + firstNonEmpty(detail, "working")
		}
		role := taskRoleFromText(text)
		return "Subagent " + role + "\n" + firstNonEmpty(detail, "starting")
	default:
		if detail == "" {
			detail = toolName
		}
		return "Running " + detail
	}
}

func summarizeTaskProgressForChat(toolName, text string) string {
	detail := taskProgressDetail(text)
	if strings.TrimSpace(toolName) == "parallel_reason" {
		return "Parallel reasoning\n" + firstNonEmpty(detail, "running")
	}
	role := taskRoleFromText(text)
	if taskProgressFailed(text) {
		return "Subagent " + role + "\nFailed: " + firstNonEmpty(detail, "subagent failed")
	}
	return "Subagent " + role + "\n" + firstNonEmpty(detail, "running")
}

func taskProgressDetail(text string) string {
	t := strings.TrimSpace(text)
	if t == "" {
		return ""
	}
	parts := strings.SplitN(t, "·", 3)
	if len(parts) == 3 {
		return strings.TrimSpace(parts[2])
	}
	parts = strings.Split(t, "·")
	return strings.TrimSpace(parts[len(parts)-1])
}

func taskRoleFromText(text string) string {
	t := strings.TrimSpace(text)
	if before, _, ok := strings.Cut(t, "·"); ok {
		if _, role, hasColon := strings.Cut(before, ":"); hasColon {
			role = strings.TrimSpace(role)
			if role != "" {
				return role
			}
		}
	}
	parts := strings.Split(t, "·")
	if len(parts) >= 2 {
		role := strings.TrimSpace(parts[1])
		if role != "" && !strings.Contains(role, "prompt") {
			return role
		}
	}
	return "explore"
}

func taskProgressFailed(text string) bool {
	first := strings.TrimSpace(strings.SplitN(text, "·", 2)[0])
	return strings.Contains(first, "failed")
}

func completedToolTitle(toolName, raw, previous string) string {
	env := parseToolEnvelope(raw)
	switch toolDisplayKind(toolName) {
	case "shell":
		cmd := strings.TrimSpace(asString(env.payload["command"]))
		if cmd == "" {
			cmd = "shell command"
		}
		return "Ran " + cmd
	case "explore":
		return "Explored\n" + explorationLine(toolName, previousToolActionLine(previous), env)
	case "edit":
		return editLine(toolName, "", env)
	case "task":
		if toolName == "parallel_reason" {
			return "Parallel reasoning"
		}
		role := firstNonEmpty(asString(env.data["role"]), "explore")
		return "Subagent " + role
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
	case "parallel_reason", "spawn_subagent":
		return "task"
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
		return formatSearchActionLine(firstNonEmpty(searchDetailFromPayload(payload, data, ""), fallback), "files")
	case "grep", "search_content":
		return formatSearchActionLine(firstNonEmpty(searchDetailFromPayload(payload, data, asString(payload["include"])), fallback), "content")
	case "fetch", "web_fetch":
		return "Fetch " + firstNonEmpty(asString(payload["url"]), asString(data["url"]), fallback, "url")
	case "web_search":
		query := firstNonEmpty(webSearchQueryFromMaps(payload, data), fallback)
		if query != "" {
			return "Search web for " + query
		}
		return "Search web"
	default:
		return "Run " + firstNonEmpty(fallback, toolName)
	}
}

func searchDetailFromPayload(payload, data map[string]any, includeFallback string) string {
	pattern := firstNonEmpty(asString(payload["pattern"]), asString(data["pattern"]))
	path := firstNonEmpty(asString(payload["path"]), asString(data["path"]))
	include := firstNonEmpty(asString(payload["include"]), asString(data["include"]), includeFallback)
	return appendSearchDetail(pattern, path, include)
}

func webSearchQueryFromMaps(payload, data map[string]any) string {
	return firstNonEmpty(asString(payload["query"]), asString(data["query"]))
}

func formatSearchActionLine(detail, fallback string) string {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		detail = fallback
	}
	if strings.HasPrefix(detail, "Search ") {
		return detail
	}
	return "Search " + detail
}

func appendSearchDetail(subject, path, include string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return ""
	}
	if strings.TrimSpace(path) != "" {
		subject += " in " + strings.TrimSpace(path)
	}
	if strings.TrimSpace(include) != "" {
		subject += " (" + strings.TrimSpace(include) + ")"
	}
	return subject
}

func previousToolActionLine(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) >= 2 {
		return strings.TrimSpace(lines[1])
	}
	if len(lines) == 1 {
		return strings.TrimSpace(lines[0])
	}
	return ""
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
	case "Ask mode enabled":
		m.chatMode = "ask"
	case "Agent mode enabled":
		m.chatMode = "agent"
	}
}

func chatModeDisplay(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "ask":
		return "ask"
	case "plan":
		return "plan"
	default:
		return "agent"
	}
}

func visibleSubmittedText(value string) string {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(trimmed, "/ask ") {
		payload := strings.TrimSpace(strings.TrimPrefix(trimmed, "/ask"))
		if payload != "" {
			return payload
		}
	}
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

func (m model) busySubmitNoticeText() string {
	return lipgloss.NewStyle().
		Foreground(tuitheme.Default.Warn).
		Bold(true).
		Render("■ Agent is working, please wait...")
}

func (m *model) resetTranscriptWithHeader() {
	m.transcript = nil
	m.nativeScrollbackPrinted = 0
	m.appendTranscript("info", tuirender.KindText, buildHeaderBanner(m.model, m.effort, m.cwd, m.version))
	m.nativeScrollbackPrinted = len(m.transcript)
}

func (m *model) appendTranscript(role string, kind tuirender.MessageKind, text string) {
	t := strings.TrimSpace(strings.TrimRight(text, "\n"))
	if t == "" {
		return
	}
	if kind == "" {
		kind = tuirender.KindText
	}
	m.transcript = append(m.transcript, tuirender.UIMessage{
		Role: role,
		Kind: kind,
		Text: t,
	})
	m.refreshViewportContentFollow(true)
}

func (m *model) appendTranscriptMessages(messages []tuirender.UIMessage) {
	for _, msg := range messages {
		if strings.TrimSpace(msg.Text) == "" {
			continue
		}
		m.transcript = append(m.transcript, msg)
	}
}

func (m *model) commitLiveTranscript(forceBottom bool) {
	if m.assembler == nil {
		return
	}
	m.appendTranscriptMessages(m.assembler.Snapshot())
	m.assembler.Reset()
	m.refreshViewportContentFollow(forceBottom)
}

const maxHydratedTranscriptLines = 300

func (m *model) trimHydratedTranscriptForDisplay(maxLines int) {
	if maxLines <= 0 || len(m.transcript) <= 1 {
		return
	}
	header := m.transcript[0]
	messages := m.transcript[1:]
	width := m.chatRenderWidth()
	selected := make([]tuirender.UIMessage, 0, len(messages))
	lineCount := 0
	for i := len(messages) - 1; i >= 0; i-- {
		msgLines := len(tuirender.ChatLines([]tuirender.UIMessage{messages[i]}, width))
		if len(selected) > 0 && lineCount+msgLines > maxLines {
			break
		}
		lineCount += msgLines
		selected = append(selected, messages[i])
	}
	for i, j := 0, len(selected)-1; i < j; i, j = i+1, j-1 {
		selected[i], selected[j] = selected[j], selected[i]
	}
	m.transcript = append([]tuirender.UIMessage{header}, selected...)
	m.refreshViewportContentFollow(true)
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
