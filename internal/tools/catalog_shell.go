package tools

import (
	"runtime"
	"strings"

	"github.com/usewhale/whale/internal/core"
)

func (b *Toolset) shellTools() []core.Tool {
	return []core.Tool{
		toolFn{
			name:        "shell_run",
			description: shellRunDescription(runtime.GOOS),
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
			readOnlyCheck: shellReadOnlyCheckForGOOS(runtime.GOOS),
			fn:            b.shellRun,
		},
		toolFn{
			name:        "shell_wait",
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
			fn:       b.shellWait,
		},
	}
}

var shellReadOnlyAllowPrefixesUnix = []string{
	"ls", "pwd", "echo", "cat", "head", "tail", "wc", "file", "tree", "find", "grep", "rg",
	"git status", "git diff", "git log", "git show", "git branch", "git remote", "git rev-parse", "git config --get",
	"go test", "go vet", "go version",
	"cargo test", "cargo check", "cargo clippy", "rustc --version",
	"python --version", "python3 --version", "node --version", "npm --version", "npx --version",
}

var shellReadOnlyAllowPrefixesWindows = []string{
	"dir", "cd", "echo", "type", "more", "where", "findstr", "rg",
	"git status", "git diff", "git log", "git show", "git branch", "git remote", "git rev-parse", "git config --get",
	"go test", "go vet", "go version",
	"cargo test", "cargo check", "cargo clippy", "rustc --version",
	"python --version", "py --version", "node --version", "npm --version", "npx --version",
}

func shellRunDescription(goos string) string {
	base := "shell_run runs a shell command from the current Whale workspace. Commands default to the workspace root; do not assume synthetic paths like /workspace. Use relative paths, or set cwd to a subdirectory inside the workspace, instead of prefixing commands with cd."
	if strings.EqualFold(strings.TrimSpace(goos), "windows") {
		return base + " On Windows, commands run through pwsh when available, then fall back to ComSpec or cmd.exe."
	}
	return base + " On Unix-like systems, commands run through /bin/sh."
}

func shellReadOnlyCheck(args map[string]any) bool {
	return shellReadOnlyCheckForGOOS(runtime.GOOS)(args)
}

func shellReadOnlyCheckForGOOS(goos string) func(map[string]any) bool {
	prefixes := shellReadOnlyAllowPrefixesUnix
	if strings.EqualFold(strings.TrimSpace(goos), "windows") {
		prefixes = shellReadOnlyAllowPrefixesWindows
	}
	return func(args map[string]any) bool {
		return shellReadOnlyCheckWithPrefixes(args, prefixes)
	}
}

func shellReadOnlyCheckWithPrefixes(args map[string]any, prefixes []string) bool {
	cmd, _ := args["command"].(string)
	cmd = strings.ToLower(strings.TrimSpace(cmd))
	if cmd == "" {
		return false
	}
	for _, prefix := range prefixes {
		p := strings.ToLower(strings.TrimSpace(prefix))
		if cmd == p || strings.HasPrefix(cmd, p+" ") {
			return true
		}
	}
	return false
}
