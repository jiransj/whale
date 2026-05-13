package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/usewhale/whale/internal/agent"
	"github.com/usewhale/whale/internal/app"
	"github.com/usewhale/whale/internal/core"
	"github.com/usewhale/whale/internal/session"
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

func TestResumeMenuStartupOpensSessionPickerBeforeHydration(t *testing.T) {
	dir := t.TempDir()
	writeSessionFile(t, dir, "sess-1", "hello resume")
	cfg := app.DefaultConfig()
	cfg.DataDir = dir

	svc, err := New(t.Context(), cfg, app.StartOptions{ResumeMenu: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer svc.Close()

	for {
		ev := nextServiceEvent(t, svc)
		switch ev.Kind {
		case EventSessionHydrated:
			t.Fatal("session hydrated before resume picker was shown")
		case EventSessionsListed:
			if joined := strings.Join(ev.Choices, "\n"); !strings.Contains(joined, "hello resume") {
				t.Fatalf("expected session choice to include conversation, got:\n%s", joined)
			}
			svc.Dispatch(Intent{Kind: IntentSelectSession, SessionInput: "1"})
			assertSessionSelectedAndHydrated(t, svc)
			return
		}
	}
}

func TestResumeMenuStartupWithNoSessionsHydratesFallbackSession(t *testing.T) {
	cfg := app.DefaultConfig()
	cfg.DataDir = t.TempDir()

	svc, err := New(t.Context(), cfg, app.StartOptions{ResumeMenu: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer svc.Close()

	sawNoSaved := false
	for {
		ev := nextServiceEvent(t, svc)
		switch ev.Kind {
		case EventSessionsListed:
			t.Fatal("did not expect an empty session picker")
		case EventInfo:
			if ev.Text == "no saved sessions" {
				sawNoSaved = true
			}
		case EventSessionHydrated:
			if !sawNoSaved {
				t.Fatal("expected no saved sessions notice before hydration")
			}
			return
		}
	}
}

func TestResumeMenuCrossWorkspaceSelectionDoesNotHydrate(t *testing.T) {
	dir := t.TempDir()
	other := t.TempDir()
	writeSessionFile(t, dir, "sess-1", "hello from elsewhere")
	if err := session.SaveSessionMeta(filepath.Join(dir, "sessions"), "sess-1", session.SessionMeta{Workspace: other}); err != nil {
		t.Fatalf("save session meta: %v", err)
	}
	cfg := app.DefaultConfig()
	cfg.DataDir = dir

	svc, err := New(t.Context(), cfg, app.StartOptions{ResumeMenu: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer svc.Close()

	for {
		ev := nextServiceEvent(t, svc)
		switch ev.Kind {
		case EventSessionHydrated:
			t.Fatal("session hydrated before cross-workspace selection")
		case EventSessionsListed:
			svc.Dispatch(Intent{Kind: IntentSelectSession, SessionInput: "1"})
		case EventInfo:
			if strings.Contains(ev.Text, "This conversation is from a different directory.") {
				for {
					select {
					case queued := <-svc.Events():
						if queued.Kind == EventSessionHydrated {
							t.Fatalf("did not expect hydration after cross-workspace message: %+v", queued)
						}
					default:
						return
					}
				}
			}
		}
	}
}

func TestSkillsCommandOpensMenuAndToggleUpdatesSuggestions(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("DEEPSEEK_API_KEY", "sk-test")
	work := t.TempDir()
	t.Chdir(work)
	writeServiceSkill(t, filepath.Join(work, ".whale", "skills", "test-skill"), "test-skill", "Workspace skill.")

	cfg := app.DefaultConfig()
	cfg.DataDir = t.TempDir()
	svc, err := New(t.Context(), cfg, app.StartOptions{NewSession: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer svc.Close()
	waitForServiceEvent(t, svc, EventSessionHydrated)

	svc.Dispatch(Intent{Kind: IntentSubmit, Input: "/skills"})
	evMenu := waitForServiceEvent(t, svc, EventSkillsMenu)
	if evMenu.Kind != EventSkillsMenu {
		t.Fatalf("expected skills menu event, got %+v", evMenu)
	}
	svc.Dispatch(Intent{Kind: IntentRequestSkillsManage})
	ev := waitForServiceEvent(t, svc, EventSkillsManager)
	if len(ev.Skills) != 1 || ev.Skills[0].Name != "test-skill" {
		t.Fatalf("unexpected skills manager event: %+v", ev.Skills)
	}
	if len(svc.SkillSuggestions()) != 1 {
		t.Fatalf("expected skill suggestion before disabling, got %+v", svc.SkillSuggestions())
	}

	svc.Dispatch(Intent{Kind: IntentSetSkillEnabled, SkillName: "test-skill", SkillEnabled: false})
	ev = waitForServiceEvent(t, svc, EventSkillsManager)
	if len(ev.Skills) != 1 || ev.Skills[0].Name != "test-skill" || ev.Skills[0].Status != "disabled" {
		t.Fatalf("expected disabled skill manager event, got %+v", ev.Skills)
	}
	if got := svc.SkillSuggestions(); len(got) != 0 {
		t.Fatalf("expected disabled skill to disappear from suggestions, got %+v", got)
	}

	svc.Dispatch(Intent{Kind: IntentSetSkillEnabled, SkillName: "test-skill", SkillEnabled: true})
	ev = waitForServiceEvent(t, svc, EventSkillsManager)
	if len(ev.Skills) != 1 || ev.Skills[0].Name != "test-skill" || ev.Skills[0].Status != "ready" {
		t.Fatalf("expected ready skill manager event, got %+v", ev.Skills)
	}
	if got := svc.SkillSuggestions(); len(got) != 1 || got[0].Name != "test-skill" {
		t.Fatalf("expected enabled skill suggestion, got %+v", got)
	}
}

func TestSkillMentionEmitsLoadedEventNotInfo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("DEEPSEEK_API_KEY", "")
	work := t.TempDir()
	t.Chdir(work)
	writeServiceSkill(t, filepath.Join(work, ".whale", "skills", "test-skill"), "test-skill", "Workspace skill.")

	cfg := app.DefaultConfig()
	cfg.DataDir = t.TempDir()
	svc, err := New(t.Context(), cfg, app.StartOptions{NewSession: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer svc.Close()
	waitForServiceEvent(t, svc, EventSessionHydrated)

	svc.Dispatch(Intent{Kind: IntentSubmit, Input: "$test-skill review this"})
	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev := <-svc.Events():
			if ev.Kind == EventInfo && strings.Contains(ev.Text, "loaded skill: test-skill") {
				t.Fatalf("skill load should not be emitted as info: %+v", ev)
			}
			if ev.Kind == EventSkillLoaded {
				if ev.Text != "loaded skill: test-skill" {
					t.Fatalf("unexpected skill loaded text: %q", ev.Text)
				}
				waitForServiceEvent(t, svc, EventTurnDone)
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for skill loaded event")
		}
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

func nextServiceEvent(t *testing.T, s *Service) Event {
	t.Helper()
	select {
	case ev := <-s.Events():
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for service event")
		return Event{}
	}
}

func waitForServiceEvent(t *testing.T, s *Service, kind EventKind) Event {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev := <-s.Events():
			if ev.Kind == kind {
				return ev
			}
		case <-deadline:
			t.Fatalf("timed out waiting for service event %s", kind)
			return Event{}
		}
	}
}

func assertSessionSelectedAndHydrated(t *testing.T, s *Service) {
	t.Helper()
	sawInfo := false
	for {
		ev := nextServiceEvent(t, s)
		switch ev.Kind {
		case EventInfo:
			if strings.Contains(ev.Text, "resumed session: sess-1") {
				sawInfo = true
			}
		case EventSessionHydrated:
			if !sawInfo {
				t.Fatal("expected resumed session info before hydration")
			}
			if ev.SessionID != "sess-1" {
				t.Fatalf("hydrated session = %s, want sess-1", ev.SessionID)
			}
			return
		}
	}
}

func writeSessionFile(t *testing.T, dataDir, id, text string) {
	t.Helper()
	sessionsDir := filepath.Join(dataDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}
	line := fmt.Sprintf("{\"role\":\"user\",\"text\":%q}\n", text)
	if err := os.WriteFile(filepath.Join(sessionsDir, id+".jsonl"), []byte(line), 0o600); err != nil {
		t.Fatalf("write session: %v", err)
	}
}

func writeServiceSkill(t *testing.T, dir, name, desc string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	content := fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n# %s\n", name, desc, name)
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
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
