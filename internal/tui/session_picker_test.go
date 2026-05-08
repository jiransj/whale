package tui

import "testing"

func TestDisplaySessionChoiceRowPreservesOrdinal(t *testing.T) {
	got := displaySessionChoiceRow("   2) 1m ago    main                     hello")
	if got != "   2) 1m ago    main                     hello" {
		t.Fatalf("expected ordinal to be preserved, got %q", got)
	}
}

func TestDisplaySessionChoiceRowHidesCurrentSessionMarker(t *testing.T) {
	got := displaySessionChoiceRow("*  1) 4s ago    -                        who are you")
	if got != "   1) 4s ago    -                        who are you" {
		t.Fatalf("expected marker to be replaced with spacing, got %q", got)
	}
}

func TestSessionChoiceNumberAtStillParsesCurrentMarker(t *testing.T) {
	rows := []string{
		"recent sessions:",
		"   #   Updated   Branch                    Conversation",
		"*  1) 4s ago    -                        who are you",
	}
	if got := sessionChoiceNumberAt(rows, 2); got != 1 {
		t.Fatalf("expected session number 1, got %d", got)
	}
}
