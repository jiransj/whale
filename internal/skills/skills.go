// Package skills discovers and parses local Agent Skills.
package skills

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	SkillFileName        = "SKILL.md"
	MaxNameLength        = 64
	MaxDescriptionLength = 1024
)

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9]+(-[a-zA-Z0-9]+)*$`)

// Skill represents a parsed SKILL.md file.
type Skill struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	Instructions  string `json:"instructions"`
	Path          string `json:"path"`
	SkillFilePath string `json:"skill_file_path"`
}

// DiscoveryState represents the outcome of discovering a single skill file.
type DiscoveryState int

const (
	// StateNormal indicates the skill was parsed and validated successfully.
	StateNormal DiscoveryState = iota
	// StateError indicates discovery encountered a scan/parse/validate error.
	StateError
)

// SkillState represents the latest discovery status of a skill file.
type SkillState struct {
	Name  string
	Path  string
	State DiscoveryState
	Err   error
}

// DefaultRoots returns the skill discovery roots for a workspace.
func DefaultRoots(workspaceRoot string) []string {
	var roots []string
	if root := strings.TrimSpace(workspaceRoot); root != "" {
		roots = append(roots,
			filepath.Join(root, ".whale", "skills"),
			filepath.Join(root, ".agents", "skills"),
		)
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		roots = append(roots,
			filepath.Join(home, ".whale", "skills"),
			filepath.Join(home, ".agents", "skills"),
		)
	}
	return uniqueCleanPaths(roots)
}

// ValidName reports whether name is a valid skill name.
func ValidName(name string) bool {
	name = strings.TrimSpace(name)
	return name != "" && len(name) <= MaxNameLength && namePattern.MatchString(name)
}

// Validate checks if the skill meets Whale's v1 skill requirements.
func (s *Skill) Validate() error {
	var errs []error
	if s.Name == "" {
		errs = append(errs, errors.New("name is required"))
	} else {
		if len(s.Name) > MaxNameLength {
			errs = append(errs, fmt.Errorf("name exceeds %d characters", MaxNameLength))
		}
		if !namePattern.MatchString(s.Name) {
			errs = append(errs, errors.New("name must be alphanumeric with hyphens, no leading/trailing/consecutive hyphens"))
		}
		if s.Path != "" && !strings.EqualFold(filepath.Base(s.Path), s.Name) {
			errs = append(errs, fmt.Errorf("name %q must match directory %q", s.Name, filepath.Base(s.Path)))
		}
	}
	if s.Description == "" {
		errs = append(errs, errors.New("description is required"))
	} else if len(s.Description) > MaxDescriptionLength {
		errs = append(errs, fmt.Errorf("description exceeds %d characters", MaxDescriptionLength))
	}
	return errors.Join(errs...)
}

// Parse parses a SKILL.md file from disk.
func Parse(path string) (*Skill, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	skill, err := ParseContent(content)
	if err != nil {
		return nil, err
	}
	skill.Path = filepath.Dir(path)
	skill.SkillFilePath = path
	return skill, nil
}

// ParseContent parses a SKILL.md from raw bytes.
func ParseContent(content []byte) (*Skill, error) {
	frontmatter, body, err := splitFrontmatter(string(content))
	if err != nil {
		return nil, err
	}
	values := parseFlatFrontmatter(frontmatter)
	skill := &Skill{
		Name:         values["name"],
		Description:  values["description"],
		Instructions: strings.TrimSpace(body),
	}
	return skill, nil
}

// Discover finds all valid skills in the given roots. Earlier roots take
// precedence when multiple skills have the same name.
func Discover(roots []string) []*Skill {
	skills, _ := DiscoverWithStates(roots)
	return skills
}

// DiscoverWithStates finds all valid skills and returns parse/validation
// states for diagnostics.
func DiscoverWithStates(roots []string) ([]*Skill, []*SkillState) {
	var all []*Skill
	var states []*SkillState
	seenFiles := map[string]bool{}
	for _, root := range uniqueCleanPaths(roots) {
		if strings.TrimSpace(root) == "" {
			continue
		}
		if _, err := os.Stat(root); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			states = append(states, &SkillState{Path: root, State: StateError, Err: err})
			continue
		}
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				states = append(states, &SkillState{Path: path, State: StateError, Err: err})
				return nil
			}
			if d.IsDir() || d.Name() != SkillFileName {
				return nil
			}
			abs, err := filepath.Abs(path)
			if err != nil {
				states = append(states, &SkillState{Path: path, State: StateError, Err: err})
				return nil
			}
			if seenFiles[abs] {
				return nil
			}
			seenFiles[abs] = true
			skill, err := Parse(abs)
			if err != nil {
				states = append(states, &SkillState{Path: abs, State: StateError, Err: err})
				return nil
			}
			if err := skill.Validate(); err != nil {
				states = append(states, &SkillState{Name: skill.Name, Path: abs, State: StateError, Err: err})
				return nil
			}
			all = append(all, skill)
			states = append(states, &SkillState{Name: skill.Name, Path: abs, State: StateNormal})
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			states = append(states, &SkillState{Path: root, State: StateError, Err: err})
		}
	}
	return Sort(Deduplicate(all)), states
}

// Find returns a skill by exact name.
func Find(roots []string, name string) (*Skill, []*SkillState, bool) {
	discovered, states := DiscoverWithStates(roots)
	for _, skill := range discovered {
		if skill.Name == name {
			return skill, states, true
		}
	}
	return nil, states, false
}

// Sort returns a copy sorted by lowercase skill name.
func Sort(all []*Skill) []*Skill {
	out := append([]*Skill(nil), all...)
	sort.SliceStable(out, func(i, j int) bool {
		left := strings.ToLower(out[i].Name)
		right := strings.ToLower(out[j].Name)
		if left == right {
			return out[i].Name < out[j].Name
		}
		return left < right
	})
	return out
}

// Deduplicate removes duplicate skills by name. The first occurrence wins,
// so callers can pass roots in priority order.
func Deduplicate(all []*Skill) []*Skill {
	seen := map[string]bool{}
	out := make([]*Skill, 0, len(all))
	for _, skill := range all {
		if skill == nil || seen[skill.Name] {
			continue
		}
		seen[skill.Name] = true
		out = append(out, skill)
	}
	return out
}

// RenderAvailableSkills renders a compact system-prompt index.
func RenderAvailableSkills(all []*Skill) string {
	all = Sort(all)
	if len(all) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Available skills:\n")
	for _, skill := range all {
		b.WriteString("- ")
		b.WriteString(skill.Name)
		if strings.TrimSpace(skill.Description) != "" {
			b.WriteString(": ")
			b.WriteString(strings.TrimSpace(skill.Description))
		}
		if strings.TrimSpace(skill.SkillFilePath) != "" {
			b.WriteString(" (file: ")
			b.WriteString(skill.SkillFilePath)
			b.WriteString(")")
		}
		b.WriteString("\n")
	}
	b.WriteString("\nUsers can invoke a skill with a leading $skill-name mention. Use load_skill when the user explicitly mentions a skill, or when no direct tool/delegation path is clearly requested and the task strongly matches a skill. If the user explicitly asks for subagent, delegation, or parallel work, follow the delegation policy first and do not load a skill unless the user also names one. The index above is metadata only; load the skill before relying on its instructions. Do not browse skill file paths with ordinary file tools.")
	return strings.TrimSpace(b.String())
}

// ApproxTokenCount returns a rough estimate using a common ~4 chars/token heuristic.
func ApproxTokenCount(s string) int {
	if s == "" {
		return 0
	}
	return (len(s) + 3) / 4
}

// Filter removes skills whose names appear in the disabled list.
func Filter(all []*Skill, disabled []string) []*Skill {
	if len(disabled) == 0 {
		return all
	}
	disabledSet := make(map[string]bool, len(disabled))
	for _, name := range disabled {
		disabledSet[name] = true
	}
	out := make([]*Skill, 0, len(all))
	for _, skill := range all {
		if skill != nil && !disabledSet[skill.Name] {
			out = append(out, skill)
		}
	}
	return out
}

func splitFrontmatter(content string) (frontmatter, body string, err error) {
	content = strings.TrimPrefix(content, "\uFEFF")
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	lines := strings.Split(content, "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) != "" {
			start = i
			break
		}
	}
	if start == -1 || strings.TrimSpace(lines[start]) != "---" {
		return "", "", errors.New("no YAML frontmatter found")
	}
	end := -1
	for i := start + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return "", "", errors.New("unclosed frontmatter")
	}
	return strings.Join(lines[start+1:end], "\n"), strings.Join(lines[end+1:], "\n"), nil
}

func parseFlatFrontmatter(frontmatter string) map[string]string {
	values := map[string]string{}
	for _, line := range strings.Split(frontmatter, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key == "name" || key == "description" {
			values[key] = value
		}
	}
	return values
}

func uniqueCleanPaths(paths []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		clean, err := filepath.Abs(filepath.Clean(path))
		if err != nil {
			clean = filepath.Clean(path)
		}
		if seen[clean] {
			continue
		}
		seen[clean] = true
		out = append(out, clean)
	}
	return out
}
