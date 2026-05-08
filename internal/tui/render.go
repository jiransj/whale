package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/usewhale/whale/internal/build"
	tuitheme "github.com/usewhale/whale/internal/tui/theme"
)

func (m model) renderBody(mainWidth, bodyHeight int) string {
	if m.page != pageChat {
		return lipgloss.NewStyle().
			Width(mainWidth).
			Height(bodyHeight).
			Border(lipgloss.NormalBorder()).
			BorderForeground(tuitheme.Default.Border).
			Render(m.viewport.View())
	}
	return m.renderLiveArea(mainWidth, bodyHeight)
}

func (m model) renderLiveArea(width, bodyHeight int) string {
	if bodyHeight <= 0 {
		return ""
	}
	lines := m.renderChatLines(max(20, width-2))
	if len(lines) == 0 {
		return ""
	}
	maxLines := bodyHeight
	truncated := false
	if len(lines) > maxLines {
		truncated = true
		lines = lines[len(lines)-maxLines:]
	}
	if truncated {
		prefix := lipgloss.NewStyle().
			Foreground(tuitheme.Default.Muted).
			Render("... live output truncated; full turn will be added to scrollback when complete")
		lines = append([]string{prefix}, lines...)
	}
	return lipgloss.NewStyle().
		Width(width).
		Render(strings.TrimRight(strings.Join(lines, "\n"), "\n"))
}

func (m model) View() string {
	mainWidth, _ := m.layoutDims()
	m.refreshViewportContent()
	footerText := "model: " + m.model + "  effort: " + m.effort + "  thinking: " + m.thinking
	if m.chatMode == "ask" || m.chatMode == "plan" {
		footerText += "  mode: " + m.chatMode + " (Shift+Tab to switch)"
	}
	if m.cwd != "" {
		footerText = appendFooterDir(footerText, m.cwd, mainWidth)
	}
	footer := lipgloss.NewStyle().Width(mainWidth).MaxWidth(mainWidth).Render(lipgloss.JoinHorizontal(lipgloss.Left, footerText))
	bottomParts := make([]string, 0, 8)
	if statusLine := m.renderBusyStatusLine(mainWidth); statusLine != "" {
		bottomParts = append(bottomParts, statusLine)
	}
	if m.mode == modeChat && m.hasSlashSuggestions() {
		bottomParts = append(bottomParts, m.renderSlashSuggestions())
	}
	if m.mode == modeApproval {
		opts := []string{"Allow (a)", "Allow for Session (s)", "Deny (d)"}
		for i := range opts {
			if i == m.approval.selected {
				opts[i] = "[" + opts[i] + "]"
			}
		}
		approvalBody := m.approval.reason
		if diff := renderFileDiffMetadataPlain(m.approval.metadata, 80); diff != "" {
			approvalBody += "\n\n" + diff
		}
		approval := lipgloss.NewStyle().Foreground(tuitheme.Default.Error).Render(
			fmt.Sprintf(
				"approval: %s\n%s\n\n%s\n(←/→/tab select, enter confirm, esc deny)",
				m.approval.toolName,
				approvalBody,
				strings.Join(opts, "   "),
			),
		)
		bottomParts = append(bottomParts, approval)
	}
	if m.mode == modePlanImplementation {
		bottomParts = append(bottomParts, m.renderPlanImplementationPicker())
	}
	if m.mode == modeSessionPicker {
		rows := []string{"sessions (↑/↓ select, enter confirm, esc cancel):"}
		for i, row := range m.sessionChoices {
			if isSessionHeaderRow(row) {
				rows = append(rows, row)
				continue
			}
			prefix := "  "
			if i == m.sessionIndex {
				prefix = "> "
			}
			rows = append(rows, prefix+displaySessionChoiceRow(row))
		}
		bottomParts = append(bottomParts, lipgloss.NewStyle().Foreground(tuitheme.Default.Plan).Render(strings.Join(rows, "\n")))
	}
	if m.mode == modeUserInput {
		if m.userInput.index < len(m.userInput.questions) {
			q := m.userInput.questions[m.userInput.index]
			rows := make([]string, 0, len(q.Options)+3)
			rows = append(rows, q.Question)
			rows = append(rows, "")
			for i, opt := range q.Options {
				prefix := "  "
				if i == m.userInput.selectedOption {
					prefix = "> "
				}
				rows = append(rows, fmt.Sprintf("%s%s - %s", prefix, opt.Label, opt.Description))
			}
			rows = append(rows, "", "(up/down choose, enter confirm, esc cancel)")
			bottomParts = append(bottomParts, lipgloss.NewStyle().Foreground(tuitheme.Default.Info).Render(strings.Join(rows, "\n")))
		}
	}
	if m.mode == modeModelPicker {
		bottomParts = append(bottomParts, m.renderModelPicker())
	}
	if m.mode == modePermissionsPicker {
		bottomParts = append(bottomParts, m.renderPermissionsPicker())
	}
	bottomParts = append(bottomParts, m.input.View(), footer)
	bottom := strings.Join(bottomParts, "\n")

	bodyHeight := m.height - countVisibleLines(bottom)
	if m.height <= 0 {
		bodyHeight = 0
	}
	bodyHeight = max(0, bodyHeight)
	body := m.renderBody(mainWidth, bodyHeight)
	body = padVisibleLines(body, bodyHeight, mainWidth)
	if body == "" {
		return bottom
	}
	return body + "\n" + bottom
}

func countVisibleLines(s string) int {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func padVisibleLines(s string, targetLines, width int) string {
	if targetLines <= 0 {
		return ""
	}
	s = strings.TrimRight(s, "\n")
	lines := []string{}
	if s != "" {
		lines = strings.Split(s, "\n")
	}
	if len(lines) > targetLines {
		lines = lines[len(lines)-targetLines:]
	}
	for len(lines) < targetLines {
		lines = append(lines, "")
	}
	style := lipgloss.NewStyle().Width(width).MaxWidth(width)
	for i, line := range lines {
		lines[i] = style.Render(line)
	}
	return strings.Join(lines, "\n")
}

func appendFooterDir(base, cwd string, width int) string {
	segment := "  "
	available := width - lipgloss.Width(base) - lipgloss.Width(segment)
	if available <= 0 {
		return base
	}
	return base + segment + fitTail(cwd, available)
}

func fitTail(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	if width <= 3 {
		return strings.Repeat(".", width)
	}
	runes := []rune(s)
	tail := ""
	for i := len(runes) - 1; i >= 0; i-- {
		next := string(runes[i:])
		if lipgloss.Width("..."+next) > width {
			break
		}
		tail = next
	}
	return "..." + tail
}

func (m model) renderBusyStatusLine(width int) string {
	if !m.busy {
		return ""
	}
	label := "Working"
	if m.stopping {
		label = "Stopping"
	}
	line := fmt.Sprintf("%s (%s)", label, formatElapsedCompact(m.busyElapsed()))
	return lipgloss.NewStyle().
		Width(width).
		Foreground(tuitheme.Default.Warn).
		Render(line)
}

func (m model) busyElapsed() time.Duration {
	if m.busySince.IsZero() {
		return 0
	}
	return time.Since(m.busySince)
}

func formatElapsedCompact(elapsed time.Duration) string {
	seconds := int(elapsed / time.Second)
	if seconds < 0 {
		seconds = 0
	}
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		minutes := seconds / 60
		remSeconds := seconds % 60
		return fmt.Sprintf("%dm %02ds", minutes, remSeconds)
	}
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	remSeconds := seconds % 60
	return fmt.Sprintf("%dh %02dm %02ds", hours, minutes, remSeconds)
}

func resolveVersion() string {
	return build.CurrentVersion()
}

func buildHeaderBanner(modelName, effort, cwd, version string) string {
	return fmt.Sprintf("▸ Whale %s   model: %s %s   dir: %s",
		version, modelName, effort, cwd)
}

func resolveWorkingDirectory() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	home, hErr := os.UserHomeDir()
	if hErr == nil {
		if rel, rErr := filepath.Rel(home, wd); rErr == nil && rel != "" && rel != "." && !strings.HasPrefix(rel, "..") {
			return "~/" + rel
		}
		if filepath.Clean(wd) == filepath.Clean(home) {
			return "~"
		}
	}
	return wd
}

func (m model) pageLabel() string {
	if m.page == pageLogs {
		return "logs"
	}
	if m.page == pageDiff {
		return "diff"
	}
	return "chat"
}

func (m model) renderPalette() string {
	rows := []string{"Command Palette (enter to run, esc to close)"}
	for i, it := range m.palette.actions {
		prefix := "  "
		if i == m.palette.selected {
			prefix = "> "
		}
		rows = append(rows, prefix+it.Label)
	}
	return lipgloss.NewStyle().Foreground(tuitheme.Default.Palette).Render(strings.Join(rows, "\n"))
}

func (m model) renderModelPicker() string {
	rows := []string{"Select Model and Effort"}
	rows = append(rows, "")
	rows = append(rows, "Model:")
	for i, item := range m.modelPicker.models {
		prefix := "  "
		if m.modelPicker.stage == 0 && i == m.modelPicker.modelIx {
			prefix = "> "
		}
		rows = append(rows, prefix+item)
	}
	if m.modelPicker.stage >= 1 {
		rows = append(rows, "")
		rows = append(rows, "Effort:")
		for i, item := range m.modelPicker.efforts {
			prefix := "  "
			if m.modelPicker.stage == 1 && i == m.modelPicker.effIx {
				prefix = "> "
			}
			rows = append(rows, prefix+item)
		}
	}
	if m.modelPicker.stage >= 2 {
		rows = append(rows, "", "Thinking:")
		for i, item := range m.modelPicker.thinkings {
			prefix := "  "
			if m.modelPicker.stage == 2 && i == m.modelPicker.thinkIx {
				prefix = "> "
			}
			rows = append(rows, prefix+item)
		}
	}
	rows = append(rows, "", "(up/down choose, enter next/confirm, esc back)")
	return lipgloss.NewStyle().Foreground(tuitheme.Default.Info).Render(strings.Join(rows, "\n"))
}

func (m model) renderPermissionsPicker() string {
	rows := []string{"Permissions", ""}
	descriptions := map[string]string{
		"Ask first":    "Ask before write, patch, or shell tools run.",
		"Auto approve": "Never ask; auto-approve tool calls.",
	}
	for i, item := range m.permissionsPicker.choices {
		prefix := "  "
		if i == m.permissionsPicker.index {
			prefix = "> "
		}
		if desc := descriptions[item]; desc != "" {
			rows = append(rows, fmt.Sprintf("%s%s - %s", prefix, item, desc))
		} else {
			rows = append(rows, prefix+item)
		}
	}
	rows = append(rows, "", "(up/down choose, enter confirm, esc cancel)")
	return lipgloss.NewStyle().Foreground(tuitheme.Default.Info).Render(strings.Join(rows, "\n"))
}

func (m model) renderPlanImplementationPicker() string {
	rows := []string{"Implement this plan?", ""}
	items := []struct {
		label string
	}{
		{"Yes, implement this plan"},
		{"No, stay in Plan mode"},
	}
	for i, item := range items {
		prefix := "  "
		if i == m.planImplementation.index {
			prefix = "> "
		}
		rows = append(rows, prefix+item.label)
	}
	rows = append(rows, "", "(up/down choose, enter confirm, esc cancel)")
	return lipgloss.NewStyle().Foreground(tuitheme.Default.Info).Render(strings.Join(rows, "\n"))
}

func (m model) layoutDims() (mainWidth, bodyHeight int) {
	bodyHeight = max(3, m.height-6)
	mainWidth = m.width
	if m.sidebar && m.width > 80 {
		mainWidth = int(float64(m.width) * 0.72)
	}
	return mainWidth, bodyHeight
}

func (m model) chatRenderWidth() int {
	mainWidth, _ := m.layoutDims()
	return max(20, max(10, mainWidth-2))
}
