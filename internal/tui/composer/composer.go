package composer

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	tuitheme "github.com/usewhale/whale/internal/tui/theme"
)

const (
	composerCollapseThreshold = 20
	composerHeadLines         = 3
	composerTailLines         = 2
)

type Composer struct {
	textarea textarea.Model
	width    int
}

func New() Composer {
	ta := textarea.New()
	ta.Placeholder = "Type message or command"
	ta.Prompt = "› "
	ta.SetPromptFunc(2, func(lineIdx int) string {
		if lineIdx == 0 {
			return "› "
		}
		return "  "
	})
	ta.ShowLineNumbers = false
	ta.CharLimit = 20000
	ta.MaxHeight = composerCollapseThreshold
	ta.SetWidth(80)
	ta.SetHeight(1)
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.BlurredStyle.CursorLine = lipgloss.NewStyle()
	ta.Focus()
	return Composer{textarea: ta, width: 80}
}

func (c Composer) Value() string {
	return c.textarea.Value()
}

func (c *Composer) SetValue(value string) {
	c.textarea.SetValue(value)
	c.moveToEnd()
	c.reflow()
}

func (c *Composer) Reset() {
	c.textarea.Reset()
	c.reflow()
}

func (c *Composer) SetCursorEnd() {
	c.moveToEnd()
}

func (c *Composer) SetWidth(width int) {
	c.width = max(20, width)
	c.textarea.SetWidth(max(16, c.width-2))
	c.reflow()
}

func (c *Composer) InsertNewline() {
	c.textarea.InsertRune('\n')
	c.reflow()
}

func (c *Composer) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	c.textarea, cmd = c.textarea.Update(msg)
	c.reflow()
	return cmd
}

func (c *Composer) HandleKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "ctrl+u":
		c.Reset()
		return true
	case "ctrl+p", "ctrl+n":
		return false
	case "ctrl+j", "shift+enter":
		c.InsertNewline()
		return true
	case "pgup":
		for c.textarea.Line() > 0 {
			c.textarea.CursorUp()
		}
		c.textarea.CursorStart()
		return true
	case "pgdown":
		c.moveToEnd()
		return true
	}
	return false
}

func (c Composer) View() string {
	value := c.Value()
	lines := splitComposerLines(value)
	if len(lines) <= composerCollapseThreshold {
		return c.textarea.View()
	}
	return c.foldedView(lines)
}

func (c Composer) LineCount() int {
	return c.textarea.LineCount()
}

func (c Composer) Height() int {
	return c.textarea.Height()
}

func (c Composer) AtStart() bool {
	return c.textarea.Line() == 0 && c.textarea.LineInfo().ColumnOffset == 0
}

func (c Composer) AtEnd() bool {
	if c.textarea.Line() != c.textarea.LineCount()-1 {
		return false
	}
	info := c.textarea.LineInfo()
	line := splitComposerLines(c.Value())[c.textarea.Line()]
	return info.StartColumn+info.ColumnOffset >= len([]rune(line))
}

func (c *Composer) moveToEnd() {
	for c.textarea.Line() < c.textarea.LineCount()-1 {
		c.textarea.CursorDown()
	}
	c.textarea.CursorEnd()
}

func (c *Composer) reflow() {
	lines := splitComposerLines(c.Value())
	height := len(lines)
	if height < 1 {
		height = 1
	}
	if height > composerCollapseThreshold {
		height = composerCollapseThreshold
	}
	c.textarea.SetHeight(height)
}

func (c Composer) foldedView(lines []string) string {
	cursorLine := c.textarea.Line()
	keep := map[int]bool{}
	for i := 0; i < composerHeadLines && i < len(lines); i++ {
		keep[i] = true
	}
	for i := max(0, len(lines)-composerTailLines); i < len(lines); i++ {
		keep[i] = true
	}
	if cursorLine >= 0 && cursorLine < len(lines) {
		keep[cursorLine] = true
	}

	out := make([]string, 0, composerHeadLines+composerTailLines+4)
	prev := -1
	for i := 0; i < len(lines); i++ {
		if !keep[i] {
			continue
		}
		if i-prev > 1 {
			out = append(out, c.hiddenLine(i-prev-1))
		}
		out = append(out, c.promptLine(lines[i], i == 0, i == cursorLine))
		prev = i
	}
	out = append(out, c.hintLine(len(lines)))
	return strings.Join(out, "\n")
}

func (c Composer) promptLine(line string, first bool, cursor bool) string {
	prefix := "  "
	if first {
		prefix = lipgloss.NewStyle().Foreground(tuitheme.Default.Accent).Bold(true).Render("›") + " "
	}
	if cursor {
		info := c.textarea.LineInfo()
		col := info.StartColumn + info.ColumnOffset
		runes := []rune(line)
		if col < 0 {
			col = 0
		}
		if col > len(runes) {
			col = len(runes)
		}
		line = string(runes[:col]) + "█" + string(runes[col:])
	}
	return prefix + line
}

func (c Composer) hiddenLine(n int) string {
	return lipgloss.NewStyle().
		Foreground(tuitheme.Default.Muted).
		Render(fmt.Sprintf("  [… %d lines hidden - full content kept …]", n))
}

func (c Composer) hintLine(n int) string {
	return lipgloss.NewStyle().
		Foreground(tuitheme.Default.Muted).
		Render(fmt.Sprintf("  [%d lines · PgUp/PgDn jump · Ctrl+U clear · Ctrl+W del word]", n))
}

func splitComposerLines(value string) []string {
	if value == "" {
		return []string{""}
	}
	return strings.Split(value, "\n")
}
