package core

import "strings"

const (
	ProposedPlanOpenTag  = "<proposed_plan>"
	ProposedPlanCloseTag = "</proposed_plan>"
)

type ProposedPlanSegmentKind string

const (
	ProposedPlanSegmentNormal ProposedPlanSegmentKind = "normal"
	ProposedPlanSegmentStart  ProposedPlanSegmentKind = "start"
	ProposedPlanSegmentDelta  ProposedPlanSegmentKind = "delta"
	ProposedPlanSegmentEnd    ProposedPlanSegmentKind = "end"
)

type ProposedPlanSegment struct {
	Kind ProposedPlanSegmentKind
	Text string
}

type ProposedPlanParser struct {
	inPlan bool
	buf    string
}

func (p *ProposedPlanParser) Parse(delta string) []ProposedPlanSegment {
	if delta == "" {
		return nil
	}
	p.buf += delta
	return p.drain(false)
}

func (p *ProposedPlanParser) Finish() []ProposedPlanSegment {
	if p.buf == "" && p.inPlan {
		p.inPlan = false
		return []ProposedPlanSegment{{Kind: ProposedPlanSegmentEnd}}
	}
	return p.drain(true)
}

func (p *ProposedPlanParser) drain(final bool) []ProposedPlanSegment {
	var out []ProposedPlanSegment
	for p.buf != "" {
		if !p.inPlan {
			idx := strings.Index(p.buf, ProposedPlanOpenTag)
			if idx >= 0 {
				if idx > 0 {
					out = append(out, ProposedPlanSegment{Kind: ProposedPlanSegmentNormal, Text: p.buf[:idx]})
				}
				p.buf = p.buf[idx+len(ProposedPlanOpenTag):]
				p.inPlan = true
				out = append(out, ProposedPlanSegment{Kind: ProposedPlanSegmentStart})
				continue
			}
			if final {
				out = append(out, ProposedPlanSegment{Kind: ProposedPlanSegmentNormal, Text: p.buf})
				p.buf = ""
				break
			}
			keep := tagPrefixSuffixLen(p.buf, ProposedPlanOpenTag)
			if keep == len(p.buf) {
				break
			}
			text := p.buf[:len(p.buf)-keep]
			if text != "" {
				out = append(out, ProposedPlanSegment{Kind: ProposedPlanSegmentNormal, Text: text})
			}
			p.buf = p.buf[len(p.buf)-keep:]
			break
		}

		idx := strings.Index(p.buf, ProposedPlanCloseTag)
		if idx >= 0 {
			if idx > 0 {
				out = append(out, ProposedPlanSegment{Kind: ProposedPlanSegmentDelta, Text: p.buf[:idx]})
			}
			p.buf = p.buf[idx+len(ProposedPlanCloseTag):]
			p.inPlan = false
			out = append(out, ProposedPlanSegment{Kind: ProposedPlanSegmentEnd})
			continue
		}
		if final {
			out = append(out, ProposedPlanSegment{Kind: ProposedPlanSegmentDelta, Text: p.buf})
			p.buf = ""
			p.inPlan = false
			out = append(out, ProposedPlanSegment{Kind: ProposedPlanSegmentEnd})
			break
		}
		keep := tagPrefixSuffixLen(p.buf, ProposedPlanCloseTag)
		if keep == len(p.buf) {
			break
		}
		text := p.buf[:len(p.buf)-keep]
		if text != "" {
			out = append(out, ProposedPlanSegment{Kind: ProposedPlanSegmentDelta, Text: text})
		}
		p.buf = p.buf[len(p.buf)-keep:]
		break
	}
	return out
}

func StripProposedPlanBlocks(text string) string {
	var p ProposedPlanParser
	var out strings.Builder
	for _, seg := range append(p.Parse(text), p.Finish()...) {
		if seg.Kind == ProposedPlanSegmentNormal {
			out.WriteString(seg.Text)
		}
	}
	return out.String()
}

func ExtractProposedPlanText(text string) (string, bool) {
	var p ProposedPlanParser
	var out strings.Builder
	seen := false
	for _, seg := range append(p.Parse(text), p.Finish()...) {
		switch seg.Kind {
		case ProposedPlanSegmentStart:
			seen = true
			out.Reset()
		case ProposedPlanSegmentDelta:
			out.WriteString(seg.Text)
		}
	}
	return out.String(), seen
}

func tagPrefixSuffixLen(s, tag string) int {
	maxLen := len(tag) - 1
	if maxLen > len(s) {
		maxLen = len(s)
	}
	for n := maxLen; n > 0; n-- {
		if strings.HasSuffix(s, tag[:n]) {
			return n
		}
	}
	return 0
}
