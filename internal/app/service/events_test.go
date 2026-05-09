package service

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/usewhale/whale/internal/agent"
	"github.com/usewhale/whale/internal/core"
)

func TestCriticalEventsDeliverAfterDeltaBackpressure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &Service{ctx: ctx, events: make(chan Event, 1)}
	s.events <- Event{Kind: EventInfo, Text: "fill buffer"}

	deltas := newTurnDeltaCoalescers(s)
	for i := 0; i < 200; i++ {
		deltas.add(EventPlanDelta, strings.Repeat("x", 64))
	}

	done := make(chan struct{})
	go func() {
		deltas.flushReliable()
		s.emit(Event{Kind: EventPlanCompleted, Text: "final plan"})
		s.emit(Event{Kind: EventTurnDone, LastResponse: "done"})
		close(done)
	}()

	seenCompleted := false
	seenDone := false
	deadline := time.After(2 * time.Second)
	for !seenCompleted || !seenDone {
		select {
		case ev := <-s.Events():
			if ev.Kind == EventPlanCompleted && ev.Text == "final plan" {
				seenCompleted = true
			}
			if ev.Kind == EventTurnDone {
				seenDone = true
			}
		case <-deadline:
			t.Fatalf("timed out waiting for critical events, completed=%v done=%v", seenCompleted, seenDone)
		}
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("critical sender remained blocked after consumer drained events")
	}
}

func TestTurnDeltaCoalescerPreservesCrossKindOrder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &Service{ctx: ctx, events: make(chan Event, 10)}

	deltas := newTurnDeltaCoalescers(s)
	deltas.add(EventReasoningDelta, "think-a ")
	deltas.add(EventAssistantDelta, "answer ")
	deltas.add(EventReasoningDelta, "think-b")
	deltas.flushReliable()

	want := []Event{
		{Kind: EventReasoningDelta, Text: "think-a "},
		{Kind: EventAssistantDelta, Text: "answer "},
		{Kind: EventReasoningDelta, Text: "think-b"},
	}
	for i, w := range want {
		select {
		case got := <-s.Events():
			if got.Kind != w.Kind || got.Text != w.Text {
				t.Fatalf("event %d mismatch: got kind=%s text=%q, want kind=%s text=%q", i, got.Kind, got.Text, w.Kind, w.Text)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for event %d", i)
		}
	}
}

func TestTurnDeltaCoalescerCoalescesOnlyAdjacentSameKind(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &Service{ctx: ctx, events: make(chan Event, 10)}

	deltas := newTurnDeltaCoalescers(s)
	deltas.add(EventReasoningDelta, "a")
	deltas.add(EventReasoningDelta, "b")
	deltas.add(EventAssistantDelta, "c")
	deltas.add(EventAssistantDelta, "d")
	deltas.flushReliable()

	want := []Event{
		{Kind: EventReasoningDelta, Text: "ab"},
		{Kind: EventAssistantDelta, Text: "cd"},
	}
	for i, w := range want {
		select {
		case got := <-s.Events():
			if got.Kind != w.Kind || got.Text != w.Text {
				t.Fatalf("event %d mismatch: got kind=%s text=%q, want kind=%s text=%q", i, got.Kind, got.Text, w.Kind, w.Text)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for event %d", i)
		}
	}
}

func TestEmitReliableUnblocksOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{ctx: ctx, events: make(chan Event)}
	done := make(chan struct{})
	go func() {
		s.emit(Event{Kind: EventTurnDone})
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("reliable emit did not unblock after context cancellation")
	}
}

func TestShouldSuppressCancelledTurnErrorOnlyForCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	wrapped := fmt.Errorf("request failed: %w", context.Canceled)
	if shouldSuppressCancelledTurnError(ctx, wrapped) {
		t.Fatal("did not expect suppression before the turn context is cancelled")
	}
	cancel()
	if !shouldSuppressCancelledTurnError(ctx, wrapped) {
		t.Fatal("expected user-cancelled context error to be suppressed")
	}
	if shouldSuppressCancelledTurnError(ctx, fmt.Errorf("request failed: boom")) {
		t.Fatal("did not expect unrelated errors to be suppressed")
	}
}

func TestSummarizeToolCall_GrepShowsPatternPathAndInclude(t *testing.T) {
	got := summarizeToolCall(core.ToolCall{
		Name:  "grep",
		Input: `{"pattern":"assistant_delta","path":"internal/tui","include":"*.go"}`,
	})
	if got != "grep: assistant_delta in internal/tui (*.go)" {
		t.Fatalf("unexpected grep summary: %q", got)
	}
}

func TestSummarizeToolCall_SearchFilesShowsPatternAndPath(t *testing.T) {
	got := summarizeToolCall(core.ToolCall{
		Name:  "search_files",
		Input: `{"pattern":"markdown.go","path":"internal/tui"}`,
	})
	if got != "search_files: markdown.go in internal/tui" {
		t.Fatalf("unexpected search_files summary: %q", got)
	}
}

func TestSummarizeToolCall_WebSearchUsesNestedSearchQuery(t *testing.T) {
	got := summarizeToolCall(core.ToolCall{
		Name:  "web_search",
		Input: `{"search_query":[{"q":"F1 pit strategy tools"}]}`,
	})
	if got != "web_search: F1 pit strategy tools" {
		t.Fatalf("unexpected web_search summary: %q", got)
	}
}

func TestSummarizeToolCall_TaskTools(t *testing.T) {
	got := summarizeToolCall(core.ToolCall{
		Name:  "parallel_reason",
		Input: `{"prompts":["a","b","c"]}`,
	})
	if got != "parallel_reason: 3 prompt(s)" {
		t.Fatalf("unexpected parallel_reason summary: %q", got)
	}
	got = summarizeToolCall(core.ToolCall{
		Name:  "spawn_subagent",
		Input: `{"role":"review","task":"review internal/tasks\nignore details"}`,
	})
	if got != "spawn_subagent: review · review internal/tasks" {
		t.Fatalf("unexpected spawn_subagent summary: %q", got)
	}
}

func TestSummarizeTaskActivity(t *testing.T) {
	got := summarizeTaskActivity(EventTaskStarted, &agent.TaskActivityInfo{ToolName: "parallel_reason", Status: "started", Count: 4})
	if got != "parallel_reason started · 4 prompt(s)" {
		t.Fatalf("unexpected parallel activity: %q", got)
	}
	got = summarizeTaskActivity(EventTaskCompleted, &agent.TaskActivityInfo{ToolName: "spawn_subagent", Status: "completed", Role: "review", DurationMS: 1200})
	if got != "spawn_subagent completed · review · 1200ms" {
		t.Fatalf("unexpected subagent activity: %q", got)
	}
}
