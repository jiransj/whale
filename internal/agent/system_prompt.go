package agent

import (
	"sort"
	"strings"

	"github.com/usewhale/whale/internal/agent/planning"
	"github.com/usewhale/whale/internal/core"
	"github.com/usewhale/whale/internal/memory"
	"github.com/usewhale/whale/internal/session"
)

func (a *Agent) buildTurnProviderHistory(sessionID string, rt *memory.RuntimeState) []core.Message {
	out := rt.BuildProviderHistory()
	return out
}

func (a *Agent) buildImmutableSystemBlocks() []string {
	systemBlocks := make([]string, 0, 2)
	if a.mode == session.ModePlan {
		systemBlocks = append(systemBlocks, planning.ModeInstructions())
	} else if a.mode == session.ModeAsk {
		systemBlocks = append(systemBlocks, strings.TrimSpace(`
Ask mode is active.

- Answer questions about the codebase, architecture, behavior, bugs, and possible changes.
- You may use read-only tools, including file reads/search, read-only shell commands, and web lookup/fetch tools, when they help answer the question.
- Do not modify files, do not call mutating tools, and do not act as though you are implementing changes right now.
- If code changes are needed, explain them, summarize them, or outline them briefly instead of attempting to make them.
`))
	} else {
		systemBlocks = append(systemBlocks, strings.TrimSpace(`
Agent mode is active.

- You have access to all tools, including read-only and write tools.
- You may read, edit, and create files, run shell commands, and use all other available tools to accomplish the user's request.
- When mode restrictions blocked a previous turn, you are no longer constrained by those restrictions — carry out the request fully.
`))
	}
	if strings.TrimSpace(a.workspaceRoot) != "" {
		systemBlocks = append(systemBlocks, "Current Whale workspace root: "+a.workspaceRoot+"\nShell commands run from this directory by default. Do not assume a synthetic path such as /workspace; use relative paths or the exec_shell cwd parameter for subdirectories.")
	}
	systemBlocks = append(systemBlocks, renderToolSpecsBlock(a.tools.Specs()))
	systemBlocks = append(systemBlocks, "For branch decisions or key assumptions requiring user choice, call request_user_input instead of presenting long A/B/C prose menus.")
	if a.projectMemoryEnabled {
		if mem, ok := memory.ReadProjectMemory(a.workspaceRoot, a.projectMemoryFileOrder, a.projectMemoryMaxChars); ok {
			systemBlocks = append(systemBlocks,
				"# Project Memory\n\nThe user pinned these notes about this project. Treat them as authoritative context for this workspace:\n\n```\n"+mem.Content+"\n```",
			)
		}
	}
	return systemBlocks
}

func renderToolSpecsBlock(specs []core.ToolSpec) string {
	if len(specs) == 0 {
		return "No tools are available."
	}
	var b strings.Builder
	b.WriteString("Available tools (source of truth from registry):\n")
	for _, s := range specs {
		mode := "write"
		if s.ReadOnly {
			mode = "read-only"
		}
		b.WriteString("- ")
		b.WriteString(s.Name)
		b.WriteString(" [")
		b.WriteString(mode)
		b.WriteString("]")
		if strings.TrimSpace(s.Description) != "" {
			b.WriteString(": ")
			b.WriteString(strings.TrimSpace(s.Description))
		}
		if s.Parameters != nil {
			if propsAny, ok := s.Parameters["properties"]; ok {
				if props, ok := propsAny.(map[string]any); ok && len(props) > 0 {
					keys := make([]string, 0, len(props))
					for k := range props {
						keys = append(keys, k)
					}
					sort.Strings(keys)
					max := len(keys)
					if max > 5 {
						max = 5
					}
					b.WriteString(" args:")
					b.WriteString(strings.Join(keys[:max], ","))
				}
			}
		}
		if strings.TrimSpace(s.ApprovalHint) != "" {
			b.WriteString(" approval:")
			b.WriteString(strings.TrimSpace(s.ApprovalHint))
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
