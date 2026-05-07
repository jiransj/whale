package tui

import (
	"testing"

	tuirender "github.com/usewhale/whale/internal/tui/render"
)

func TestLiveStreamPushCommitsCompleteLinesAndKeepsTail(t *testing.T) {
	stream := newLiveStream("assistant", tuirender.KindText)
	commit := stream.push("one\npartial")
	if commit == nil || commit.message == nil {
		t.Fatal("expected completed line commit")
	}
	if commit.message.Text != "one" {
		t.Fatalf("unexpected committed text: %q", commit.message.Text)
	}
	if tail := stream.tailMessage(); tail == nil || tail.Text != "partial" {
		t.Fatalf("expected partial tail, got %+v", tail)
	}

	commit = stream.push(" two\nlast")
	if commit == nil || commit.message == nil {
		t.Fatal("expected second completed line commit")
	}
	if commit.message.Text != "partial two" {
		t.Fatalf("unexpected second committed text: %q", commit.message.Text)
	}
	if tail := stream.tailMessage(); tail == nil || tail.Text != "last" {
		t.Fatalf("expected final tail, got %+v", tail)
	}
}

func TestLiveStreamNormalizesCRLF(t *testing.T) {
	stream := newLiveStream("assistant", tuirender.KindText)
	commit := stream.push("one\r\ntwo")
	if commit == nil || commit.message == nil {
		t.Fatal("expected completed line commit")
	}
	if commit.message.Text != "one" {
		t.Fatalf("unexpected committed text: %q", commit.message.Text)
	}
	if tail := stream.tailMessage(); tail == nil || tail.Text != "two" {
		t.Fatalf("expected normalized tail, got %+v", tail)
	}
}

func TestLiveStreamFinalizeAfterCommittedAddsGapOnly(t *testing.T) {
	stream := newLiveStream("assistant", tuirender.KindText)
	if commit := stream.push("one\n"); commit == nil || commit.message == nil {
		t.Fatal("expected initial committed line")
	}
	commit := stream.finalize()
	if commit == nil {
		t.Fatal("expected finalize marker")
	}
	if commit.message != nil || !commit.gapAfter {
		t.Fatalf("expected gap-only finalize, got %+v", commit)
	}
	if tail := stream.tailMessage(); tail != nil {
		t.Fatalf("expected cleared tail, got %+v", tail)
	}
}
