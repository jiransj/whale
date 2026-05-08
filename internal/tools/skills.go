package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/usewhale/whale/internal/core"
	"github.com/usewhale/whale/internal/skills"
)

func (b *Toolset) loadSkill(_ context.Context, call core.ToolCall) (core.ToolResult, error) {
	var in struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}
	if err := decodeInput(call.Input, &in); err != nil {
		return marshalToolError(call, "invalid_args", err.Error()), nil
	}
	name := strings.TrimSpace(in.Name)
	if !skills.ValidName(name) {
		return marshalToolError(call, "invalid_args", "skill name must be alphanumeric with hyphens"), nil
	}
	roots := skills.DefaultRoots(b.root)
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
		return marshalToolError(call, "not_found", msg), nil
	}
	content, trunc := truncateTextSmart(skill.Instructions, maxToolTextChars)
	payload := map[string]any{
		"name":         skill.Name,
		"description":  skill.Description,
		"path":         skill.Path,
		"skill_file":   skill.SkillFilePath,
		"instructions": content,
		"arguments":    strings.TrimSpace(in.Arguments),
		"truncation":   trunc,
		"read_only":    true,
		"execution":    "not_executed",
		"usage_hint":   "Follow these instructions for the current task. This tool only loads skill instructions; it does not execute scripts or modify files.",
	}
	return marshalToolResult(call, map[string]any{
		"status":  "ok",
		"payload": payload,
		"summary": "loaded skill: " + skill.Name,
	})
}
