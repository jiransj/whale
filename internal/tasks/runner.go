package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/usewhale/whale/internal/agent"
	"github.com/usewhale/whale/internal/core"
	"github.com/usewhale/whale/internal/defaults"
	"github.com/usewhale/whale/internal/llm"
	"github.com/usewhale/whale/internal/policy"
	"github.com/usewhale/whale/internal/session"
	"github.com/usewhale/whale/internal/store"
)

const (
	MaxParallelPrompts    = 8
	DefaultMaxTokens      = 800
	DefaultMaxToolIters   = 12
	DefaultSummaryMaxChar = 8 * 1024
)

type ProviderFactory func(model string, maxTokens int) (llm.Provider, error)

type RunnerConfig struct {
	ProviderFactory     ProviderFactory
	ParentTools         *core.ToolRegistry
	MessageStore        store.MessageStore
	SessionsDir         string
	ParentSessionID     string
	ParentSessionIDFunc func() string
	WorkspaceRoot       string
	MemoryEnabled       bool
	MemoryMaxChars      int
	MemoryFileOrder     []string
	DefaultModel        string
	DefaultMaxTokens    int
	DefaultMaxToolIters int
	SummaryMaxChars     int
}

type Runner struct {
	providerFactory     ProviderFactory
	parentTools         *core.ToolRegistry
	messageStore        store.MessageStore
	sessionsDir         string
	parentSessionID     string
	parentSessionIDFunc func() string
	workspaceRoot       string
	memoryEnabled       bool
	memoryMaxChars      int
	memoryFileOrder     []string
	defaultModel        string
	defaultMaxTokens    int
	defaultMaxToolIters int
	summaryMaxChars     int
}

func NewRunner(cfg RunnerConfig) *Runner {
	model := strings.TrimSpace(cfg.DefaultModel)
	if model == "" {
		model = defaults.DefaultModel
	}
	maxTokens := cfg.DefaultMaxTokens
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}
	maxToolIters := cfg.DefaultMaxToolIters
	if maxToolIters <= 0 {
		maxToolIters = DefaultMaxToolIters
	}
	summaryMaxChars := cfg.SummaryMaxChars
	if summaryMaxChars <= 0 {
		summaryMaxChars = DefaultSummaryMaxChar
	}
	return &Runner{
		providerFactory:     cfg.ProviderFactory,
		parentTools:         cfg.ParentTools,
		messageStore:        cfg.MessageStore,
		sessionsDir:         strings.TrimSpace(cfg.SessionsDir),
		parentSessionID:     strings.TrimSpace(cfg.ParentSessionID),
		parentSessionIDFunc: cfg.ParentSessionIDFunc,
		workspaceRoot:       strings.TrimSpace(cfg.WorkspaceRoot),
		memoryEnabled:       cfg.MemoryEnabled,
		memoryMaxChars:      cfg.MemoryMaxChars,
		memoryFileOrder:     append([]string(nil), cfg.MemoryFileOrder...),
		defaultModel:        model,
		defaultMaxTokens:    maxTokens,
		defaultMaxToolIters: maxToolIters,
		summaryMaxChars:     summaryMaxChars,
	}
}

type ParallelReasonRequest struct {
	Prompts   []string `json:"prompts"`
	Model     string   `json:"model,omitempty"`
	MaxTokens int      `json:"max_tokens,omitempty"`
}

type ParallelReasonResult struct {
	Index  int       `json:"index"`
	Prompt string    `json:"prompt"`
	Output string    `json:"output,omitempty"`
	Error  string    `json:"error,omitempty"`
	Usage  llm.Usage `json:"usage,omitempty"`
}

type ParallelReasonResponse struct {
	Model   string                 `json:"model"`
	Results []ParallelReasonResult `json:"results"`
	Usage   llm.Usage              `json:"usage"`
}

func (r *Runner) ParallelReason(ctx context.Context, req ParallelReasonRequest) (ParallelReasonResponse, error) {
	prompts := compactPrompts(req.Prompts)
	if len(prompts) == 0 {
		return ParallelReasonResponse{}, errors.New("prompts is required")
	}
	if len(prompts) > MaxParallelPrompts {
		return ParallelReasonResponse{}, fmt.Errorf("prompts supports at most %d items", MaxParallelPrompts)
	}
	if r.providerFactory == nil {
		return ParallelReasonResponse{}, errors.New("provider factory is not configured")
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = r.defaultModel
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = r.defaultMaxTokens
	}
	results := make([]ParallelReasonResult, len(prompts))
	var wg sync.WaitGroup
	for i, prompt := range prompts {
		i, prompt := i, prompt
		results[i] = ParallelReasonResult{Index: i, Prompt: prompt}
		wg.Add(1)
		go func() {
			defer wg.Done()
			output, usage, err := r.runOneReasoningQuery(ctx, model, maxTokens, prompt)
			results[i].Output = output
			results[i].Usage = usage
			if err != nil {
				results[i].Error = err.Error()
			}
		}()
	}
	wg.Wait()
	var usage llm.Usage
	for _, res := range results {
		usage = addUsage(usage, res.Usage)
	}
	return ParallelReasonResponse{Model: model, Results: results, Usage: usage}, nil
}

func (r *Runner) runOneReasoningQuery(ctx context.Context, model string, maxTokens int, prompt string) (string, llm.Usage, error) {
	provider, err := r.providerFactory(model, maxTokens)
	if err != nil {
		return "", llm.Usage{}, err
	}
	history := []core.Message{
		{Role: core.RoleSystem, Text: "You are a cheap parallel reasoning worker. Answer only the assigned subquery, be concise, and do not use tools."},
		{Role: core.RoleUser, Text: prompt},
	}
	var b strings.Builder
	var final string
	var usage llm.Usage
	for ev := range provider.StreamResponse(ctx, history, nil) {
		switch ev.Type {
		case llm.EventContentDelta:
			b.WriteString(ev.Content)
		case llm.EventComplete:
			if ev.Response != nil {
				final = ev.Response.Content
				usage = ev.Response.Usage
			}
		case llm.EventError:
			if ev.Err != nil {
				return strings.TrimSpace(firstNonEmpty(final, b.String())), usage, ev.Err
			}
			return strings.TrimSpace(firstNonEmpty(final, b.String())), usage, errors.New("provider error")
		}
	}
	return strings.TrimSpace(firstNonEmpty(final, b.String())), usage, ctx.Err()
}

type SpawnSubagentRequest struct {
	Task             string `json:"task"`
	Role             string `json:"role,omitempty"`
	Model            string `json:"model,omitempty"`
	MaxToolIters     int    `json:"max_tool_iters,omitempty"`
	ParentToolCallID string `json:"-"`
}

type SpawnSubagentResponse struct {
	SessionID         string   `json:"session_id"`
	Role              string   `json:"role"`
	Model             string   `json:"model"`
	PermissionProfile string   `json:"permission_profile"`
	Status            string   `json:"status"`
	Summary           string   `json:"summary"`
	Error             string   `json:"error,omitempty"`
	Truncated         bool     `json:"truncated"`
	ToolCalls         []string `json:"tool_calls,omitempty"`
	DurationMS        int64    `json:"duration_ms"`
	CompletedAt       string   `json:"completed_at"`
}

type SpawnSubagentError struct {
	SessionID string
	Code      string
	Message   string
	Err       error
}

func (e *SpawnSubagentError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "subagent failed"
}

func (e *SpawnSubagentError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func (r *Runner) SpawnSubagent(ctx context.Context, req SpawnSubagentRequest) (SpawnSubagentResponse, error) {
	return r.SpawnSubagentWithProgress(ctx, req, nil)
}

func (r *Runner) SpawnSubagentWithProgress(ctx context.Context, req SpawnSubagentRequest, progress func(core.ToolProgress)) (SpawnSubagentResponse, error) {
	task := strings.TrimSpace(req.Task)
	if task == "" {
		return SpawnSubagentResponse{}, errors.New("task is required")
	}
	if r.providerFactory == nil {
		return SpawnSubagentResponse{}, errors.New("provider factory is not configured")
	}
	role := strings.TrimSpace(req.Role)
	if role == "" {
		role = "explore"
	}
	if !validRole(role) {
		return SpawnSubagentResponse{}, fmt.Errorf("unsupported subagent role %q", role)
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = r.defaultModel
	}
	maxToolIters := req.MaxToolIters
	if maxToolIters <= 0 {
		maxToolIters = r.defaultMaxToolIters
	}
	childTools, err := BuildReadOnlyRegistry(r.parentTools)
	if err != nil {
		return SpawnSubagentResponse{}, err
	}
	provider, err := r.providerFactory(model, 0)
	if err != nil {
		return SpawnSubagentResponse{}, err
	}
	sessionID := r.childSessionID(req.ParentToolCallID)
	childStore := r.messageStore
	if childStore == nil {
		childStore = store.NewInMemoryStore()
	}
	start := time.Now()
	parentSessionID := r.currentParentSessionID()
	r.saveSubagentMeta(sessionID, session.SessionMeta{
		Kind:            "subagent",
		ParentSessionID: parentSessionID,
		Role:            role,
		Model:           model,
		Task:            task,
		Status:          "running",
		Workspace:       r.workspaceRoot,
		StartedAt:       start.UTC(),
	})
	child := agent.NewAgentWithRegistry(provider, childStore, childTools,
		agent.WithSessionMode(session.ModeAsk),
		agent.WithToolPolicy(policy.DefaultToolPolicy{Mode: policy.ApprovalModeNever}),
		agent.WithSessionsDir(r.sessionsDir),
		agent.WithProjectMemory(r.memoryEnabled, r.memoryMaxChars, r.memoryFileOrder, r.workspaceRoot),
		agent.WithUsageLogPath(""),
		agent.WithMaxToolIters(maxToolIters),
		agent.WithExtraSystemBlocks(subagentSystemBlock(role)),
	)
	events, err := child.RunStream(ctx, sessionID, task)
	if err != nil {
		r.patchSubagentMeta(sessionID, session.SessionMeta{Status: "failed", Error: err.Error(), CompletedAt: time.Now().UTC()})
		return SpawnSubagentResponse{}, &SpawnSubagentError{SessionID: sessionID, Code: "spawn_subagent_failed", Message: err.Error(), Err: err}
	}
	var summary string
	var toolCalls []string
	childActions := map[string]childToolAction{}
	progressCount := 0
	fail := func(code string, err error) (SpawnSubagentResponse, error) {
		msg := "subagent failed"
		if code == "cancelled" {
			msg = "turn cancelled"
		}
		if err != nil {
			msg = err.Error()
		}
		r.patchSubagentMeta(sessionID, session.SessionMeta{Status: code, Error: msg, CompletedAt: time.Now().UTC()})
		return SpawnSubagentResponse{}, &SpawnSubagentError{SessionID: sessionID, Code: code, Message: msg, Err: err}
	}
	for ev := range events {
		switch ev.Type {
		case agent.AgentEventTypeToolCall:
			if ev.ToolCall != nil {
				toolCalls = append(toolCalls, ev.ToolCall.Name)
				action := summarizeChildToolCall(*ev.ToolCall)
				childActions[ev.ToolCall.ID] = action
				progressCount++
				emitSubagentProgress(progress, role, model, progressCount, "running", action.Running, map[string]any{
					"child_session_id": sessionID,
					"child_tool":       ev.ToolCall.Name,
				})
			}
		case agent.AgentEventTypeToolResult:
			if ev.Result != nil {
				progressCount++
				status := "running"
				if ev.Result.IsError {
					status = "tool_failed"
				}
				action := childActions[ev.Result.ToolCallID]
				emitSubagentProgress(progress, role, model, progressCount, status, summarizeChildToolResult(*ev.Result, action), map[string]any{
					"child_session_id": sessionID,
					"child_tool":       ev.Result.Name,
				})
			}
		case agent.AgentEventTypeDone:
			if ev.Message != nil {
				summary = ev.Message.Text
				emitSubagentProgress(progress, role, model, progressCount, "summarizing", "child produced final summary", map[string]any{
					"child_session_id": sessionID,
				})
			}
		case agent.AgentEventTypeError:
			if ev.Err != nil {
				return fail("failed", ev.Err)
			}
			return fail("failed", errors.New("subagent failed"))
		case agent.AgentEventTypeTurnCancelled:
			return fail("cancelled", ctx.Err())
		}
		if err := ctx.Err(); err != nil {
			return fail("cancelled", err)
		}
	}
	summary, truncated := truncateString(strings.TrimSpace(summary), r.summaryMaxChars)
	completedAt := time.Now().UTC()
	r.patchSubagentMeta(sessionID, session.SessionMeta{Status: "completed", Summary: summary, CompletedAt: completedAt})
	return SpawnSubagentResponse{
		SessionID:         sessionID,
		Role:              role,
		Model:             model,
		PermissionProfile: "read_only",
		Status:            "completed",
		Summary:           summary,
		Truncated:         truncated,
		ToolCalls:         toolCalls,
		DurationMS:        time.Since(start).Milliseconds(),
		CompletedAt:       completedAt.Format(time.RFC3339),
	}, nil
}

func (r *Runner) childSessionID(parentToolCallID string) string {
	childID := safeSessionPart(parentToolCallID)
	if childID == "" {
		childID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	parentID := safeSessionPart(r.currentParentSessionID())
	if parentID == "" {
		return "subagent-" + childID
	}
	return parentID + "--subagent-" + childID
}

func (r *Runner) currentParentSessionID() string {
	if r != nil && r.parentSessionIDFunc != nil {
		if id := strings.TrimSpace(r.parentSessionIDFunc()); id != "" {
			return id
		}
	}
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.parentSessionID)
}

func safeSessionPart(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	out := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-', r == '_', r == '.':
			return r
		default:
			return '-'
		}
	}, v)
	out = strings.Trim(out, "-_.")
	if len(out) > 96 {
		out = out[:96]
	}
	return out
}

func (r *Runner) saveSubagentMeta(sessionID string, meta session.SessionMeta) {
	if strings.TrimSpace(r.sessionsDir) == "" || strings.TrimSpace(sessionID) == "" {
		return
	}
	_ = session.SaveSessionMeta(r.sessionsDir, sessionID, meta)
}

func (r *Runner) patchSubagentMeta(sessionID string, meta session.SessionMeta) {
	if strings.TrimSpace(r.sessionsDir) == "" || strings.TrimSpace(sessionID) == "" {
		return
	}
	_, _ = session.PatchSessionMeta(r.sessionsDir, sessionID, meta)
}

func emitSubagentProgress(progress func(core.ToolProgress), role, model string, count int, status, summary string, metadata map[string]any) {
	if progress == nil {
		return
	}
	progress(core.ToolProgress{
		ToolName: "spawn_subagent",
		Role:     role,
		Model:    model,
		Count:    count,
		Status:   status,
		Summary:  strings.TrimSpace(summary),
		Metadata: metadata,
	})
}

type childToolAction struct {
	ToolName string
	Target   string
	Running  string
	DoneVerb string
}

func summarizeChildToolCall(call core.ToolCall) childToolAction {
	var args map[string]any
	_ = json.Unmarshal([]byte(call.Input), &args)
	switch call.Name {
	case "read_file":
		target := compactProgressTarget(firstNonEmptyString(asString(args["file_path"]), asString(args["path"]), "file"))
		return childToolAction{ToolName: call.Name, Target: target, Running: "Reading " + target, DoneVerb: "Read"}
	case "list_dir":
		target := compactProgressTarget(firstNonEmptyString(asString(args["path"]), "."))
		return childToolAction{ToolName: call.Name, Target: target, Running: "Listing " + target, DoneVerb: "Listed"}
	case "grep", "search_content":
		target := summarizeSearchTarget(args)
		return childToolAction{ToolName: call.Name, Target: target, Running: "Searching " + target, DoneVerb: "Searched"}
	case "search_files":
		pattern := quoteProgressTerm(firstNonEmptyString(asString(args["pattern"]), asString(args["query"]), "files"))
		return childToolAction{ToolName: call.Name, Target: pattern, Running: "Searching files " + pattern, DoneVerb: "Searched files"}
	case "web_search":
		target := quoteProgressTerm(firstNonEmptyString(asString(args["query"]), "query"))
		return childToolAction{ToolName: call.Name, Target: target, Running: "Searching web " + target, DoneVerb: "Searched web"}
	case "fetch", "web_fetch":
		target := compactURLForProgress(firstNonEmptyString(asString(args["url"]), "url"))
		return childToolAction{ToolName: call.Name, Target: target, Running: "Fetching " + target, DoneVerb: "Fetched"}
	default:
		if call.Name != "" {
			return childToolAction{ToolName: call.Name, Target: call.Name, Running: "Using " + call.Name, DoneVerb: "Used"}
		}
		return childToolAction{ToolName: call.Name, Target: "tool", Running: "Using tool", DoneVerb: "Used"}
	}
}

func summarizeSearchTarget(args map[string]any) string {
	pattern := quoteProgressTerm(firstNonEmptyString(asString(args["pattern"]), asString(args["query"]), "content"))
	path := compactProgressTarget(firstNonEmptyString(asString(args["path"]), asString(args["directory"]), ""))
	include := compactProgressTarget(firstNonEmptyString(asString(args["include"]), ""))
	if path != "" && include != "" {
		return fmt.Sprintf("%s in %s (%s)", pattern, path, include)
	}
	if path != "" {
		return fmt.Sprintf("%s in %s", pattern, path)
	}
	if include != "" {
		return fmt.Sprintf("%s (%s)", pattern, include)
	}
	return pattern
}

func summarizeChildToolResult(res core.ToolResult, action childToolAction) string {
	if res.IsError {
		if action.Target != "" {
			return action.DoneVerb + " " + action.Target + " failed"
		}
		return res.Name + " failed"
	}
	if action.Target == "" {
		return res.Name + " completed"
	}
	summary := action.DoneVerb + " " + action.Target
	if suffix := childResultMetricSuffix(res); suffix != "" {
		summary += " · " + suffix
	}
	return summary
}

func childResultMetricSuffix(res core.ToolResult) string {
	env, ok := core.ParseToolEnvelope(res.Content)
	if !ok || !env.OK || !env.Success {
		return ""
	}
	metrics := asMap(env.Data["metrics"])
	payload := asMap(env.Data["payload"])
	switch res.Name {
	case "read_file":
		total := asInt(metrics["total_lines"])
		returned := asInt(metrics["returned_lines"])
		if total > 0 && returned > 0 {
			return fmt.Sprintf("%d/%d lines", returned, total)
		}
	case "list_dir":
		items := asAnySlice(payload["items"])
		if len(items) == 0 {
			items = asAnySlice(env.Data["items"])
		}
		if len(items) > 0 {
			return fmt.Sprintf("%d items", len(items))
		}
	case "grep", "search_content":
		total := asInt(metrics["total_matches"])
		files := asInt(metrics["files_matched"])
		if files > 0 {
			return fmt.Sprintf("%d matches in %d files", total, files)
		}
		if total >= 0 {
			return fmt.Sprintf("%d matches", total)
		}
	case "search_files":
		total := asInt(metrics["total_matches"])
		if total > 0 {
			return fmt.Sprintf("%d matches", total)
		}
		items := asAnySlice(payload["items"])
		if len(items) > 0 {
			return fmt.Sprintf("%d matches", len(items))
		}
	case "web_search":
		count := asInt(env.Data["count"])
		if count > 0 {
			return fmt.Sprintf("%d results", count)
		}
	case "fetch", "web_fetch":
		status := asInt(env.Data["status_code"])
		if status > 0 {
			return fmt.Sprintf("HTTP %d", status)
		}
	}
	return ""
}

func compactPrompts(in []string) []string {
	out := make([]string, 0, len(in))
	for _, prompt := range in {
		if trimmed := strings.TrimSpace(prompt); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func addUsage(a, b llm.Usage) llm.Usage {
	a.PromptTokens += b.PromptTokens
	a.CompletionTokens += b.CompletionTokens
	a.TotalTokens += b.TotalTokens
	a.PromptCacheHitTokens += b.PromptCacheHitTokens
	a.PromptCacheMissTokens += b.PromptCacheMissTokens
	a.ReasoningReplayTokens += b.ReasoningReplayTokens
	return a
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func quoteProgressTerm(v string) string {
	v = compactProgressTarget(v)
	if v == "" {
		return ""
	}
	return `"` + v + `"`
}

func compactProgressTarget(v string) string {
	v = strings.Join(strings.Fields(strings.TrimSpace(v)), " ")
	const limit = 100
	if len(v) <= limit {
		return v
	}
	return v[:limit-3] + "..."
}

func compactURLForProgress(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return compactProgressTarget(raw)
	}
	target := u.Host + u.EscapedPath()
	if target == u.Host && u.RawQuery != "" {
		target += "?" + u.RawQuery
	}
	return compactProgressTarget(target)
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	if m == nil {
		return map[string]any{}
	}
	return m
}

func asAnySlice(v any) []any {
	if v == nil {
		return nil
	}
	arr, ok := v.([]any)
	if ok {
		return arr
	}
	return nil
}

func asInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return -1
	}
}

func truncateString(v string, limit int) (string, bool) {
	if limit <= 0 || len(v) <= limit {
		return v, false
	}
	return v[:limit], true
}

func marshalSuccess(call core.ToolCall, data map[string]any) (core.ToolResult, error) {
	content, err := core.MarshalToolEnvelope(core.NewToolSuccessEnvelope(data))
	if err != nil {
		return core.ToolResult{}, err
	}
	return core.ToolResult{ToolCallID: call.ID, Name: call.Name, Content: content}, nil
}

func marshalError(call core.ToolCall, code, msg string) (core.ToolResult, error) {
	content, err := core.MarshalToolEnvelope(core.NewToolErrorEnvelope(code, msg))
	if err != nil {
		return core.ToolResult{}, err
	}
	return core.ToolResult{ToolCallID: call.ID, Name: call.Name, Content: content, IsError: true}, nil
}

func marshalErrorWithData(call core.ToolCall, code, msg string, data map[string]any) (core.ToolResult, error) {
	env := core.NewToolErrorEnvelope(code, msg)
	env.Data = data
	content, err := core.MarshalToolEnvelope(env)
	if err != nil {
		return core.ToolResult{}, err
	}
	return core.ToolResult{ToolCallID: call.ID, Name: call.Name, Content: content, IsError: true}, nil
}

func decodeInput[T any](call core.ToolCall) (T, error) {
	var out T
	if err := json.Unmarshal([]byte(call.Input), &out); err != nil {
		return out, err
	}
	return out, nil
}
