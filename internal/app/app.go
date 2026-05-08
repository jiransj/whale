package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/usewhale/whale/internal/agent"
	"github.com/usewhale/whale/internal/core"
	"github.com/usewhale/whale/internal/defaults"
	whalemcp "github.com/usewhale/whale/internal/mcp"
	"github.com/usewhale/whale/internal/policy"
	"github.com/usewhale/whale/internal/session"
	"github.com/usewhale/whale/internal/store"
	"github.com/usewhale/whale/internal/tools"
)

const CommandsHelp = "/model, /permissions, /ask [prompt], /plan [prompt], /skills, /new [id], /resume, /clear, /status, /mcp, /compact, /init, /exit"

type Config struct {
	DataDir              string
	ApprovalMode         string
	AllowPrefixes        string
	DenyPrefixes         string
	AutoCompact          bool
	AutoCompactThreshold float64
	ContextWindow        int
	MemoryEnabled        bool
	MemoryMaxChars       int
	MemoryFileOrder      string
	BudgetWarningUSD     float64
	Model                string
	ModelExplicit        bool
	ReasoningEffort      string
	ThinkingEnabled      bool
	MCPConfigPath        string
}

type StartOptions struct {
	SessionID     string
	ModeOverride  string
	ResumeMenu    bool
	NewSession    bool
	ApprovalFunc  policy.ApprovalFunc
	UserInputFunc agent.UserInputFunc
}

type ResumeChoice struct {
	Index int
	ID    string
}

type App struct {
	ctx              context.Context
	sessionsDir      string
	workspaceRoot    string
	branch           string
	msgStore         *store.JSONLStore
	toolRegistry     *core.ToolRegistry
	hooks            []agent.ResolvedHook
	hookRunner       *agent.HookRunner
	hookSources      []string
	currentMode      session.Mode
	sessionID        string
	approvalMode     policy.ApprovalMode
	allowPrefixes    []string
	denyPrefixes     []string
	budgetWarningUSD float64
	cfg              Config
	model            string
	reasoningEffort  string
	thinkingEnabled  bool
	mcpManager       *whalemcp.Manager

	a          *agent.Agent
	apiKey     string
	approvalMu sync.Mutex
	approvalFn policy.ApprovalFunc
	userInput  agent.UserInputFunc
}

func DefaultConfig() Config {
	return Config{
		DataDir:              store.DefaultDataDir(),
		ApprovalMode:         string(policy.ApprovalModeOnRequest),
		AutoCompact:          true,
		AutoCompactThreshold: defaults.DefaultAutoCompactThreshold,
		ContextWindow:        defaultContextWindow,
		MemoryEnabled:        true,
		MemoryMaxChars:       defaults.DefaultMemoryMaxChars,
		MemoryFileOrder:      defaults.DefaultMemoryFileOrderCSV,
		Model:                defaults.DefaultModel,
		ReasoningEffort:      defaults.DefaultReasoningEffort,
		ThinkingEnabled:      defaults.DefaultThinkingEnabled,
	}
}

func New(ctx context.Context, cfg Config, start StartOptions) (*App, error) {
	sessionsDir := store.DefaultSessionsDir(cfg.DataDir)
	msgStore, err := store.NewJSONLStore(sessionsDir)
	if err != nil {
		return nil, fmt.Errorf("init session store failed: %w", err)
	}
	sessionID := ""
	if sid := strings.TrimSpace(start.SessionID); sid != "" {
		sessionID = sid
	} else if start.NewSession {
		sessionID = newSessionID(time.Now())
	} else {
		var err error
		sessionID, err = resolveInitialSessionID(sessionsDir)
		if err != nil {
			return nil, fmt.Errorf("resolve session failed: %w", err)
		}
	}
	approvalMode, err := policy.ParseApprovalMode(cfg.ApprovalMode)
	if err != nil {
		return nil, fmt.Errorf("invalid --approval-mode: %w", err)
	}
	workspaceRoot, _ := os.Getwd()
	toolset, err := tools.NewToolset(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("init tools failed: %w", err)
	}
	mcpConfigPath := strings.TrimSpace(cfg.MCPConfigPath)
	if mcpConfigPath == "" {
		mcpConfigPath = whalemcp.DefaultConfigPath(cfg.DataDir)
	}
	mcpConfig, err := whalemcp.LoadConfig(mcpConfigPath)
	if err != nil {
		return nil, fmt.Errorf("load mcp config: %w", err)
	}
	mcpManager := whalemcp.NewManager(mcpConfig)
	mcpManager.SetWorkspaceRoot(workspaceRoot)
	mcpManager.Initialize(ctx)
	registeredTools := append([]core.Tool{}, toolset.Tools()...)
	registeredTools = append(registeredTools, mcpManager.Tools()...)
	toolRegistry, err := core.NewToolRegistryChecked(registeredTools)
	if err != nil {
		return nil, fmt.Errorf("init tool registry failed: %w", err)
	}
	hooks, hookSources, hookLoadErr := agent.LoadHooks(workspaceRoot, "")
	if hookLoadErr != nil {
		return nil, fmt.Errorf("load hooks failed: %w", hookLoadErr)
	}
	modeState, err := session.LoadModeState(sessionsDir, sessionID)
	if err != nil {
		return nil, fmt.Errorf("load session mode failed: %w", err)
	}
	if raw := strings.TrimSpace(start.ModeOverride); raw != "" {
		mode, err := session.ParseMode(raw)
		if err != nil {
			return nil, err
		}
		modeState.Mode = mode
		if err := session.SaveModeState(sessionsDir, sessionID, mode); err != nil {
			return nil, fmt.Errorf("save mode state failed: %w", err)
		}
	}
	branch := session.DetectGitBranch(workspaceRoot)
	if _, err := session.PatchSessionMeta(sessionsDir, sessionID, session.SessionMeta{Workspace: workspaceRoot, Branch: branch}); err != nil {
		return nil, fmt.Errorf("patch session meta failed: %w", err)
	}

	// Load persisted preferences to overlay on Config defaults.
	// Explicit CLI flags (non-default cfg values) take priority over preferences.
	prefs, _ := LoadPreferences(cfg.DataDir)
	model := firstNonEmpty(strings.TrimSpace(cfg.Model), defaults.DefaultModel)
	effort := normalizeEffort(firstNonEmpty(strings.TrimSpace(cfg.ReasoningEffort), defaults.DefaultReasoningEffort))
	thinking := cfg.ThinkingEnabled
	// If cfg values match hardcoded defaults, try preferences.
	if !cfg.ModelExplicit && strings.TrimSpace(cfg.Model) == defaults.DefaultModel && strings.TrimSpace(prefs.Model) != "" {
		model = prefs.Model
	}
	if strings.TrimSpace(cfg.ReasoningEffort) == defaults.DefaultReasoningEffort && strings.TrimSpace(prefs.ReasoningEffort) != "" {
		effort = normalizeEffort(prefs.ReasoningEffort)
	}
	if cfg.ThinkingEnabled && prefs.ThinkingEnabled != nil {
		thinking = *prefs.ThinkingEnabled
	}
	cfg.ContextWindow = resolveContextWindow(cfg.ContextWindow, model)
	apiKey, err := LoadDeepSeekAPIKey(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("load api key failed: %w", err)
	}

	return &App{
		ctx:              ctx,
		sessionsDir:      sessionsDir,
		workspaceRoot:    workspaceRoot,
		branch:           branch,
		msgStore:         msgStore,
		toolRegistry:     toolRegistry,
		hooks:            hooks,
		hookRunner:       agent.NewHookRunner(hooks, workspaceRoot),
		hookSources:      hookSources,
		currentMode:      modeState.Mode,
		sessionID:        sessionID,
		approvalMode:     approvalMode,
		allowPrefixes:    parseCSVList(cfg.AllowPrefixes),
		denyPrefixes:     parseCSVList(cfg.DenyPrefixes),
		budgetWarningUSD: cfg.BudgetWarningUSD,
		cfg:              cfg,
		model:            model,
		reasoningEffort:  effort,
		thinkingEnabled:  thinking,
		mcpManager:       mcpManager,
		apiKey:           apiKey,
		approvalFn:       defaultApprovalFunc(start.ApprovalFunc),
		userInput:        defaultUserInputFunc(start.UserInputFunc),
	}, nil
}

func defaultApprovalFunc(fn policy.ApprovalFunc) policy.ApprovalFunc {
	if fn != nil {
		return fn
	}
	return func(policy.ApprovalRequest) bool { return false }
}

func defaultUserInputFunc(fn agent.UserInputFunc) agent.UserInputFunc {
	if fn != nil {
		return fn
	}
	return func(agent.UserInputRequest) (core.UserInputResponse, bool) {
		return core.UserInputResponse{}, false
	}
}

func (a *App) SetApprovalFunc(fn policy.ApprovalFunc) {
	if fn == nil {
		return
	}
	a.approvalFn = fn
	a.a = nil
}

func (a *App) SetUserInputFunc(fn agent.UserInputFunc) {
	if fn == nil {
		return
	}
	a.userInput = fn
	a.a = nil
}

func (a *App) StartupLines() []string {
	lines := []string{"whale repl", fmt.Sprintf("session: %s", a.sessionID), fmt.Sprintf("mode: %s", a.currentMode), fmt.Sprintf("approval_mode: %s", a.approvalMode)}
	lines = append(lines, fmt.Sprintf("model: %s", a.model), fmt.Sprintf("effort: %s", a.reasoningEffort), fmt.Sprintf("thinking: %s", onOff(a.thinkingEnabled)))
	if a.budgetWarningUSD > 0 {
		lines = append(lines, fmt.Sprintf("budget_warning_usd: %.4f", a.budgetWarningUSD))
	} else {
		lines = append(lines, "budget_warning_usd: disabled")
	}
	if len(a.hookSources) > 0 {
		lines = append(lines, fmt.Sprintf("hooks: %s", strings.Join(a.hookSources, ", ")))
	}
	if a.mcpManager != nil {
		states := a.mcpManager.States()
		if len(states) > 0 {
			connected := 0
			failed := 0
			for _, st := range states {
				if st.Connected {
					connected++
				} else if st.Error != "" {
					failed++
				}
			}
			lines = append(lines, fmt.Sprintf("mcp: %d server(s), %d connected, %d failed", len(states), connected, failed))
		}
	}
	lines = append(lines, "commands: "+CommandsHelp, "env: DEEPSEEK_API_KEY=...")
	if ust, err := session.LoadUserInputState(a.sessionsDir, a.sessionID); err == nil && ust.Pending {
		lines = append(lines, fmt.Sprintf("pending user input: tool_call=%s questions=%d", ust.ToolCallID, len(ust.Questions)))
	}
	return lines
}

func (a *App) SessionID() string                 { return a.sessionID }
func (a *App) CurrentMode() session.Mode         { return a.currentMode }
func (a *App) ApprovalMode() policy.ApprovalMode { return a.approvalMode }
func (a *App) SetMode(mode session.Mode) (string, error) {
	if _, err := session.ParseMode(string(mode)); err != nil {
		return "", err
	}
	if err := session.SaveModeState(a.sessionsDir, a.sessionID, mode); err != nil {
		return "", err
	}
	a.currentMode = mode
	a.a = nil
	return fmt.Sprintf("%s mode enabled", modeTitle(mode)), nil
}
func (a *App) ToggleMode() (string, error) {
	switch a.currentMode {
	case session.ModeAgent:
		return a.SetMode(session.ModeAsk)
	case session.ModeAsk:
		return a.SetMode(session.ModePlan)
	default:
		return a.SetMode(session.ModeAgent)
	}
}
func (a *App) SetApprovalMode(mode policy.ApprovalMode) {
	a.approvalMode = mode
	a.a = nil
}
func (a *App) WorkspaceRoot() string   { return a.workspaceRoot }
func (a *App) Model() string           { return a.model }
func (a *App) ReasoningEffort() string { return a.reasoningEffort }
func (a *App) ThinkingEnabled() bool   { return a.thinkingEnabled }
func (a *App) ListMessages() ([]core.Message, error) {
	return a.msgStore.List(a.ctx, a.sessionID)
}
func (a *App) SupportedModels() []string { return defaults.SupportedModels() }
func (a *App) SupportedEfforts() []string {
	return []string{"high", "max"}
}

func (a *App) SetModelAndEffort(modelName, effort string) error {
	m := strings.TrimSpace(strings.ToLower(modelName))
	e := normalizeEffort(effort)
	if m == "" || e == "" {
		return errors.New("model and effort are required")
	}
	if !containsString(a.SupportedModels(), m) {
		return fmt.Errorf("unsupported model: %s", modelName)
	}
	if !containsString(a.SupportedEfforts(), e) {
		return fmt.Errorf("unsupported effort: %s", effort)
	}
	a.model = m
	a.reasoningEffort = e
	a.a = nil
	a.savePreferences()
	return nil
}

func (a *App) SetThinkingEnabled(enabled bool) {
	a.thinkingEnabled = enabled
	a.a = nil
	a.savePreferences()
}

func (a *App) Close() error {
	if a == nil || a.mcpManager == nil {
		return nil
	}
	return a.mcpManager.Close()
}

func (a *App) savePreferences() {
	enabled := a.thinkingEnabled
	_ = SavePreferences(a.cfg.DataDir, Preferences{
		Model:           a.model,
		ReasoningEffort: a.reasoningEffort,
		ThinkingEnabled: &enabled,
	})
}
