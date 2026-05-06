package tools

import (
	"context"
	"strings"

	"github.com/usewhale/whale/internal/core"
)

func (b *Toolset) Tools() []core.Tool {
	return []core.Tool{
		toolFn{
			name:        "read_file",
			description: "Read file content under workspace root. Use this before edit/write to confirm exact text. Prefer scoped reads with offset/limit for large files instead of loading entire files.",
			parameters: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"file_path": map[string]any{"type": "string", "description": "Path relative to workspace root"},
					"offset":    map[string]any{"type": "integer", "minimum": 0, "description": "Start line offset (0-based)"},
					"limit":     map[string]any{"type": "integer", "minimum": 1, "maximum": 2000, "description": "Max lines to read"},
				},
				"required": []string{"file_path"},
			},
			readOnly: true,
			fn:       b.readFile,
		},
		toolFn{
			name:        "list_dir",
			description: "List directory entries under workspace root. Use for structure discovery before deeper reads. Not recursive; combine with grep/read_file for targeted exploration.",
			parameters: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"path":   map[string]any{"type": "string", "description": "Directory path relative to workspace root"},
					"ignore": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
			},
			readOnly: true,
			fn:       b.listDir,
		},
		toolFn{
			name:        "grep",
			description: "Search file contents recursively with ripgrep. Use for symbol/reference discovery before read_file/edit. For literal matching set literal_text=true; use include to narrow file patterns.",
			parameters: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"pattern":      map[string]any{"type": "string", "description": "Pattern or literal query"},
					"path":         map[string]any{"type": "string", "description": "Search root relative to workspace"},
					"include":      map[string]any{"type": "string", "description": "Glob include filter, e.g. *.go"},
					"literal_text": map[string]any{"type": "boolean", "description": "When true, treat pattern as plain text"},
				},
				"required": []string{"pattern"},
			},
			readOnly: true,
			fn:       b.searchContent,
		},
		toolFn{
			name:        "search_files",
			description: "Search file names and relative paths recursively under workspace root. Best for locating candidate files before read_file.",
			parameters: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"path":    map[string]any{"type": "string", "description": "Search root relative to workspace"},
					"pattern": map[string]any{"type": "string", "description": "Case-insensitive file/path pattern"},
					"limit":   map[string]any{"type": "integer", "minimum": 1, "maximum": 2000},
				},
				"required": []string{"pattern"},
			},
			readOnly: true,
			fn:       b.searchFiles,
		},
		toolFn{
			name:        "web_search",
			description: "Search the public web and return structured results. Uses DuckDuckGo HTML with Bing fallback when needed.",
			parameters: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "Search query"},
					"q":     map[string]any{"type": "string", "description": "Alias for query"},
					"search_query": map[string]any{
						"type":        "array",
						"description": "Compatibility format: [{\"q\":\"...\", \"max_results\": 5}]",
						"items": map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"properties": map[string]any{
								"q":           map[string]any{"type": "string"},
								"query":       map[string]any{"type": "string"},
								"max_results": map[string]any{"type": "integer"},
							},
						},
					},
					"max_results": map[string]any{"type": "integer", "minimum": 1, "maximum": 10},
					"timeout_ms":  map[string]any{"type": "integer", "minimum": 1, "maximum": 60000},
				},
			},
			readOnly: true,
			fn:       b.webSearch,
		},
		toolFn{
			name:        "fetch",
			description: "Fetch a URL and return content. Supports text|markdown|html output formats with timeout and truncation control.",
			parameters: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"url":        map[string]any{"type": "string", "description": "Target URL (http/https)"},
					"format":     map[string]any{"type": "string", "enum": []string{"text", "markdown", "html"}},
					"timeout_ms": map[string]any{"type": "integer", "minimum": 1, "maximum": 60000},
				},
				"required": []string{"url"},
			},
			readOnly: true,
			fn:       b.fetch,
		},
		toolFn{
			name:        "web_fetch",
			description: "Fetch a web page and extract readable text content plus page title.",
			parameters: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"url":        map[string]any{"type": "string", "description": "Target URL (http/https)"},
					"timeout_ms": map[string]any{"type": "integer", "minimum": 1, "maximum": 60000},
				},
				"required": []string{"url"},
			},
			readOnly: true,
			fn:       b.webFetch,
		},
		toolFn{
			name:        "request_user_input",
			description: "Request user input for one to three short questions and wait for the response. Use this for branch decisions and key assumptions.",
			parameters: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"questions": map[string]any{
						"type":        "array",
						"minItems":    1,
						"maxItems":    3,
						"description": "Questions to show the user. Prefer one.",
						"items": map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"properties": map[string]any{
								"id":       map[string]any{"type": "string"},
								"header":   map[string]any{"type": "string"},
								"question": map[string]any{"type": "string"},
								"options": map[string]any{
									"type":     "array",
									"minItems": 2,
									"maxItems": 3,
									"items": map[string]any{
										"type":                 "object",
										"additionalProperties": false,
										"properties": map[string]any{
											"label":       map[string]any{"type": "string"},
											"description": map[string]any{"type": "string"},
										},
										"required": []string{"label", "description"},
									},
								},
							},
							"required": []string{"id", "header", "question", "options"},
						},
					},
				},
				"required": []string{"questions"},
			},
			readOnly: true,
			fn:       b.requestUserInputPlaceholder,
		},
		toolFn{
			name:        "edit",
			description: "Apply SEARCH/REPLACE edits to an existing file. Requires exact search text; returns error when search is not found. Prefer this for surgical changes over full-file rewrites.",
			parameters: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"file_path": map[string]any{"type": "string", "description": "Target file path relative to workspace"},
					"search":    map[string]any{"type": "string", "description": "Exact text to replace"},
					"replace":   map[string]any{"type": "string", "description": "Replacement text"},
					"all":       map[string]any{"type": "boolean", "description": "Replace all occurrences"},
				},
				"required": []string{"file_path", "search", "replace"},
			},
			fn: b.editFile,
		},
		toolFn{
			name:        "write",
			description: "Write full file content under workspace root (create or overwrite). Use for new files or intentional full rewrites. For partial modifications, prefer edit.",
			parameters: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"file_path": map[string]any{"type": "string", "description": "Target file path relative to workspace"},
					"content":   map[string]any{"type": "string", "description": "Full file content to write"},
				},
				"required": []string{"file_path", "content"},
			},
			fn: b.writeFile,
		},
		toolFn{
			name:        "apply_patch",
			description: "Apply structured multi-file patch text. Supports *** Begin Patch format with Update/Add/Delete blocks and @@ hunks. Prefer this for coordinated edits across files.",
			parameters: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"patch": map[string]any{"type": "string", "description": "Patch text in *** Begin Patch format"},
				},
				"required": []string{"patch"},
			},
			fn: b.applyPatch,
		},
		toolFn{
			name:        "exec_shell",
			description: "Run a shell command from the current Whale workspace. Commands default to the workspace root; do not assume synthetic paths like /workspace. Use relative paths, or set cwd to a subdirectory inside the workspace, instead of prefixing commands with cd.",
			parameters: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"command":    map[string]any{"type": "string", "description": "Shell command to execute"},
					"timeout_ms": map[string]any{"type": "integer", "minimum": 1, "maximum": maxBackgroundShellTimeoutMS, "description": "Command timeout in milliseconds"},
					"background": map[string]any{"type": "boolean", "description": "When true, return immediately with task_id"},
					"cwd":        map[string]any{"type": "string", "description": "Optional working directory relative to the workspace root. Must stay inside the workspace. Use this for subdirectory commands instead of cd."},
				},
				"required": []string{"command"},
			},
			readOnlyCheck: shellReadOnlyCheck,
			fn:            b.execShell,
		},
		toolFn{
			name:        "exec_shell_wait",
			description: "Wait for a background shell task by task_id and return status plus captured output when complete.",
			parameters: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"task_id":    map[string]any{"type": "string"},
					"timeout_ms": map[string]any{"type": "integer", "minimum": 1, "maximum": 120000},
				},
				"required": []string{"task_id"},
			},
			readOnly: true,
			fn:       b.execShellWait,
		},
		toolFn{
			name:        "todo_add",
			description: "Add a todo item to current session checklist.",
			parameters: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"text":     map[string]any{"type": "string"},
					"priority": map[string]any{"type": "integer", "minimum": 0, "maximum": 9},
				},
				"required": []string{"text"},
			},
			readOnly: true,
			fn:       b.sessionRuntimePlaceholder,
		},
		toolFn{
			name:        "todo_list",
			description: "List current session todo items.",
			parameters: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"include_done": map[string]any{"type": "boolean"},
				},
			},
			readOnly: true,
			fn:       b.sessionRuntimePlaceholder,
		},
		toolFn{
			name:        "todo_update",
			description: "Update a todo item fields such as done/text/priority.",
			parameters: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"id":       map[string]any{"type": "string"},
					"text":     map[string]any{"type": "string"},
					"done":     map[string]any{"type": "boolean"},
					"priority": map[string]any{"type": "integer", "minimum": 0, "maximum": 9},
				},
				"required": []string{"id"},
			},
			readOnly: true,
			fn:       b.sessionRuntimePlaceholder,
		},
		toolFn{
			name:        "todo_remove",
			description: "Remove a todo item by id.",
			parameters: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"id": map[string]any{"type": "string"},
				},
				"required": []string{"id"},
			},
			readOnly: true,
			fn:       b.sessionRuntimePlaceholder,
		},
		toolFn{
			name:        "todo_clear_done",
			description: "Remove all completed todo items from current session checklist.",
			parameters: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties":           map[string]any{},
			},
			readOnly: true,
			fn:       b.sessionRuntimePlaceholder,
		},
	}
}

func (b *Toolset) sessionRuntimePlaceholder(_ context.Context, call core.ToolCall) (core.ToolResult, error) {
	return marshalToolError(call, "tool_unavailable", "session runtime tool is handled by agent runtime"), nil
}

func (b *Toolset) requestUserInputPlaceholder(_ context.Context, call core.ToolCall) (core.ToolResult, error) {
	return marshalToolError(call, "tool_unavailable", "request_user_input is handled by agent runtime"), nil
}

type toolFn struct {
	name          string
	description   string
	parameters    map[string]any
	readOnly      bool
	readOnlyCheck func(args map[string]any) bool
	fn            func(context.Context, core.ToolCall) (core.ToolResult, error)
}

func (t toolFn) Name() string               { return t.name }
func (t toolFn) Description() string        { return t.description }
func (t toolFn) Parameters() map[string]any { return t.parameters }
func (t toolFn) ReadOnly() bool             { return t.readOnly }
func (t toolFn) ReadOnlyCheck(args map[string]any) bool {
	if t.readOnlyCheck == nil {
		return t.readOnly
	}
	return t.readOnlyCheck(args)
}
func (t toolFn) Run(ctx context.Context, call core.ToolCall) (core.ToolResult, error) {
	return t.fn(ctx, call)
}

var shellReadOnlyAllowPrefixes = []string{
	"ls", "pwd", "echo", "cat", "head", "tail", "wc", "file", "tree", "find", "grep", "rg",
	"git status", "git diff", "git log", "git show", "git branch", "git remote", "git rev-parse", "git config --get",
	"go test", "go vet", "go version",
	"cargo test", "cargo check", "cargo clippy", "rustc --version",
	"python --version", "python3 --version", "node --version", "npm --version", "npx --version",
}

func shellReadOnlyCheck(args map[string]any) bool {
	cmd, _ := args["command"].(string)
	cmd = strings.ToLower(strings.TrimSpace(cmd))
	if cmd == "" {
		return false
	}
	for _, prefix := range shellReadOnlyAllowPrefixes {
		p := strings.ToLower(strings.TrimSpace(prefix))
		if cmd == p || strings.HasPrefix(cmd, p+" ") {
			return true
		}
	}
	return false
}
