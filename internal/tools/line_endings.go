package tools

import "strings"

type lineEndingStyle string

const (
	lineEndingLF   lineEndingStyle = "\n"
	lineEndingCRLF lineEndingStyle = "\r\n"
	lineEndingCR   lineEndingStyle = "\r"
)

func normalizeLineEndings(s string) (string, lineEndingStyle) {
	switch {
	case strings.Contains(s, "\r\n"):
		s = strings.ReplaceAll(s, "\r\n", "\n")
		return strings.ReplaceAll(s, "\r", "\n"), lineEndingCRLF
	case strings.Contains(s, "\r"):
		return strings.ReplaceAll(s, "\r", "\n"), lineEndingCR
	default:
		return s, lineEndingLF
	}
}

func normalizeLineEndingText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\r", "\n")
}

func restoreLineEndings(s string, style lineEndingStyle) string {
	switch style {
	case lineEndingCRLF:
		return strings.ReplaceAll(s, "\n", "\r\n")
	case lineEndingCR:
		return strings.ReplaceAll(s, "\n", "\r")
	default:
		return s
	}
}
