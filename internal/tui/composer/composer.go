package composer

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	rw "github.com/mattn/go-runewidth"
	"github.com/rivo/uniseg"
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
	wasAtEnd := c.AtEnd()
	prevHeight := c.textarea.Height()
	var cmd tea.Cmd
	c.textarea, cmd = c.textarea.Update(msg)
	c.reflow()
	if wasAtEnd && c.textarea.Height() > prevHeight {
		c.realignViewportAtEnd()
	}
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
	if c.Value() == "" && c.textarea.Height() != 1 {
		copy := c.textarea
		copy.SetHeight(1)
		return copy.View()
	}
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
	height := c.visualLineCount()
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

func (c Composer) visualLineCount() int {
	width := c.textarea.Width()
	if width <= 0 {
		return len(splitComposerLines(c.Value()))
	}
	total := 0
	for _, line := range splitComposerLines(c.Value()) {
		total += wrappedLineCount([]rune(line), width)
	}
	return total
}

func wrappedLineCount(runes []rune, width int) int {
	if width <= 0 {
		return 1
	}
	var (
		lines  = 1
		row    []rune
		word   []rune
		spaces int
	)

	flushWord := func() {
		if len(word) == 0 && spaces == 0 {
			return
		}
		if spaces > 0 {
			if uniseg.StringWidth(string(row))+uniseg.StringWidth(string(word))+spaces > width {
				lines++
				row = append([]rune{}, word...)
				row = append(row, repeatSpaces(spaces)...)
			} else {
				row = append(row, word...)
				row = append(row, repeatSpaces(spaces)...)
			}
			word = nil
			spaces = 0
			return
		}
		lastCharLen := rw.RuneWidth(word[len(word)-1])
		if uniseg.StringWidth(string(word))+lastCharLen > width {
			if len(row) > 0 {
				lines++
				row = nil
			}
			row = append(row, word...)
			word = nil
		}
	}

	for _, r := range runes {
		if unicode.IsSpace(r) {
			spaces++
		} else {
			word = append(word, r)
		}
		flushWord()
	}

	if uniseg.StringWidth(string(row))+uniseg.StringWidth(string(word))+spaces >= width {
		lines++
	} else if len(word) > 0 || spaces > 0 {
		row = append(row, word...)
		row = append(row, repeatSpaces(spaces+1)...)
	}

	return lines
}

func repeatSpaces(n int) []rune {
	return []rune(strings.Repeat(" ", n))
}

func (c *Composer) realignViewportAtEnd() {
	value := c.textarea.Value()
	c.textarea.SetValue(value)
}
