package app

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/usewhale/whale/internal/session"
)

func (a *App) IsResumeMenu(line string) bool { return strings.TrimSpace(line) == "/resume" }

func (a *App) ListResumeChoices(limit int) ([]string, error) {
	summaries, err := session.ListSessions(a.sessionsDir, limit)
	if err != nil {
		return nil, err
	}
	if len(summaries) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(summaries)+1)
	out = append(out, "recent sessions:")
	out = append(out, "   #   Updated   Branch                    Conversation")
	for i, s := range summaries {
		marker := " "
		if s.ID == a.sessionID {
			marker = "*"
		}
		branch := strings.TrimSpace(s.Meta.Branch)
		if branch == "" {
			branch = "-"
		}
		out = append(out, fmt.Sprintf("%s %2d) %-9s %-24s %s", marker, i+1, humanAgo(s.ModTime), truncateRunes(branch, 24), truncateRunes(s.Conversation, 80)))
	}
	return out, nil
}

func humanAgo(ts time.Time) string {
	if ts.IsZero() {
		return "-"
	}
	d := time.Since(ts)
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

func truncateRunes(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	return string(runes[:max-1]) + "…"
}

func (a *App) ApplyResumeChoice(choice string) (string, error) {
	choice = strings.TrimSpace(choice)
	if choice == "" {
		return "resume canceled", nil
	}
	summaries, err := session.ListSessions(a.sessionsDir, 20)
	if err != nil {
		return "", err
	}
	next := ""
	if idx, err := strconv.Atoi(choice); err == nil {
		if idx < 1 || idx > len(summaries) {
			return "", errors.New("invalid selection")
		}
		next = summaries[idx-1].ID
	} else {
		next = choice
	}
	a.sessionID = next
	modeState, err := session.LoadModeState(a.sessionsDir, a.sessionID)
	if err != nil {
		return "", err
	}
	a.currentMode = modeState.Mode
	if _, err := session.PatchSessionMeta(a.sessionsDir, a.sessionID, session.SessionMeta{Workspace: a.workspaceRoot, Branch: a.branch}); err != nil {
		return "", err
	}
	out := fmt.Sprintf("resumed session: %s\nmode: %s", a.sessionID, a.currentMode)
	if ust, err := session.LoadUserInputState(a.sessionsDir, a.sessionID); err == nil && ust.Pending {
		out += fmt.Sprintf("\npending user input: tool_call=%s questions=%d", ust.ToolCallID, len(ust.Questions))
	}
	return out, nil
}
