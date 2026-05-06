package service

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
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
