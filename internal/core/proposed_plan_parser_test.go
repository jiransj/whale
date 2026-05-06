package core

import "testing"

func TestProposedPlanParserStreamsSegments(t *testing.T) {
	var p ProposedPlanParser
	var got []ProposedPlanSegment
	for _, chunk := range []string{"Intro\n<prop", "osed_plan>\n- one\n", "</proposed_plan>\nOutro"} {
		got = append(got, p.Parse(chunk)...)
	}
	got = append(got, p.Finish()...)

	want := []ProposedPlanSegment{
		{Kind: ProposedPlanSegmentNormal, Text: "Intro\n"},
		{Kind: ProposedPlanSegmentStart},
		{Kind: ProposedPlanSegmentDelta, Text: "\n- one\n"},
		{Kind: ProposedPlanSegmentEnd},
		{Kind: ProposedPlanSegmentNormal, Text: "\nOutro"},
	}
	if len(got) != len(want) {
		t.Fatalf("len got=%d want=%d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("segment %d got=%+v want=%+v", i, got[i], want[i])
		}
	}
}

func TestExtractAndStripProposedPlan(t *testing.T) {
	text := "before\n<proposed_plan>\n# Plan\n- A\n</proposed_plan>\nafter"
	plan, ok := ExtractProposedPlanText(text)
	if !ok || plan != "\n# Plan\n- A\n" {
		t.Fatalf("unexpected plan ok=%v plan=%q", ok, plan)
	}
	if got := StripProposedPlanBlocks(text); got != "before\n\nafter" {
		t.Fatalf("unexpected stripped text: %q", got)
	}
}

func TestProposedPlanParserClosesMissingEndTag(t *testing.T) {
	var p ProposedPlanParser
	got := p.Parse("<proposed_plan>\n- A")
	got = append(got, p.Finish()...)
	if len(got) != 3 || got[0].Kind != ProposedPlanSegmentStart || got[1].Kind != ProposedPlanSegmentDelta || got[2].Kind != ProposedPlanSegmentEnd {
		t.Fatalf("unexpected finish segments: %+v", got)
	}
}
