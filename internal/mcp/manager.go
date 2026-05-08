package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

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
	kind, err := srv.transportKind()
	if err != nil {
		return nil, fmt.Errorf("mcp server %q: %w", srv.Name, err)
	}
	mcpCtx, cancel := context.WithCancel(ctx)
	timeoutCtx, timeoutCancel := context.WithTimeout(mcpCtx, srv.TimeoutDuration())
	defer timeoutCancel()

	transport, stdioCmd, err := createTransport(mcpCtx, kind, srv)
	if err != nil {
		cancel()
		return nil, err
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "whale", Title: "Whale", Version: build.CurrentVersion()}, nil)
	session, err := client.Connect(timeoutCtx, transport, nil)
	if err != nil {
		cancel()
		if errors.Is(err, io.EOF) && stdioCmd != nil {
			err = maybeStdioErr(err, stdioCmd)
		}
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

func createTransport(ctx context.Context, kind string, srv ServerConfig) (sdk.Transport, *exec.Cmd, error) {
	switch kind {
	case "stdio":
		if strings.TrimSpace(srv.Command) == "" {
			return nil, nil, fmt.Errorf("mcp server %q requires command", srv.Name)
		}
		env, err := resolvedEnvPairs(srv.Env)
		if err != nil {
			return nil, nil, fmt.Errorf("mcp server %q env config: %w", srv.Name, err)
		}
		cmd := exec.CommandContext(ctx, expandHome(srv.Command), srv.Args...)
		cmd.Env = append(os.Environ(), env...)
		return &sdk.CommandTransport{Command: cmd}, cmd, nil
	case "http":
		if strings.TrimSpace(srv.URL) == "" {
			return nil, nil, fmt.Errorf("mcp server %q requires url", srv.Name)
		}
		headers, err := resolvedHeaders(srv.Headers)
		if err != nil {
			return nil, nil, fmt.Errorf("mcp server %q headers config: %w", srv.Name, err)
		}
		return &sdk.StreamableClientTransport{
			Endpoint: strings.TrimSpace(srv.URL),
			HTTPClient: &http.Client{Transport: headerRoundTripper{
				headers: headers,
				base:    http.DefaultTransport,
			}},
		}, nil, nil
	default:
		return nil, nil, fmt.Errorf("mcp server %q unsupported transport %q", srv.Name, kind)
	}
}

type headerRoundTripper struct {
	headers map[string]string
	base    http.RoundTripper
}

func (rt headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(rt.headers) > 0 {
		req = req.Clone(req.Context())
		for k, v := range rt.headers {
			req.Header.Set(k, v)
		}
	}
	base := rt.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
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

func maybeStdioErr(err error, cmd *exec.Cmd) error {
	checkErr := stdioCheck(cmd)
	if checkErr == nil {
		return err
	}
	return errors.Join(err, checkErr)
}

func stdioCheck(old *exec.Cmd) error {
	if old == nil {
		return nil
	}
	name := old.Path
	if name == "" && len(old.Args) > 0 {
		name = old.Args[0]
	}
	if name == "" {
		return nil
	}
	args := []string{}
	if len(old.Args) > 1 {
		args = old.Args[1:]
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = old.Env
	out, err := cmd.CombinedOutput()
	if err == nil || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil
	}
	return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
}
