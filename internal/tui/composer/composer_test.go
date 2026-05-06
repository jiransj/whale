package composer

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestComposerCtrlJInsertsNewline(t *testing.T) {
	c := New()
	c.SetValue("hello")
	if !c.HandleKey(tea.KeyMsg{Type: tea.KeyCtrlJ}) {
		t.Fatal("expected ctrl+j to be handled")
	}
	if got := c.Value(); got != "hello\n" {
		t.Fatalf("unexpected value: %q", got)
	}
}

func TestComposerMultilinePromptOnlyMarksFirstLine(t *testing.T) {
	c := New()
	c.SetValue("hello\nworld")
	view := c.View()
	if strings.Count(view, "›") != 1 {
		t.Fatalf("expected only first line to use prompt glyph, got %q", view)
	}
	if !strings.Contains(view, "\n  world") {
		t.Fatalf("expected continuation line indentation, got %q", view)
	}
}

func TestComposerCtrlUClearsWholeBuffer(t *testing.T) {
	c := New()
	c.SetValue("one\ntwo")
	if !c.HandleKey(tea.KeyMsg{Type: tea.KeyCtrlU}) {
		t.Fatal("expected ctrl+u to be handled")
	}
	if got := c.Value(); got != "" {
		t.Fatalf("expected empty buffer, got %q", got)
	}
}

func TestComposerCtrlWDeletesPreviousWord(t *testing.T) {
	c := New()
	c.SetValue("hello world")
	c.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	if got := c.Value(); got != "hello " {
		t.Fatalf("unexpected value: %q", got)
	}
}

func TestComposerCtrlAAndCtrlEMoveWithinCurrentLine(t *testing.T) {
	c := New()
	c.SetValue("abc\ndef")
	c.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	if got := c.Value(); got != "abc\nXdef" {
		t.Fatalf("expected insert at second line start, got %q", got)
	}
	c.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Y")})
	if got := c.Value(); got != "abc\nXdefY" {
		t.Fatalf("expected insert at second line end, got %q", got)
	}
}

func TestComposerPgUpPgDownJumpBuffer(t *testing.T) {
	c := New()
	c.SetValue("first\nsecond\nthird")
	c.HandleKey(tea.KeyMsg{Type: tea.KeyPgUp})
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("A")})
	if got := c.Value(); got != "Afirst\nsecond\nthird" {
		t.Fatalf("expected insert at buffer start, got %q", got)
	}
	c.HandleKey(tea.KeyMsg{Type: tea.KeyPgDown})
	c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Z")})
	if got := c.Value(); got != "Afirst\nsecond\nthirdZ" {
		t.Fatalf("expected insert at buffer end, got %q", got)
	}
}

func TestComposerFoldedViewKeepsFullContentHint(t *testing.T) {
	c := New()
	lines := make([]string, 25)
	for i := range lines {
		lines[i] = "line"
	}
	c.SetValue(strings.Join(lines, "\n"))
	view := c.View()
	if !strings.Contains(view, "lines hidden - full content kept") {
		t.Fatalf("expected folded full-content hint, got %q", view)
	}
	if !strings.Contains(view, "25 lines") {
		t.Fatalf("expected line-count hint, got %q", view)
	}
	if got := c.Value(); strings.Count(got, "\n")+1 != 25 {
		t.Fatalf("folded view should not alter buffer, got %d lines", strings.Count(got, "\n")+1)
	}
}

func TestComposerTwentyLinesRenderWithoutFoldHint(t *testing.T) {
	c := New()
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = "line"
	}
	c.SetValue(strings.Join(lines, "\n"))
	view := c.View()
	if strings.Contains(view, "lines hidden") {
		t.Fatalf("did not expect folded hint for 20 lines, got %q", view)
	}
	if c.Height() != 20 {
		t.Fatalf("expected textarea height to grow to 20, got %d", c.Height())
	}
}
