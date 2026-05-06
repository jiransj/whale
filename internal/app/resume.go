package app

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

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
	for i, s := range summaries {
		marker := " "
		if s.ID == a.sessionID {
			marker = "*"
		}
		out = append(out, fmt.Sprintf("%s %2d) %s  updated:%s  branch:%s", marker, i+1, s.ID, s.ModTime.Local().Format("2006-01-02 15:04:05"), s.Meta.Branch))
	}
	return out, nil
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
