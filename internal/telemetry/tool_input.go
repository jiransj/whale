package telemetry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/usewhale/whale/internal/session"
)

const ToolInputEventsSuffix = ".tool_input_events.jsonl"

type ToolInputEvent struct {
	Session            string `json:"session"`
	Event              string `json:"event"`
	ToolCallID         string `json:"tool_call_id,omitempty"`
	ToolName           string `json:"tool_name,omitempty"`
	InputRaw           string `json:"input_raw,omitempty"`
	InputRunes         int    `json:"input_runes,omitempty"`
	CreatedAt          time.Time `json:"created_at,omitempty"`
	TS                 int64 `json:"ts,omitempty"`

	// Fields used by tool_input_telemetry.go for repair/invalid tracking
	Model              string `json:"model,omitempty"`
	AssistantMessageID string `json:"assistant_message_id,omitempty"`
	Tool               string `json:"tool,omitempty"`
	RepairKind         string `json:"repair_kind,omitempty"`
	Path               string `json:"path,omitempty"`
	BeforeType         string `json:"before_type,omitempty"`
	AfterType          string `json:"after_type,omitempty"`
	ErrorCode          string `json:"error_code,omitempty"`
}

func ToolInputEventsPath(sessionsDir, sessionID string) string {
	return filepath.Join(strings.TrimSpace(sessionsDir), session.SanitizeSessionID(sessionID)+ToolInputEventsSuffix)
}

func AppendToolInputEvent(sessionsDir string, rec ToolInputEvent, now time.Time) error {
	sessionsDir = strings.TrimSpace(sessionsDir)
	if sessionsDir == "" || strings.TrimSpace(rec.Session) == "" || strings.TrimSpace(rec.Event) == "" {
		return nil
	}
	rec.CreatedAt = now
	b, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal tool input event: %w", err)
	}
	path := ToolInputEventsPath(sessionsDir, rec.Session)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir tool input events dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open tool input events: %w", err)
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}
