package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/usewhale/whale/internal/core"
)

func TestImmutableSystemPromptIncludesSkillIndexOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workspace := t.TempDir()
	skillDir := filepath.Join(workspace, ".whale", "skills", "prompt-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: prompt-skill\ndescription: Prompt skill.\n---\n\n# Prompt Skill\n\nDo not inline this body.\n"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	a := &Agent{
		tools:                  core.NewToolRegistry(nil),
		workspaceRoot:          workspace,
		projectMemoryEnabled:   false,
		projectMemoryFileOrder: nil,
	}
	blocks := a.buildImmutableSystemBlocks()
	joined := strings.Join(blocks, "\n\n")
	if !strings.Contains(joined, "Available skills") || !strings.Contains(joined, "prompt-skill") || !strings.Contains(joined, "load_skill") {
		t.Fatalf("missing skill index in system prompt:\n%s", joined)
	}
	if strings.Contains(joined, "Do not inline this body") {
		t.Fatalf("system prompt should not inline skill instructions:\n%s", joined)
	}
}

func TestImmutableSystemPromptIncludesDelegationPolicyBeforeToolSpecs(t *testing.T) {
	a := &Agent{
		tools:                core.NewToolRegistry(nil),
		projectMemoryEnabled: false,
	}
	blocks := a.buildImmutableSystemBlocks()
	joined := strings.Join(blocks, "\n\n")
	policyIx := strings.Index(joined, "Delegation policy.")
	toolIx := strings.Index(joined, "No tools are available.")
	if policyIx < 0 {
		t.Fatalf("missing delegation policy:\n%s", joined)
	}
	if toolIx < 0 {
		t.Fatalf("missing tool specs block:\n%s", joined)
	}
	if policyIx > toolIx {
		t.Fatalf("delegation policy should appear before tool specs:\n%s", joined)
	}
	for _, want := range []string{"Use parallel_reason for 2-8 independent", "Use spawn_subagent for one bounded read-only", "Use a single agent for direct questions", "Do not load a skill first unless the user explicitly names one"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("delegation policy missing %q:\n%s", want, joined)
		}
	}
}
