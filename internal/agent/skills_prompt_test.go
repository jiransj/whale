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
