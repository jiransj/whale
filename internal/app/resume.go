package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/usewhale/whale/internal/session"
)

func (a *App) IsResumeMenu(line string) bool { return strings.TrimSpace(line) == "/resume" }

type ResumeApplyResult struct {
	Message string
	Resumed bool
}

type CrossWorkspaceResumeError struct {
	Message string
}

func (e *CrossWorkspaceResumeError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func IsCrossWorkspaceResumeError(err error) bool {
	var target *CrossWorkspaceResumeError
	return errors.As(err, &target)
}

func CheckResumeWorkspace(sessionsDir, sessionID, currentWorkspace string) (string, bool, error) {
	meta, err := session.LoadSessionMeta(sessionsDir, sessionID)
	if err != nil {
		return "", false, err
	}
	workspace := strings.TrimSpace(meta.Workspace)
	if workspace == "" || sameWorkspace(workspace, currentWorkspace) {
		return "", false, nil
	}
	return crossWorkspaceResumeMessage(workspace, sessionID), true, nil
}

func crossWorkspaceResumeMessage(workspace, sessionID string) string {
	return strings.Join([]string{
		"This conversation is from a different directory.",
		"",
		"To resume, run:",
		"  " + resumeCommand(workspace, sessionID),
	}, "\n")
}

func resumeCommand(workspace, sessionID string) string {
	bin := resumeExecutable()
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("cd /d %s && %s resume %s", cmdQuote(workspace), cmdQuote(bin), sessionID)
	}
	return fmt.Sprintf("cd %s && %s resume %s", shQuote(workspace), shQuote(bin), sessionID)
}

func resumeExecutable() string {
	exe, err := os.Executable()
	if err != nil || strings.TrimSpace(exe) == "" {
		return "whale"
	}
	return exe
}

func sameWorkspace(a, b string) bool {
	a = normalizedWorkspacePath(a)
	b = normalizedWorkspacePath(b)
	if a == "" || b == "" {
		return false
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func normalizedWorkspacePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	return filepath.Clean(path)
}

func shQuote(v string) string {
	if v == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(v, "'", "'\"'\"'") + "'"
}

func cmdQuote(v string) string {
	return `"` + strings.ReplaceAll(v, `"`, `\"`) + `"`
}

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

func (a *App) ApplyResumeChoice(choice string) (ResumeApplyResult, error) {
	choice = strings.TrimSpace(choice)
	if choice == "" {
		return ResumeApplyResult{Message: "resume canceled"}, nil
	}
	summaries, err := session.ListSessions(a.sessionsDir, 20)
	if err != nil {
		return ResumeApplyResult{}, err
	}
	next := ""
	if idx, err := strconv.Atoi(choice); err == nil {
		if idx < 1 || idx > len(summaries) {
			return ResumeApplyResult{}, errors.New("invalid selection")
		}
		next = summaries[idx-1].ID
	} else {
		next = choice
	}
	if msg, blocked, err := CheckResumeWorkspace(a.sessionsDir, next, a.workspaceRoot); err != nil {
		return ResumeApplyResult{}, err
	} else if blocked {
		return ResumeApplyResult{Message: msg}, nil
	}
	a.sessionID = next
	modeState, err := session.LoadModeState(a.sessionsDir, a.sessionID)
	if err != nil {
		return ResumeApplyResult{}, err
	}
	a.currentMode = modeState.Mode
	out := fmt.Sprintf("resumed session: %s\nmode: %s", a.sessionID, a.currentMode)
	if ust, err := session.LoadUserInputState(a.sessionsDir, a.sessionID); err == nil && ust.Pending {
		out += fmt.Sprintf("\npending user input: tool_call=%s questions=%d", ust.ToolCallID, len(ust.Questions))
	}
	return ResumeApplyResult{Message: out, Resumed: true}, nil
}
