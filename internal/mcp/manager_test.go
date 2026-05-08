package mcp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usewhale/whale/internal/core"
)

const runMCPTestServerEnv = "WHALE_RUN_MCP_TEST_SERVER"

type echoInput struct {
	Message string `json:"message"`
}

type echoOutput struct {
	Message string `json:"message"`
}

func TestMain(m *testing.M) {
	if os.Getenv(runMCPTestServerEnv) == "1" {
		os.Unsetenv(runMCPTestServerEnv)
		os.Exit(runTestMCPServer())
	}
	os.Exit(m.Run())
}

func runTestMCPServer() int {
	server := newEchoMCPServer()
	if err := server.Run(context.Background(), &sdk.StdioTransport{}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	return 0
}

func newEchoMCPServer() *sdk.Server {
	server := sdk.NewServer(&sdk.Implementation{Name: "whale-test-mcp", Version: "v0.0.0"}, nil)
	sdk.AddTool(server, &sdk.Tool{Name: "echo", Description: "echoes a message"}, func(ctx context.Context, req *sdk.CallToolRequest, input echoInput) (*sdk.CallToolResult, echoOutput, error) {
		return &sdk.CallToolResult{
			Content: []sdk.Content{&sdk.TextContent{Text: "echo:" + input.Message}},
		}, echoOutput{Message: input.Message}, nil
	})
	return server
}

func TestManagerInitializesAndCallsStdioTool(t *testing.T) {
	mgr := NewManager(Config{
		Servers: map[string]ServerConfig{
			"local": {
				Command: os.Args[0],
				Args:    []string{"-test.run=^$"},
				Env:     map[string]string{runMCPTestServerEnv: "1"},
				Timeout: 5,
			},
		},
	})
	mgr.Initialize(context.Background())
	t.Cleanup(func() { _ = mgr.Close() })

	states := mgr.States()
	if len(states) != 1 {
		t.Fatalf("states: %+v", states)
	}
	if !states[0].Connected || states[0].Error != "" || states[0].Tools != 1 {
		t.Fatalf("state: %+v", states[0])
	}

	tools := mgr.Tools()
	if len(tools) != 1 {
		t.Fatalf("tools: %+v", tools)
	}
	if got := tools[0].Name(); got != "mcp__local__echo" {
		t.Fatalf("tool name = %q", got)
	}

	res, err := tools[0].Run(context.Background(), core.ToolCall{
		ID:    "call-1",
		Name:  tools[0].Name(),
		Input: `{"message":"hi"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %+v", res)
	}
	env, ok := core.ParseToolEnvelope(res.Content)
	if !ok {
		t.Fatalf("invalid envelope: %s", res.Content)
	}
	if text, _ := env.Data["text"].(string); !strings.Contains(text, "echo:hi") {
		t.Fatalf("text = %q, envelope = %+v", text, env)
	}
}

func TestManagerRecordsFailedServer(t *testing.T) {
	mgr := NewManager(Config{
		Servers: map[string]ServerConfig{
			"broken": {
				Command: "definitely-not-a-whale-mcp-command",
				Timeout: 1,
			},
		},
	})
	mgr.Initialize(context.Background())
	t.Cleanup(func() { _ = mgr.Close() })

	if tools := mgr.Tools(); len(tools) != 0 {
		t.Fatalf("tools: %+v", tools)
	}
	states := mgr.States()
	if len(states) != 1 || states[0].Error == "" || states[0].Connected {
		t.Fatalf("states: %+v", states)
	}
}

func TestManagerInitializesAndCallsStreamableHTTPToolWithHeaders(t *testing.T) {
	t.Setenv("WHALE_MCP_TEST_TOKEN", "ctx-test-token")
	server := newEchoMCPServer()
	handler := sdk.NewStreamableHTTPHandler(func(*http.Request) *sdk.Server { return server }, nil)
	var mu sync.Mutex
	var sawToken bool
	var sawStatic bool
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if r.Header.Get("CONTEXT7_API_KEY") == "ctx-test-token" {
			sawToken = true
		}
		if r.Header.Get("X-Static") == "ok" {
			sawStatic = true
		}
		mu.Unlock()
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(httpServer.Close)

	mgr := NewManager(Config{
		Servers: map[string]ServerConfig{
			"context7": {
				Type: "http",
				URL:  httpServer.URL,
				Headers: map[string]string{
					"CONTEXT7_API_KEY": "${WHALE_MCP_TEST_TOKEN}",
					"X-Static":         "ok",
				},
				Timeout: 5,
			},
		},
	})
	mgr.Initialize(context.Background())
	t.Cleanup(func() { _ = mgr.Close() })

	states := mgr.States()
	if len(states) != 1 || !states[0].Connected || states[0].Tools != 1 || states[0].Error != "" {
		t.Fatalf("states: %+v", states)
	}
	tools := mgr.Tools()
	if len(tools) != 1 {
		t.Fatalf("tools: %+v", tools)
	}
	res, err := tools[0].Run(context.Background(), core.ToolCall{
		ID:    "call-http",
		Name:  tools[0].Name(),
		Input: `{"message":"remote"}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected tool error: %+v", res)
	}
	if !strings.Contains(res.Content, "echo:remote") {
		t.Fatalf("content = %s", res.Content)
	}
	mu.Lock()
	defer mu.Unlock()
	if !sawToken || !sawStatic {
		t.Fatalf("headers not received: sawToken=%v sawStatic=%v", sawToken, sawStatic)
	}
}

func TestManagerRecordsHTTPConfigErrorWithoutLeakingHeaderValue(t *testing.T) {
	mgr := NewManager(Config{
		Servers: map[string]ServerConfig{
			"remote": {
				URL:     "http://127.0.0.1:1/mcp",
				Headers: map[string]string{"Authorization": "Bearer ${WHALE_MISSING_SECRET}"},
				Timeout: 1,
			},
		},
	})
	mgr.Initialize(context.Background())
	t.Cleanup(func() { _ = mgr.Close() })

	states := mgr.States()
	if len(states) != 1 || states[0].Error == "" || states[0].Connected {
		t.Fatalf("states: %+v", states)
	}
	if !strings.Contains(states[0].Error, "WHALE_MISSING_SECRET") {
		t.Fatalf("missing env var not named in error: %q", states[0].Error)
	}
	if strings.Contains(states[0].Error, "Bearer ") {
		t.Fatalf("error leaked header value shape: %q", states[0].Error)
	}
}
