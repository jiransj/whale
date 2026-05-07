package core

import "time"

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type FinishReason string

const (
	FinishReasonEndTurn  FinishReason = "end_turn"
	FinishReasonToolUse  FinishReason = "tool_use"
	FinishReasonCanceled FinishReason = "canceled"
	FinishReasonError    FinishReason = "error"
)

type Message struct {
	ID           string
	SessionID    string
	Role         Role
	Text         string
	Hidden       bool
	Reasoning    string
	ToolCalls    []ToolCall
	ToolResults  []ToolResult
	FinishReason FinishReason
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type ToolCall struct {
	ID    string
	Name  string
	Input string
}

type ToolResult struct {
	ToolCallID string
	Name       string
	Content    string
	Metadata   map[string]any `json:"metadata,omitempty"`
	IsError    bool
}
