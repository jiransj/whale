package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	appcommands "github.com/usewhale/whale/internal/app/commands"
	"github.com/usewhale/whale/internal/compact"
	"github.com/usewhale/whale/internal/core"
	whalemcp "github.com/usewhale/whale/internal/mcp"
	"github.com/usewhale/whale/internal/memory"
	"github.com/usewhale/whale/internal/policy"
	"github.com/usewhale/whale/internal/session"
	"github.com/usewhale/whale/internal/skills"
	"github.com/usewhale/whale/internal/store"
)

func resolveInitialSessionID(sessionsDir string) (string, error) {
	recent, err := store.MostRecentSessionID(sessionsDir)
	if err == nil && recent != "" {
		return recent, nil
	}
	return "default", nil
}

func newSessionID(now time.Time) string {
	return appcommands.NewSessionID(now)
}

func resolveCLIResumeID(args []string) (string, bool, error) {
	if len(args) == 0 {
		return "", false, nil
	}
	if args[0] != "resume" {
		return "", false, nil
	}
	if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
		return "", true, fmt.Errorf("usage: whale resume <id>")
	}
	return strings.TrimSpace(args[1]), true, nil
}

func handleCommand(line, currentSessionID string, now time.Time) (appcommands.Result, error) {
	return appcommands.Parse(line, currentSessionID, now)
}

func planPromptFromSlash(line string) (string, bool) {
	return appcommands.PlanPromptFromSlash(line)
}

func (a *App) buildStatus() string {
	parts := []string{
		"Status",
		"",
		fmt.Sprintf("- session: %s", a.sessionID),
		fmt.Sprintf("- mode: %s", modeDisplay(a.currentMode)),
		fmt.Sprintf("- approval: %s", approvalModeDisplay(a.approvalMode)),
		fmt.Sprintf("- model: %s", a.model),
		fmt.Sprintf("- effort: %s", a.reasoningEffort),
		fmt.Sprintf("- thinking: %s", onOff(a.thinkingEnabled)),
	}
	parts = append(parts, formatContextWindowStatus(a))
	if mcpLine := a.formatMCPStatusLine(); mcpLine != "" {
		parts = append(parts, mcpLine)
	}
	return strings.Join(parts, "\n")
}

func (a *App) formatMCPStatusLine() string {
	if a == nil || a.mcpManager == nil {
		return ""
	}
	states := a.mcpManager.States()
	if len(states) == 0 {
		return "- mcp: no configured servers"
	}
	connected := 0
	failed := 0
	starting := 0
	tools := 0
	for _, st := range states {
		switch {
		case st.Connected || st.Status == whalemcp.StatusConnected:
			connected++
			tools += st.Tools
		case st.Status == whalemcp.StatusStarting:
			starting++
		case st.Error != "" || st.Status == whalemcp.StatusFailed || st.Status == whalemcp.StatusCancelled:
			failed++
		}
	}
	if starting > 0 {
		return fmt.Sprintf("- mcp: %d server(s), %d connected, %d starting, %d failed, %d tool(s)", len(states), connected, starting, failed, tools)
	}
	return fmt.Sprintf("- mcp: %d server(s), %d connected, %d failed, %d tool(s)", len(states), connected, failed, tools)
}

func (a *App) buildMCPStatus() string {
	if a == nil || a.mcpManager == nil {
		return "MCP\n\nconfig: unavailable\nservers: none"
	}
	lines := []string{"MCP", "", fmt.Sprintf("config: %s", a.mcpManager.ConfigPath())}
	states := a.mcpManager.States()
	if len(states) == 0 {
		lines = append(lines, "servers: none")
		return strings.Join(lines, "\n")
	}
	lines = append(lines, fmt.Sprintf("servers: %d", len(states)))
	for _, st := range states {
		status := st.Status
		if status == "" {
			status = "disabled"
			if st.Connected {
				status = "connected"
			} else if st.Error != "" {
				status = "failed"
			}
		}
		line := fmt.Sprintf("- %s: %s", st.Name, status)
		if st.Tools > 0 {
			line += fmt.Sprintf(" (%d tool(s))", st.Tools)
		}
		lines = append(lines, line)
		if st.Error != "" {
			lines = append(lines, "  error: "+st.Error)
		}
	}
	return strings.Join(lines, "\n")
}

func modeDisplay(mode session.Mode) string {
	if mode == session.ModeAsk {
		return "ask"
	}
	if mode == session.ModePlan {
		return "plan"
	}
	return "agent"
}

func modeTitle(mode session.Mode) string {
	if mode == session.ModeAsk {
		return "Ask"
	}
	if mode == session.ModePlan {
		return "Plan"
	}
	return "Agent"
}

func approvalModeDisplay(mode policy.ApprovalMode) string {
	switch mode {
	case policy.ApprovalModeNever:
		return "auto approve"
	default:
		return "ask first"
	}
}

func formatContextWindowStatus(a *App) string {
	msgs, err := a.msgStore.List(a.ctx, a.sessionID)
	if err != nil {
		return "- context window: unavailable"
	}
	used := compact.EstimateMessagesTokens(msgs)
	window := a.cfg.ContextWindow
	if window < 1 {
		window = 1
	}
	leftPct := 100 - (used*100)/window
	if leftPct < 0 {
		leftPct = 0
	}
	return fmt.Sprintf("- context window: %d%% left (%s used / %s)", leftPct, formatTokenCount(used), formatTokenCount(window))
}

func formatTokenCount(v int) string {
	if v >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(v)/1_000_000.0)
	}
	if v >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(v)/1_000.0)
	}
	return fmt.Sprintf("%d", v)
}

func (a *App) buildContext() (string, error) {
	msgs, err := a.msgStore.List(a.ctx, a.sessionID)
	if err != nil {
		return "", err
	}
	est := compact.EstimateMessagesTokens(msgs)
	window := a.cfg.ContextWindow
	if window < 1 {
		window = 1
	}
	usage := (est * 100) / window
	left := 100 - usage
	if left < 0 {
		left = 0
	}
	roleCount := map[core.Role]int{}
	for _, m := range msgs {
		roleCount[m.Role]++
	}
	lines := []string{
		"Context",
		"",
		fmt.Sprintf("- messages: %d", len(msgs)),
		fmt.Sprintf("- estimated tokens: %s", formatTokenCount(est)),
		fmt.Sprintf("- context window: %s", formatTokenCount(window)),
		fmt.Sprintf("- usage: %d%% used (%d%% left)", usage, left),
		fmt.Sprintf("- roles: user=%d assistant=%d tool=%d system=%d", roleCount[core.RoleUser], roleCount[core.RoleAssistant], roleCount[core.RoleTool], roleCount[core.RoleSystem]),
		"",
		"- hint: use /compact to summarize long history if needed",
	}
	return strings.Join(lines, "\n"), nil
}

func (a *App) initMemory() (string, error) {
	path := filepath.Join(a.workspaceRoot, "AGENTS.md")
	if _, err := os.Stat(path); err == nil {
		return fmt.Sprintf("AGENTS.md already exists at %s. Skipping /init to avoid overwriting it.", path), nil
	}
	return "", nil
}

func buildInitSyntheticPrompt() string {
	return strings.TrimSpace(`Generate a file named AGENTS.md that serves as a contributor guide for this repository.
Your goal is to produce a clear, concise, and well-structured document with descriptive headings and actionable explanations for each section.
Follow the outline below, but adapt as needed — add sections if relevant, and omit those that do not apply to this project.

Document Requirements

- Title the document "Repository Guidelines".
- Use Markdown headings (#, ##, etc.) for structure.
- Keep the document concise. 200-400 words is optimal.
- Keep explanations short, direct, and specific to this repository.
- Provide examples where helpful (commands, directory paths, naming patterns).
- Maintain a professional, instructional tone.

Recommended Sections

Project Structure & Module Organization

- Outline the project structure, including where the source code, tests, and assets are located.

Build, Test, and Development Commands

- List key commands for building, testing, and running locally (e.g., npm test, make build).
- Briefly explain what each command does.

Coding Style & Naming Conventions

- Specify indentation rules, language-specific style preferences, and naming patterns.
- Include any formatting or linting tools used.

Testing Guidelines

- Identify testing frameworks and coverage requirements.
- State test naming conventions and how to run tests.

Commit & Pull Request Guidelines

- Summarize commit message conventions found in the project’s Git history.
- Outline pull request requirements (descriptions, linked issues, screenshots, etc.).

(Optional) Add other sections if relevant, such as Security & Configuration Tips, Architecture Overview, or Agent-Specific Instructions.`)
}

func (a *App) showMemory() string {
	order := parseCSVList(a.cfg.MemoryFileOrder)
	pm, ok := memory.ReadProjectMemory(a.workspaceRoot, order, a.cfg.MemoryMaxChars)
	if !ok {
		return "memory: no project memory file found"
	}
	return fmt.Sprintf("memory: path=%s chars=%d truncated=%v", pm.Path, pm.OriginalChars, pm.Truncated)
}

func (a *App) buildSkillsList() string {
	roots := skills.DefaultRoots(a.workspaceRoot)
	discovered := skills.Discover(roots)
	lines := []string{"Skills", ""}
	if len(discovered) == 0 {
		lines = append(lines, "no skills found", "", "roots:")
		for _, root := range roots {
			lines = append(lines, "- "+root)
		}
		return strings.Join(lines, "\n")
	}
	for _, skill := range discovered {
		line := fmt.Sprintf("- %s: %s", skill.Name, skill.Description)
		if skill.SkillFilePath != "" {
			line += fmt.Sprintf(" (%s)", skill.SkillFilePath)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (a *App) buildSkillSyntheticPrompt(name, args string) (string, string, error) {
	name = strings.TrimSpace(name)
	args = strings.TrimSpace(args)
	if !skills.ValidName(name) {
		return "", "", fmt.Errorf("skill name must be alphanumeric with hyphens")
	}
	roots := skills.DefaultRoots(a.workspaceRoot)
	skill, _, ok := skills.Find(roots, name)
	if !ok {
		available := skills.Discover(roots)
		names := make([]string, 0, len(available))
		for _, s := range available {
			names = append(names, s.Name)
		}
		msg := fmt.Sprintf("skill not found: %s", name)
		if len(names) > 0 {
			msg += ". available skills: " + strings.Join(names, ", ")
		}
		return "", "", fmt.Errorf("%s", msg)
	}
	var b strings.Builder
	b.WriteString("Use this skill for the current turn.\n\n")
	b.WriteString("<skill>\n")
	b.WriteString("<name>")
	b.WriteString(skill.Name)
	b.WriteString("</name>\n")
	b.WriteString("<description>")
	b.WriteString(skill.Description)
	b.WriteString("</description>\n")
	b.WriteString("<path>")
	b.WriteString(skill.SkillFilePath)
	b.WriteString("</path>\n")
	if args != "" {
		b.WriteString("<arguments>\n")
		b.WriteString(args)
		b.WriteString("\n</arguments>\n")
	}
	b.WriteString("<instructions>\n")
	b.WriteString(skill.Instructions)
	b.WriteString("\n</instructions>\n")
	b.WriteString("</skill>")
	return "loaded skill: " + skill.Name, strings.TrimSpace(b.String()), nil
}

func (a *App) BuildSkillMentionSyntheticPrompt(line string) (bool, string, string, error) {
	name, args, ok := parseSkillMention(line)
	if !ok {
		return false, "", "", nil
	}
	out, synthetic, err := a.buildSkillSyntheticPrompt(name, args)
	if err != nil {
		return true, "", "", err
	}
	return true, out, synthetic, nil
}

func parseSkillMention(line string) (name, args string, ok bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "$") {
		return "", "", false
	}
	head := trimmed
	if idx := strings.IndexAny(trimmed, " \t\n"); idx >= 0 {
		head = trimmed[:idx]
		args = strings.TrimSpace(trimmed[idx:])
	}
	name = strings.TrimPrefix(head, "$")
	if !skills.ValidName(name) {
		return "", "", false
	}
	return name, args, true
}

func parseCSVList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
