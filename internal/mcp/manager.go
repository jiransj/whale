package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usewhale/whale/internal/build"
	"github.com/usewhale/whale/internal/core"
)

type Manager struct {
	mu       sync.RWMutex
	cfg      Config
	sessions map[string]*clientSession
	states   map[string]ServerState
	tools    []core.Tool
}

type ServerState struct {
	Name      string
	Disabled  bool
	Connected bool
	Error     string
	Tools     int
}

type clientSession struct {
	cfg     ServerConfig
	session *sdk.ClientSession
	cancel  context.CancelFunc
}

func NewManager(cfg Config) *Manager {
	return &Manager{
		cfg:      cfg,
		sessions: map[string]*clientSession{},
		states:   map[string]ServerState{},
	}
}

func (m *Manager) Initialize(ctx context.Context) {
	if m == nil {
		return
	}
	seen := map[string]bool{}
	registered := []core.Tool{}
	for _, name := range sortedServerNames(m.cfg.Servers) {
		srv := m.cfg.Servers[name]
		srv.Name = name
		if srv.Disabled {
			m.setState(ServerState{Name: name, Disabled: true})
			continue
		}
		tools, err := m.startServer(ctx, srv, seen)
		if err != nil {
			m.setState(ServerState{Name: name, Error: err.Error()})
			continue
		}
		registered = append(registered, tools...)
	}
	m.mu.Lock()
	m.tools = registered
	m.mu.Unlock()
}

func (m *Manager) Tools() []core.Tool {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]core.Tool, len(m.tools))
	copy(out, m.tools)
	return out
}

func (m *Manager) States() []ServerState {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ServerState, 0, len(m.states))
	for _, st := range m.states {
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (m *Manager) ConfigPath() string {
	if m == nil {
		return ""
	}
	return m.cfg.Path
}

func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var first error
	for name, sess := range m.sessions {
		if sess == nil {
			continue
		}
		if sess.session != nil {
			if err := sess.session.Close(); err != nil && first == nil {
				first = fmt.Errorf("close mcp %s: %w", name, err)
			}
		}
		if sess.cancel != nil {
			sess.cancel()
		}
		delete(m.sessions, name)
	}
	return first
}

func (m *Manager) CallTool(ctx context.Context, serverName, toolName string, args map[string]any) (*sdk.CallToolResult, error) {
	m.mu.RLock()
	sess := m.sessions[serverName]
	m.mu.RUnlock()
	if sess == nil || sess.session == nil {
		return nil, fmt.Errorf("mcp server %q is not connected", serverName)
	}
	callCtx, cancel := context.WithTimeout(ctx, sess.cfg.TimeoutDuration())
	defer cancel()
	return sess.session.CallTool(callCtx, &sdk.CallToolParams{Name: toolName, Arguments: args})
}

func (m *Manager) startServer(ctx context.Context, srv ServerConfig, seen map[string]bool) ([]core.Tool, error) {
	if strings.TrimSpace(srv.Command) == "" {
		return nil, fmt.Errorf("mcp server %q requires command", srv.Name)
	}
	mcpCtx, cancel := context.WithCancel(ctx)
	timeoutCtx, timeoutCancel := context.WithTimeout(mcpCtx, srv.TimeoutDuration())
	defer timeoutCancel()

	cmd := exec.CommandContext(mcpCtx, expandHome(srv.Command), srv.Args...)
	cmd.Env = append(os.Environ(), envPairs(srv.Env)...)
	transport := &sdk.CommandTransport{Command: cmd}
	client := sdk.NewClient(&sdk.Implementation{Name: "whale", Title: "Whale", Version: build.CurrentVersion()}, nil)
	session, err := client.Connect(timeoutCtx, transport, nil)
	if err != nil {
		cancel()
		return nil, err
	}
	listed, err := session.ListTools(timeoutCtx, &sdk.ListToolsParams{})
	if err != nil {
		_ = session.Close()
		cancel()
		return nil, err
	}
	disabled := srv.disabledToolSet()
	tools := make([]core.Tool, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		if tool == nil || strings.TrimSpace(tool.Name) == "" || disabled[tool.Name] {
			continue
		}
		name := UniqueToolName(QualifyToolName(srv.Name, tool.Name), seen)
		tools = append(tools, &Tool{manager: m, serverName: srv.Name, toolName: tool.Name, registeredName: name, spec: tool})
	}
	m.mu.Lock()
	m.sessions[srv.Name] = &clientSession{cfg: srv, session: session, cancel: cancel}
	m.states[srv.Name] = ServerState{Name: srv.Name, Connected: true, Tools: len(tools)}
	m.mu.Unlock()
	return tools, nil
}

func (m *Manager) setState(st ServerState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.states[st.Name] = st
}

func sortedServerNames(servers map[string]ServerConfig) []string {
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func envPairs(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+env[k])
	}
	return out
}

func expandHome(path string) string {
	path = strings.TrimSpace(path)
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}
