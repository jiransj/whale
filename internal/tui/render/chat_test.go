package render

import (
	"strings"
	"testing"
)

func TestChatLines_MarkdownBoldAndList(t *testing.T) {
	entries := []UIMessage{
		{Role: "assistant", Kind: KindText, Text: "Hello **world**\n- one\n- two"},
	}
	lines := ChatLines(entries, 80)
	if len(lines) == 0 {
		t.Fatalf("expected rendered lines")
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "world") {
		t.Fatalf("expected markdown content, got: %q", joined)
	}
	if !strings.Contains(joined, "one") || !strings.Contains(joined, "two") {
		t.Fatalf("expected list items rendered, got: %q", joined)
	}
}

func TestChatLines_ThinkingCardHasDistinctLabel(t *testing.T) {
	entries := []UIMessage{
		{Role: "think", Kind: KindThinking, Text: "I should answer carefully."},
	}
	lines := ChatLines(entries, 80)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "Thinking") {
		t.Fatalf("expected thinking label, got: %q", joined)
	}
	if !strings.Contains(joined, "I should answer carefully.") {
		t.Fatalf("expected reasoning body, got: %q", joined)
	}
}

func TestChatLines_UserPromptGlyphAndContinuationIndent(t *testing.T) {
	entries := []UIMessage{
		{Role: "you", Kind: KindText, Text: "first line\nsecond line"},
	}
	lines := ChatLines(entries, 80)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "› first line") {
		t.Fatalf("expected user prompt glyph, got: %q", joined)
	}
	if !strings.Contains(joined, "\n  second line") {
		t.Fatalf("expected continuation indent, got: %q", joined)
	}
	if strings.Contains(joined, "┃") || strings.Contains(joined, "│") {
		t.Fatalf("user prompt should not render as a bordered card: %q", joined)
	}
}

func TestChatLines_NoticeRendersAsPlainHint(t *testing.T) {
	entries := []UIMessage{
		{Role: "notice", Kind: KindNotice, Text: "✔ You approved whale to run uptime this time"},
	}
	lines := ChatLines(entries, 80)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "✔ You approved whale") {
		t.Fatalf("expected notice text, got: %q", joined)
	}
	if strings.Contains(joined, "┃") || strings.Contains(joined, "│") {
		t.Fatalf("notice should not render as a bordered card: %q", joined)
	}
}

func TestChatLines_ContinuationIndent(t *testing.T) {
	entries := []UIMessage{
		{Role: "assistant", Kind: KindText, Text: "line1\n\nline2"},
	}
	lines := ChatLines(entries, 80)
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 lines, got %d", len(lines))
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "line1") || !strings.Contains(joined, "line2") {
		t.Fatalf("expected multiline content preserved: %q", joined)
	}
}

func TestMarkdown_NarrowWidthFallback(t *testing.T) {
	input := "a **b** c"
	got := Markdown(input, 10, false)
	if got != input {
		t.Fatalf("expected markdown fallback to plain text for narrow width, got: %q", got)
	}
}

func TestMarkdown_TableBareURLDoesNotDuplicate(t *testing.T) {
	input := "| 项目 | 地址 |\n|---|---|\n| A | https://example.com |\n"
	got := Markdown(input, 80, false)
	if strings.Count(got, "https://example.com") != 1 {
		t.Fatalf("expected bare URL once, got: %q", got)
	}
}

func TestMarkdown_TableSelfLinkDoesNotDuplicate(t *testing.T) {
	input := "| 项目 | 地址 |\n|---|---|\n| A | [https://example.com](https://example.com) |\n"
	got := Markdown(input, 80, false)
	if strings.Count(got, "https://example.com") != 1 {
		t.Fatalf("expected self link once, got: %q", got)
	}
}

func TestMarkdown_ExplicitLinkShowsTextAndURL(t *testing.T) {
	input := "[示例](https://example.com)"
	got := Markdown(input, 80, false)
	if !strings.Contains(got, "示例") || !strings.Contains(got, "https://example.com") {
		t.Fatalf("expected link text and URL, got: %q", got)
	}
	if !strings.Contains(got, "示例 (https://example.com)") {
		t.Fatalf("expected terminal link format, got: %q", got)
	}
}

func TestMarkdown_CodeBlockKeepsLinksLiteral(t *testing.T) {
	input := "```md\n[示例](https://example.com)\nhttps://example.com\n```"
	got := Markdown(input, 80, false)
	if strings.Count(got, "https://example.com") != 2 {
		t.Fatalf("expected code block links preserved literally, got: %q", got)
	}
	if !strings.Contains(got, "[示例](https://example.com)") {
		t.Fatalf("expected markdown link preserved in code block, got: %q", got)
	}
}

func TestMarkdown_InlineCodeKeepsLinksLiteral(t *testing.T) {
	input := "`[示例](https://example.com)` and `https://example.com`"
	got := Markdown(input, 80, false)
	if !strings.Contains(got, "[示例](https://example.com)") {
		t.Fatalf("expected inline markdown link preserved, got: %q", got)
	}
	if strings.Contains(got, "示例 (https://example.com)") {
		t.Fatalf("did not expect inline code link to be rewritten, got: %q", got)
	}
}

func TestChatLines_ChineseParagraphAndList_NoCollapsedList(t *testing.T) {
	entries := []UIMessage{
		{
			Role: "assistant",
			Kind: KindText,
			Text: "你好！我是 Claude。\n我可以帮你完成各种任务，比如：\n- 阅读和编辑文件\n- 搜索和查找信息\n- 获取网页内容",
		},
	}
	lines := ChatLines(entries, 90)
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "比如：-") {
		t.Fatalf("list collapsed into paragraph: %q", joined)
	}
	if !strings.Contains(joined, "• 阅读和编辑文件") {
		t.Fatalf("expected first bullet rendered: %q", joined)
	}
	if !strings.Contains(joined, "• 搜索和查找信息") {
		t.Fatalf("expected second bullet rendered: %q", joined)
	}
	if strings.Contains(joined, "\n\n\n") {
		t.Fatalf("unexpected excessive blank lines: %q", joined)
	}
}

func TestChatLines_OrderedListKeepsDotSeparator(t *testing.T) {
	entries := []UIMessage{
		{
			Role: "assistant",
			Kind: KindText,
			Text: "1. `core.py`（720 行）\n2. `server.py` + `routing.py`\n3. 测试覆盖率",
		},
	}
	lines := ChatLines(entries, 90)
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "1core.py") || strings.Contains(joined, "2server.py") {
		t.Fatalf("ordered list marker collapsed into text: %q", joined)
	}
	if !strings.Contains(joined, "1. core.py") || !strings.Contains(joined, "2. server.py") {
		t.Fatalf("expected ordered list marker with dot separator: %q", joined)
	}
}

func TestChatLines_ToolJSON_PreservesMultilineBlock(t *testing.T) {
	entries := []UIMessage{
		{
			Role: "result",
			Kind: KindToolResult,
			Text: "exec_shell: ```json\n{\"ok\":true,\"data\":{\"payload\":{\"command\":\"date\"}}}\n```",
		},
	}
	lines := ChatLines(entries, 100)
	if len(lines) < 2 {
		t.Fatalf("expected multiline render for tool json, got: %v", lines)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "exec_shell:") {
		t.Fatalf("expected tool label: %q", joined)
	}
	if !strings.Contains(joined, "command") {
		t.Fatalf("expected json content: %q", joined)
	}
}
