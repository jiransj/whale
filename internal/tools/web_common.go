package tools

import (
	"regexp"
	"strings"
)

var (
	reTag       = regexp.MustCompile(`(?is)<[^>]+>`)
	reScript    = regexp.MustCompile(`(?is)<script[\s\S]*?</script>`)
	reStyle     = regexp.MustCompile(`(?is)<style[\s\S]*?</style>`)
	reNoScript  = regexp.MustCompile(`(?is)<noscript[\s\S]*?</noscript>`)
	reNav       = regexp.MustCompile(`(?is)<nav[\s\S]*?</nav>`)
	reFooter    = regexp.MustCompile(`(?is)<footer[\s\S]*?</footer>`)
	reAside     = regexp.MustCompile(`(?is)<aside[\s\S]*?</aside>`)
	reSvg       = regexp.MustCompile(`(?is)<svg[\s\S]*?</svg>`)
	reBlockTags = regexp.MustCompile(`(?is)</?(p|div|br|h[1-6]|li|tr|section|article)\b[^>]*>`)
	reTitle     = regexp.MustCompile(`(?is)<title[^>]*>([\s\S]*?)</title>`)
)

func decodeHTMLBasic(s string) string {
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&#39;", `'`)
	return s
}

func normalizeHTMLText(s string) string {
	r := reTag.ReplaceAllString(s, "")
	r = decodeHTMLBasic(r)
	r = strings.Join(strings.Fields(r), " ")
	return strings.TrimSpace(r)
}

func htmlToText(html string) string {
	s := html
	s = reScript.ReplaceAllString(s, "")
	s = reStyle.ReplaceAllString(s, "")
	s = reNoScript.ReplaceAllString(s, "")
	s = reNav.ReplaceAllString(s, "")
	s = reFooter.ReplaceAllString(s, "")
	s = reAside.ReplaceAllString(s, "")
	s = reSvg.ReplaceAllString(s, "")
	s = reBlockTags.ReplaceAllString(s, "\n")
	s = reTag.ReplaceAllString(s, "")
	s = decodeHTMLBasic(s)
	s = regexp.MustCompile(`[ \t]+`).ReplaceAllString(s, " ")
	s = regexp.MustCompile(`\n[ \t]+`).ReplaceAllString(s, "\n")
	s = regexp.MustCompile(`\n{3,}`).ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

func extractHTMLTitle(html string) string {
	m := reTitle.FindStringSubmatch(html)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(strings.Join(strings.Fields(decodeHTMLBasic(m[1])), " "))
}
