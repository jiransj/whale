package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseContent(t *testing.T) {
	t.Parallel()

	skill, err := ParseContent([]byte("\n---\nname: test-skill\ndescription: Use this skill for tests.\n---\n\n# Test Skill\n\nInstructions here.\n"))
	if err != nil {
		t.Fatalf("ParseContent failed: %v", err)
	}
	if skill.Name != "test-skill" {
		t.Fatalf("unexpected name: %q", skill.Name)
	}
	if skill.Description != "Use this skill for tests." {
		t.Fatalf("unexpected description: %q", skill.Description)
	}
	if skill.Instructions != "# Test Skill\n\nInstructions here." {
		t.Fatalf("unexpected instructions: %q", skill.Instructions)
	}
}

func TestParseContentRequiresFrontmatter(t *testing.T) {
	t.Parallel()

	if _, err := ParseContent([]byte("# Just Markdown")); err == nil {
		t.Fatal("expected missing frontmatter error")
	}
}

func TestSkillValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		skill   Skill
		wantErr string
	}{
		{
			name:  "valid",
			skill: Skill{Name: "test-skill", Description: "desc", Path: "/tmp/test-skill"},
		},
		{
			name:    "missing name",
			skill:   Skill{Description: "desc"},
			wantErr: "name is required",
		},
		{
			name:    "invalid name",
			skill:   Skill{Name: "-bad", Description: "desc"},
			wantErr: "alphanumeric with hyphens",
		},
		{
			name:    "missing description",
			skill:   Skill{Name: "test-skill", Path: "/tmp/test-skill"},
			wantErr: "description is required",
		},
		{
			name:    "directory mismatch",
			skill:   Skill{Name: "test-skill", Description: "desc", Path: "/tmp/other"},
			wantErr: "must match directory",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.skill.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestDiscoverDeduplicatesWithEarlierRootWinning(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	global := t.TempDir()
	writeSkill(t, filepath.Join(workspace, "shared"), "shared", "Workspace skill.", "# Workspace")
	writeSkill(t, filepath.Join(global, "shared"), "shared", "Global skill.", "# Global")
	writeSkill(t, filepath.Join(global, "other"), "other", "Other skill.", "# Other")

	discovered, states := DiscoverWithStates([]string{workspace, global})
	if len(states) != 3 {
		t.Fatalf("expected 3 states, got %d", len(states))
	}
	if names := skillNames(discovered); strings.Join(names, ",") != "other,shared" {
		t.Fatalf("unexpected names: %v", names)
	}
	shared, _, ok := Find([]string{workspace, global}, "shared")
	if !ok {
		t.Fatal("expected shared skill")
	}
	if !strings.Contains(shared.Instructions, "Workspace") {
		t.Fatalf("expected workspace skill to win, got %q", shared.Instructions)
	}
}

func TestDiscoverSkipsMissingRootAndInvalidSkill(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeSkill(t, filepath.Join(root, "valid"), "valid", "Valid skill.", "# Valid")
	writeSkill(t, filepath.Join(root, "invalid-dir"), "wrong-name", "Invalid skill.", "# Invalid")

	discovered, states := DiscoverWithStates([]string{filepath.Join(root, "missing"), root})
	if names := skillNames(discovered); strings.Join(names, ",") != "valid" {
		t.Fatalf("unexpected names: %v", names)
	}
	var errorStates int
	for _, st := range states {
		if st.State == StateError {
			errorStates++
		}
	}
	if errorStates != 1 {
		t.Fatalf("expected one invalid skill state, got %d", errorStates)
	}
}

func TestDefaultRootsIncludesWorkspaceBeforeHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workspace := t.TempDir()
	roots := DefaultRoots(workspace)
	if len(roots) != 4 {
		t.Fatalf("expected 4 roots, got %v", roots)
	}
	wantPrefix := []string{
		filepath.Join(workspace, ".whale", "skills"),
		filepath.Join(workspace, ".agents", "skills"),
		filepath.Join(home, ".whale", "skills"),
		filepath.Join(home, ".agents", "skills"),
	}
	for i, want := range wantPrefix {
		if roots[i] != want {
			t.Fatalf("root[%d] = %q, want %q", i, roots[i], want)
		}
	}
}

func TestRenderAvailableSkillsDoesNotIncludeInstructions(t *testing.T) {
	t.Parallel()

	rendered := RenderAvailableSkills([]*Skill{{
		Name:          "test-skill",
		Description:   "Use this for tests.",
		Instructions:  "secret instructions",
		SkillFilePath: "/skills/test-skill/SKILL.md",
	}})
	if !strings.Contains(rendered, "test-skill") || !strings.Contains(rendered, "load_skill") {
		t.Fatalf("unexpected rendered skills: %q", rendered)
	}
	for _, want := range []string{"follow the delegation policy first", "do not load a skill unless the user also names one", "Do not browse skill file paths with ordinary file tools"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered skill index missing %q: %q", want, rendered)
		}
	}
	if strings.Contains(rendered, "secret instructions") {
		t.Fatalf("rendered index should not include full instructions: %q", rendered)
	}
}

func TestApproxTokenCount(t *testing.T) {
	t.Parallel()

	if ApproxTokenCount("") != 0 {
		t.Fatal("empty string should have zero tokens")
	}
	if ApproxTokenCount("abcde") != 2 {
		t.Fatalf("unexpected token estimate")
	}
}

func TestFilter(t *testing.T) {
	t.Parallel()

	all := []*Skill{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	filtered := Filter(all, []string{"b"})
	if names := skillNames(filtered); strings.Join(names, ",") != "a,c" {
		t.Fatalf("unexpected filtered names: %v", names)
	}
}

func writeSkill(t *testing.T, dir, name, desc, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	content := "---\nname: " + name + "\ndescription: " + desc + "\n---\n\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, SkillFileName), []byte(content), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
}

func skillNames(all []*Skill) []string {
	names := make([]string, 0, len(all))
	for _, skill := range all {
		names = append(names, skill.Name)
	}
	return names
}
