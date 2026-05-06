package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type ToolRegistry struct {
	byName         map[string]Tool
	specs          map[string]ToolSpec
	ordered        []Tool
	maxResultChars int
}

func NewToolRegistry(tools []Tool) *ToolRegistry {
	r, err := NewToolRegistryChecked(tools)
	if err != nil {
		panic(err)
	}
	return r
}

func NewToolRegistryChecked(tools []Tool) (*ToolRegistry, error) {
	r := &ToolRegistry{
		byName:         make(map[string]Tool, len(tools)),
		specs:          make(map[string]ToolSpec, len(tools)),
		ordered:        make([]Tool, 0, len(tools)),
		maxResultChars: 24 * 1024,
	}
	for _, t := range tools {
		if t == nil {
			continue
		}
		name := t.Name()
		if name == "" {
			continue
		}
		if _, ok := r.byName[name]; !ok {
			r.ordered = append(r.ordered, t)
		}
		r.byName[name] = t
		spec := DescribeTool(t)
		spec.Parameters = normalizeToolSchema(spec.Parameters)
		if !isValidToolSpec(spec) {
			return nil, fmt.Errorf("invalid tool spec for %q", name)
		}
		r.specs[name] = spec
	}
	return r, nil
}

func (r *ToolRegistry) Get(name string) Tool {
	if r == nil {
		return nil
	}
	return r.byName[name]
}

func (r *ToolRegistry) Tools() []Tool {
	if r == nil {
		return nil
	}
	out := make([]Tool, 0, len(r.ordered))
	out = append(out, r.ordered...)
	return out
}

func (r *ToolRegistry) Specs() []ToolSpec {
	if r == nil {
		return nil
	}
	out := make([]ToolSpec, 0, len(r.ordered))
	for _, t := range r.ordered {
		out = append(out, r.specs[t.Name()])
	}
	return out
}

func (r *ToolRegistry) Spec(name string) (ToolSpec, bool) {
	if r == nil {
		return ToolSpec{}, false
	}
	spec, ok := r.specs[name]
	return spec, ok
}

func (r *ToolRegistry) SetMaxResultChars(limit int) {
	if r == nil {
		return
	}
	r.maxResultChars = limit
}

func (r *ToolRegistry) Dispatch(ctx context.Context, call ToolCall) (ToolResult, error) {
	start := time.Now()
	spec, hasSpec := r.Spec(call.Name)
	tool := r.Get(call.Name)
	if tool == nil {
		return r.normalizeResult(call, ToolResult{
			ToolCallID: call.ID,
			Name:       call.Name,
			Content:    `{"ok":false,"error":"tool not found","code":"not_found"}`,
			IsError:    true,
		}, time.Since(start).Milliseconds()), nil
	}
	if hasSpec {
		if err := validateToolInput(spec.Parameters, call.Input); err != nil {
			return r.normalizeResult(call, ToolResult{
				ToolCallID: call.ID,
				Name:       call.Name,
				Content:    fmt.Sprintf(`{"ok":false,"error":%q,"code":"invalid_input"}`, err.Error()),
				IsError:    true,
			}, time.Since(start).Milliseconds()), nil
		}
	}
	res, err := tool.Run(ctx, call)
	if err != nil {
		return r.normalizeResult(call, ToolResult{
			ToolCallID: call.ID,
			Name:       call.Name,
			Content:    fmt.Sprintf(`{"ok":false,"error":%q,"code":"exec_failed"}`, err.Error()),
			IsError:    true,
		}, time.Since(start).Milliseconds()), nil
	}
	return r.normalizeResult(call, res, time.Since(start).Milliseconds()), nil
}

func (r *ToolRegistry) normalizeResult(call ToolCall, res ToolResult, durationMS int64) ToolResult {
	content, isErr := normalizeToolContent(call.Name, res.Content, res.IsError, r.maxResultChars, durationMS)
	res.ToolCallID = call.ID
	res.Name = call.Name
	res.Content = content
	res.IsError = isErr
	return res
}

func normalizeToolContent(toolName, raw string, fallbackErr bool, maxResultChars int, durationMS int64) (string, bool) {
	env := ToolEnvelope{
		OK:        !fallbackErr,
		Success:   !fallbackErr,
		Code:      "ok",
		Data:      map[string]any{},
		Truncated: false,
		Metadata: map[string]any{
			"source_tool": toolName,
			"duration_ms": durationMS,
		},
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed != "" {
		var parsed map[string]any
		if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
			if v, ok := parsed["ok"].(bool); ok {
				env.OK = v
			} else if v, ok := parsed["success"].(bool); ok {
				env.OK = v
			}
			if v, ok := parsed["success"].(bool); ok {
				env.Success = v
			} else {
				env.Success = env.OK
			}
			if v, ok := parsed["code"].(string); ok && strings.TrimSpace(v) != "" {
				env.Code = v
			}
			if v, ok := parsed["error"].(string); ok {
				env.Error = v
			} else if v, ok := parsed["message"].(string); ok {
				env.Error = v
			}
			if v, ok := parsed["summary"].(string); ok {
				env.Summary = v
			}
			if v, ok := parsed["data"]; ok {
				if data, ok := v.(map[string]any); ok {
					env.Data = data
				} else {
					env.Data = map[string]any{"payload": v}
				}
			} else {
				delete(parsed, "ok")
				delete(parsed, "success")
				delete(parsed, "code")
				delete(parsed, "message")
				delete(parsed, "error")
				delete(parsed, "summary")
				delete(parsed, "truncated")
				delete(parsed, "meta")
				delete(parsed, "metadata")
				if len(parsed) > 0 {
					env.Data = parsed
				}
			}
			if tv, ok := parsed["truncated"].(bool); ok {
				env.Truncated = tv
			}
			if mv, ok := parsed["meta"].(map[string]any); ok {
				for k, v := range mv {
					env.Metadata[k] = v
				}
			}
			if mv, ok := parsed["metadata"].(map[string]any); ok {
				for k, v := range mv {
					env.Metadata[k] = v
				}
			}
		} else {
			env.Data = map[string]any{"text": raw}
		}
	}
	if strings.TrimSpace(env.Summary) == "" {
		env.Summary = deriveSummary(env.Data, env.Error)
	}
	if trunc, ok := inferTruncated(env.Data, env.Metadata); ok {
		env.Truncated = trunc
	}
	env.Success = env.OK
	b, err := json.Marshal(env)
	if err != nil {
		if maxResultChars > 0 && len(raw) > maxResultChars {
			return raw[:maxResultChars], fallbackErr
		}
		return raw, fallbackErr
	}
	if maxResultChars > 0 && len(b) > maxResultChars {
		short := map[string]any{
			"ok":      false,
			"error":   "tool output truncated",
			"code":    "truncated",
			"summary": "tool output truncated",
			"data": map[string]any{
				"head": string(b[:maxResultChars]),
			},
			"truncated": true,
			"metadata": map[string]any{
				"source_tool": toolName,
				"duration_ms": durationMS,
			},
		}
		sb, serr := json.Marshal(short)
		if serr == nil {
			return string(sb), true
		}
		return string(b[:maxResultChars]), true
	}
	return string(b), !env.OK
}

func deriveSummary(data any, errMsg string) string {
	if strings.TrimSpace(errMsg) != "" {
		return clipSummary(errMsg, 220)
	}
	obj, ok := data.(map[string]any)
	if !ok {
		return "tool completed"
	}
	if s, ok := obj["summary"].(string); ok && strings.TrimSpace(s) != "" {
		return clipSummary(s, 220)
	}
	if p, ok := obj["payload"].(map[string]any); ok {
		if s, ok := p["stdout"].(string); ok && strings.TrimSpace(s) != "" {
			return clipSummary(s, 220)
		}
	}
	if c, ok := obj["content"].(string); ok && strings.TrimSpace(c) != "" {
		return clipSummary(c, 220)
	}
	return "tool completed"
}

func inferTruncated(data any, metadata any) (bool, bool) {
	if md, ok := metadata.(map[string]any); ok {
		if t, ok := md["truncated"].(bool); ok {
			return t, true
		}
	}
	obj, ok := data.(map[string]any)
	if !ok {
		return false, false
	}
	if t, ok := obj["truncated"].(bool); ok {
		return t, true
	}
	if m, ok := obj["metrics"].(map[string]any); ok {
		if t, ok := m["stdout_truncation"].(map[string]any); ok {
			if b, ok := t["truncated"].(bool); ok && b {
				return true, true
			}
		}
	}
	return false, false
}

func clipSummary(s string, limit int) string {
	s = strings.TrimSpace(s)
	if len(s) <= limit {
		return s
	}
	return strings.TrimSpace(s[:limit]) + "..."
}

func isValidToolSpec(spec ToolSpec) bool {
	if spec.Name == "" || spec.Parameters == nil {
		return false
	}
	props := map[string]any{}
	if propsAny, ok := spec.Parameters["properties"]; ok {
		p, ok := propsAny.(map[string]any)
		if !ok {
			return false
		}
		props = p
	}
	requiredAny, hasReq := spec.Parameters["required"]
	if !hasReq {
		return true
	}
	req, ok := coerceStringSlice(requiredAny)
	if !ok {
		return false
	}
	for _, key := range req {
		if _, ok := props[key]; !ok {
			return false
		}
	}
	return true
}

func validateToolInput(parameters map[string]any, raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var input map[string]any
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		return fmt.Errorf("input must be valid JSON object: %w", err)
	}
	propsAny, _ := parameters["properties"]
	props, _ := propsAny.(map[string]any)
	requiredAny, hasReq := parameters["required"]
	if hasReq {
		req, ok := coerceStringSlice(requiredAny)
		if !ok {
			return fmt.Errorf("schema required must be []string")
		}
		for _, k := range req {
			if _, ok := input[k]; !ok {
				return fmt.Errorf("missing required field %q", k)
			}
		}
	}
	ap, hasAP := parameters["additionalProperties"].(bool)
	if hasAP && !ap {
		for k := range input {
			if _, ok := props[k]; !ok {
				return fmt.Errorf("unknown field %q", k)
			}
		}
	}
	return nil
}

func normalizeToolSchema(parameters map[string]any) map[string]any {
	if parameters == nil {
		return nil
	}
	if _, ok := parameters["additionalProperties"]; !ok {
		parameters["additionalProperties"] = true
	}
	return parameters
}

func coerceStringSlice(v any) ([]string, bool) {
	if s, ok := v.([]string); ok {
		return s, true
	}
	if raw, ok := v.([]any); ok {
		out := make([]string, 0, len(raw))
		for _, it := range raw {
			str, ok := it.(string)
			if !ok {
				return nil, false
			}
			out = append(out, str)
		}
		return out, true
	}
	return nil, false
}
