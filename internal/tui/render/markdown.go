package render

import (
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
)

func boolPtr(v bool) *bool       { return &v }
func stringPtr(v string) *string { return &v }
func uintPtr(v uint) *uint       { return &v }

func Markdown(input string, width int, quiet bool) string {
	if strings.TrimSpace(input) == "" {
		return ""
	}
	if width < 20 {
		return strings.TrimRight(input, "\n")
	}
	style := markdownStyle()
	if quiet {
		style = quietMarkdownStyle()
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return strings.TrimRight(input, "\n")
	}
	rendered, err := r.Render(input)
	if err != nil {
		return strings.TrimRight(input, "\n")
	}
	return strings.TrimRight(rendered, "\n")
}

func markdownStyle() ansi.StyleConfig {
	return ansi.StyleConfig{
		Document: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{BlockPrefix: "", BlockSuffix: ""},
			Margin:         uintPtr(0),
		},
		Heading: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{Bold: boolPtr(true)},
		},
		Strong: ansi.StylePrimitive{Bold: boolPtr(true)},
		Emph:   ansi.StylePrimitive{Italic: boolPtr(true)},
		BlockQuote: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Prefix: "│ ",
				Italic: boolPtr(true),
			},
		},
		List: ansi.StyleList{
			LevelIndent: 2,
			StyleBlock: ansi.StyleBlock{
				IndentToken: stringPtr(" "),
			},
		},
		Item: ansi.StylePrimitive{
			BlockPrefix: "• ",
		},
		Enumeration: ansi.StylePrimitive{
			BlockPrefix: ". ",
		},
		CodeBlock: ansi.StyleCodeBlock{
			StyleBlock: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{
					Prefix: "  ",
				},
				Margin: uintPtr(1),
			},
		},
		HorizontalRule: ansi.StylePrimitive{
			Format: "────────────────────────────",
		},
		Task: ansi.StyleTask{
			Ticked:   "[x] ",
			Unticked: "[ ] ",
		},
	}
}

func quietMarkdownStyle() ansi.StyleConfig {
	return ansi.StyleConfig{
		Document: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{BlockPrefix: "", BlockSuffix: ""},
			Margin:         uintPtr(0),
		},
		Heading: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{Prefix: "# "},
		},
		List: ansi.StyleList{
			LevelIndent: 2,
			StyleBlock: ansi.StyleBlock{
				IndentToken: stringPtr(" "),
			},
		},
		Item: ansi.StylePrimitive{
			BlockPrefix: "- ",
		},
		Enumeration: ansi.StylePrimitive{
			BlockPrefix: ". ",
		},
		BlockQuote: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{Prefix: "> "},
		},
		Code: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{Prefix: "`", Suffix: "`"},
		},
		CodeBlock: ansi.StyleCodeBlock{
			StyleBlock: ansi.StyleBlock{
				StylePrimitive: ansi.StylePrimitive{Prefix: "  "},
				Margin:         uintPtr(1),
			},
		},
		Link: ansi.StylePrimitive{
			Format: "{{.text}} ({{.url}})",
		},
		Task: ansi.StyleTask{
			Ticked:   "[x] ",
			Unticked: "[ ] ",
		},
	}
}
