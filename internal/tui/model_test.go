package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/usewhale/whale/internal/app"
	"github.com/usewhale/whale/internal/app/service"
	"github.com/usewhale/whale/internal/core"
	tuirender "github.com/usewhale/whale/internal/tui/render"
)

func newModelWithDispatchSpy() (model, *[]service.Intent) {
	m := newModel(nil, "", "", "")
	intents := []service.Intent{}
	m.dispatch = func(in service.Intent) {
		intents = append(intents, in)
	}
	return m, &intents
}

func selectSlashCommand(t *testing.T, m *model, want string) {
	t.Helper()
	for i, cmd := range m.slash.matches {
		if cmd == want {
			m.slash.selected = i
			return
		}
	}
	t.Fatalf("slash command %q not found in matches %+v", want, m.slash.matches)
}

func TestIsEnvironmentInventoryBlock_PositiveChinese(t *testing.T) {
	text := "- 系统： macOS\n- 版本： 26.0\n- 构建号： 25A354"
	if !isEnvironmentInventoryBlock(text) {
		t.Fatalf("expected environment inventory block to be detected")
	}
}

func TestIsEnvironmentInventoryBlock_NegativeNormalAssistantText(t *testing.T) {
	text := "I checked the version mismatch in package constraints and suggest bumping one dependency."
	if isEnvironmentInventoryBlock(text) {
		t.Fatalf("did not expect normal assistant text to be detected as environment inventory block")
	}
}

func TestHydrateSessionMessages_SuppressesEnvironmentInventoryAssistantBlock(t *testing.T) {
	m := &model{assembler: tuirender.NewAssembler()}
	msgs := []core.Message{
		{
			Role: core.RoleAssistant,
			Text: "- 系统： macOS\n- 版本： 26.0\n- 构建号： 25A354",
		},
	}
	m.hydrateSessionMessages(msgs)
	if got := len(m.assembler.Snapshot()); got != 0 {
		t.Fatalf("expected no chat entries for environment inventory block, got %d", got)
	}
}

func TestHydrateSessionMessages_KeptForNormalAssistantText(t *testing.T) {
	m := &model{assembler: tuirender.NewAssembler()}
	msgs := []core.Message{
		{
			Role: core.RoleAssistant,
			Text: "Implemented the layout update and kept footer semantics unchanged.",
		},
	}
	m.hydrateSessionMessages(msgs)
	snap := m.assembler.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected one assistant entry, got %d", len(snap))
	}
	if snap[0].Role != "assistant" {
		t.Fatalf("expected role assistant, got %q", snap[0].Role)
	}
}

func TestHydrateSessionMessages_RendersReasoningAsThinkingOnly(t *testing.T) {
	m := &model{assembler: tuirender.NewAssembler()}
	msgs := []core.Message{
		{
			Role:      core.RoleAssistant,
			Reasoning: "I should answer the age question.",
		},
	}
	m.hydrateSessionMessages(msgs)
	snap := m.assembler.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected only thinking entry, got %+v", snap)
	}
	if snap[0].Role != "think" || snap[0].Kind != tuirender.KindThinking {
		t.Fatalf("expected first entry to be thinking, got %+v", snap[0])
	}
}

func TestHydrateSessionMessages_RendersReasoningAndAssistantSeparately(t *testing.T) {
	m := &model{assembler: tuirender.NewAssembler()}
	msgs := []core.Message{
		{
			Role:      core.RoleAssistant,
			Reasoning: "I should answer succinctly.",
			Text:      "I do not have an age.",
		},
	}
	m.hydrateSessionMessages(msgs)
	snap := m.assembler.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("expected thinking plus assistant entries, got %+v", snap)
	}
	if snap[0].Kind != tuirender.KindThinking || snap[0].Role != "think" {
		t.Fatalf("expected thinking entry first, got %+v", snap[0])
	}
	if snap[1].Role != "assistant" || snap[1].Kind != tuirender.KindText {
		t.Fatalf("expected assistant text second, got %+v", snap[1])
	}
}

func TestHydrateSessionMessages_SuppressesHiddenUserText(t *testing.T) {
	m := &model{assembler: tuirender.NewAssembler()}
	msgs := []core.Message{
		{
			Role:   core.RoleUser,
			Text:   "Generate a file named AGENTS.md",
			Hidden: true,
		},
	}
	m.hydrateSessionMessages(msgs)
	if got := len(m.assembler.Snapshot()); got != 0 {
		t.Fatalf("expected no chat entries for hidden user text, got %d", got)
	}
}

func TestSlashCommandsShowPermissionsAndHideApproval(t *testing.T) {
	cmds := parseSlashCommands(app.CommandsHelp)
	if !containsString(cmds, "/permissions") {
		t.Fatalf("expected /permissions in slash commands: %+v", cmds)
	}
	if !containsString(cmds, "/plan") {
		t.Fatalf("expected /plan in slash commands: %+v", cmds)
	}
	if !containsString(cmds, "/ask") {
		t.Fatalf("expected /ask in slash commands: %+v", cmds)
	}
	if containsString(cmds, "/approval") {
		t.Fatalf("expected /approval to stay hidden from slash commands: %+v", cmds)
	}
	if containsString(cmds, "/thinking") {
		t.Fatalf("expected /thinking to stay hidden from slash commands: %+v", cmds)
	}
	if containsString(cmds, "/budget") {
		t.Fatalf("expected /budget to stay hidden from slash commands: %+v", cmds)
	}
	if containsString(cmds, "/step") {
		t.Fatalf("expected /step to stay hidden from slash commands: %+v", cmds)
	}
}

func TestPickerEventsClearBusyState(t *testing.T) {
	tests := []struct {
		name string
		ev   service.Event
		mode mode
	}{
		{
			name: "model picker",
			ev: service.Event{
				Kind:            service.EventModelPicker,
				ModelChoices:    []string{"deepseek-v4-pro"},
				EffortChoices:   []string{"normal"},
				ThinkingChoices: []string{"on", "off"},
				CurrentModel:    "deepseek-v4-pro",
				CurrentEffort:   "normal",
				CurrentThinking: "on",
			},
			mode: modeModelPicker,
		},
		{
			name: "permissions picker",
			ev: service.Event{
				Kind:            service.EventPermissionsPicker,
				ApprovalChoices: []string{"Ask first", "Auto approve"},
				CurrentApproval: "Ask first",
			},
			mode: modePermissionsPicker,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := model{assembler: tuirender.NewAssembler(), mode: modeChat, busy: true, stopping: true}
			m.busySince = time.Now().Add(-5 * time.Minute)
			next, _ := m.Update(svcMsg(tt.ev))
			m = next.(model)
			if m.busy || m.stopping || !m.busySince.IsZero() {
				t.Fatalf("expected picker event to clear busy state, busy=%v stopping=%v busySince=%v", m.busy, m.stopping, m.busySince)
			}
			if m.mode != tt.mode {
				t.Fatalf("expected mode %v, got %v", tt.mode, m.mode)
			}
		})
	}
}

func TestTurnDoneReasoningOnlyCommitsFallback(t *testing.T) {
	m := model{assembler: tuirender.NewAssembler(), mode: modeChat, width: 80, height: 24, busy: true}
	next, _ := m.Update(svcMsg(service.Event{Kind: service.EventReasoningDelta, Text: "I should answer."}))
	m = next.(model)
	next, cmd := m.Update(svcMsg(service.Event{Kind: service.EventTurnDone}))
	m = next.(model)
	if cmd == nil {
		t.Fatal("expected wait-event command")
	}
	if got := strings.Join(tuirender.ChatLines(m.transcript, 80), "\n"); !strings.Contains(got, "No final answer was produced") {
		t.Fatalf("expected fallback notice in transcript:\n%s", got)
	}
	if m.sawReasoningThisTurn || m.sawAssistantThisTurn {
		t.Fatal("expected turn tracking flags to reset")
	}
}

func TestMarkNoFinalAnswerIfNeeded(t *testing.T) {
	m := model{
		assembler:            tuirender.NewAssembler(),
		sawReasoningThisTurn: true,
	}
	if !m.markNoFinalAnswerIfNeeded() {
		t.Fatal("expected no-final-answer status to be marked")
	}
	if m.status != "" {
		t.Fatalf("unexpected status: %q", m.status)
	}
	snap := m.assembler.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected one notice entry, got %+v", snap)
	}
	if snap[0].Kind != tuirender.KindNotice || snap[0].Role != "notice" {
		t.Fatalf("expected notice entry, got %+v", snap[0])
	}
	if !strings.Contains(snap[0].Text, "No final answer was produced") {
		t.Fatalf("expected generic missing-answer notice, got %q", snap[0].Text)
	}
	if len(m.logs) != 1 || m.logs[0].Kind != "no_final_answer" {
		t.Fatalf("expected diagnostic log entry, got %+v", m.logs)
	}
}

func TestMarkNoFinalAnswerIfNeededAddsPlanNotice(t *testing.T) {
	m := model{
		assembler:            tuirender.NewAssembler(),
		chatMode:             "plan",
		sawReasoningThisTurn: true,
	}
	if !m.markNoFinalAnswerIfNeeded() {
		t.Fatal("expected no-final-answer status to be marked")
	}
	snap := m.assembler.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected one notice entry, got %+v", snap)
	}
	if snap[0].Kind != tuirender.KindNotice || snap[0].Role != "notice" {
		t.Fatalf("expected notice entry, got %+v", snap[0])
	}
	if !strings.Contains(snap[0].Text, "No plan was produced") {
		t.Fatalf("expected missing-plan notice, got %q", snap[0].Text)
	}
}

func TestMarkNoFinalAnswerIfNeededSkippedWithTerminalToolOutcome(t *testing.T) {
	m := model{
		assembler:                      tuirender.NewAssembler(),
		sawReasoningThisTurn:           true,
		sawTerminalToolOutcomeThisTurn: true,
	}
	if m.markNoFinalAnswerIfNeeded() {
		t.Fatal("did not expect no-final-answer status after terminal tool outcome")
	}
	if got := len(m.assembler.Snapshot()); got != 0 {
		t.Fatalf("expected no chat entries, got %d", got)
	}
}

func TestMarkNoFinalAnswerIfNeededSkippedWithAssistant(t *testing.T) {
	m := model{
		assembler:            tuirender.NewAssembler(),
		sawReasoningThisTurn: true,
		sawAssistantThisTurn: true,
	}
	if m.markNoFinalAnswerIfNeeded() {
		t.Fatal("did not expect no-final-answer status with assistant answer")
	}
	if got := len(m.assembler.Snapshot()); got != 0 {
		t.Fatalf("expected no chat entries, got %d", got)
	}
}

func TestVisibleSubmittedTextForPlanPrompt(t *testing.T) {
	if got := visibleSubmittedText("/ask inspect the parser"); got != "inspect the parser" {
		t.Fatalf("unexpected visible text for ask prompt: %q", got)
	}
	if got := visibleSubmittedText("/plan inspect the parser"); got != "inspect the parser" {
		t.Fatalf("unexpected visible text: %q", got)
	}
	if got := visibleSubmittedText("/plan"); got != "/plan" {
		t.Fatalf("unexpected visible text for bare plan: %q", got)
	}
	if got := visibleSubmittedText("/plan off"); got != "/plan off" {
		t.Fatalf("unexpected visible text for unsupported plan off: %q", got)
	}
}

func TestSlashSuggestionsHiddenForMultilineInput(t *testing.T) {
	m := newModel(nil, "", "", "")
	m.input.SetValue("/sta\nmore")
	m.updateSlashMatches()
	if len(m.slash.matches) != 0 {
		t.Fatalf("expected slash suggestions hidden for multiline input, got %+v", m.slash.matches)
	}
}

func TestSlashSuggestionsShownForSingleLineSlash(t *testing.T) {
	m := newModel(nil, "", "", "")
	m.input.SetValue("/")
	m.updateSlashMatches()
	if len(m.slash.matches) == 0 {
		t.Fatal("expected slash suggestions for bare slash")
	}
}

func TestSlashSuggestionsHiddenForAbsolutePathInput(t *testing.T) {
	m := newModel(nil, "", "", "")
	m.input.SetValue("/Users/goranka/Engineer/ai/dsk 里有好几个go项目的，你看看它们怎么做的")
	m.updateSlashMatches()
	if len(m.slash.matches) != 0 {
		t.Fatalf("expected slash suggestions hidden for absolute path prompt, got %+v", m.slash.matches)
	}
}

func TestSlashSuggestionEnterAutoRunsSingleCommandAndClearsSuggestions(t *testing.T) {
	m, intents := newModelWithDispatchSpy()
	m.input.SetValue("/co")
	m.updateSlashMatches()
	if len(m.slash.matches) == 0 {
		t.Fatal("expected slash matches")
	}
	selectSlashCommand(t, &m, "/compact")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if len(*intents) != 1 {
		t.Fatalf("expected one dispatched intent, got %+v", *intents)
	}
	if (*intents)[0].Kind != service.IntentSubmit || (*intents)[0].Input != "/compact" {
		t.Fatalf("unexpected dispatched intent: %+v", (*intents)[0])
	}
	if got := m.input.Value(); got != "" {
		t.Fatalf("expected input cleared after autorun slash enter, got %q", got)
	}
	if len(m.slash.matches) != 0 || m.slash.selected != 0 {
		t.Fatalf("expected slash state cleared, got matches=%v selected=%d", m.slash.matches, m.slash.selected)
	}
	if !m.busy || m.status != "running" {
		t.Fatalf("expected running state after autorun slash enter, busy=%v status=%q", m.busy, m.status)
	}
}

func TestSlashSuggestionTabFillsInputWithoutDispatch(t *testing.T) {
	m, intents := newModelWithDispatchSpy()
	m.input.SetValue("/co")
	m.updateSlashMatches()
	selectSlashCommand(t, &m, "/compact")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(model)
	if len(*intents) != 0 {
		t.Fatalf("expected no dispatch on tab, got %+v", *intents)
	}
	if got := m.input.Value(); got != "/compact" {
		t.Fatalf("expected tab to fill exact command, got %q", got)
	}
	if len(m.slash.matches) == 0 {
		t.Fatal("expected slash matches to remain after tab completion")
	}
}

func TestSlashSuggestionEscClearsSuggestionsWithoutMutatingInput(t *testing.T) {
	m := newModel(nil, "", "", "")
	m.input.SetValue("/co")
	m.updateSlashMatches()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(model)
	if got := m.input.Value(); got != "/co" {
		t.Fatalf("expected esc to preserve input, got %q", got)
	}
	if len(m.slash.matches) != 0 || m.slash.selected != 0 {
		t.Fatalf("expected esc to clear slash suggestions, got matches=%v selected=%d", m.slash.matches, m.slash.selected)
	}
}

func TestSlashSuggestionPlanAutoRunsWhenSelected(t *testing.T) {
	m, intents := newModelWithDispatchSpy()
	m.input.SetValue("/pl")
	m.updateSlashMatches()
	selectSlashCommand(t, &m, "/plan")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if len(*intents) != 1 {
		t.Fatalf("expected one dispatch for selected /plan, got %+v", *intents)
	}
	if (*intents)[0].Kind != service.IntentSubmit || (*intents)[0].Input != "/plan" {
		t.Fatalf("unexpected dispatched intent: %+v", (*intents)[0])
	}
	if got := m.input.Value(); got != "" {
		t.Fatalf("expected input cleared after /plan autorun, got %q", got)
	}
	if !m.busy || m.status != "running" {
		t.Fatalf("expected running state for /plan autorun, busy=%v status=%q", m.busy, m.status)
	}
}

func TestSlashSuggestionAskAutoRunsWhenSelected(t *testing.T) {
	m, intents := newModelWithDispatchSpy()
	m.input.SetValue("/as")
	m.updateSlashMatches()
	selectSlashCommand(t, &m, "/ask")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if len(*intents) != 1 {
		t.Fatalf("expected one dispatch for selected /ask, got %+v", *intents)
	}
	if (*intents)[0].Kind != service.IntentSubmit || (*intents)[0].Input != "/ask" {
		t.Fatalf("unexpected dispatched intent: %+v", (*intents)[0])
	}
	if got := m.input.Value(); got != "" {
		t.Fatalf("expected input cleared after /ask autorun, got %q", got)
	}
	if !m.busy || m.status != "running" {
		t.Fatalf("expected running state for /ask autorun, busy=%v status=%q", m.busy, m.status)
	}
}

func TestCtrlCClearsNonEmptyInputWithoutArmingQuit(t *testing.T) {
	m := newModel(nil, "", "", "")
	m.input.SetValue("draft")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	m = next.(model)
	if cmd != nil {
		t.Fatalf("expected no command when clearing input, got %T", cmd)
	}
	if got := m.input.Value(); got != "" {
		t.Fatalf("expected input cleared, got %q", got)
	}
	if !m.quitArmedUntil.IsZero() {
		t.Fatal("expected ctrl+c on non-empty input not to arm quit")
	}
	if m.status != "input cleared" {
		t.Fatalf("unexpected status: %q", m.status)
	}
}

func TestApprovalNoticeTextUsesDecisionAndSummary(t *testing.T) {
	m := newModel(nil, "", "", "")
	m.approval.reason = "exec_shell: go test ./..."
	if got := m.approvalNoticeText("allow"); !strings.Contains(got, "You approved whale to run go test ./... this time") {
		t.Fatalf("unexpected allow notice: %q", got)
	}
	if got := m.approvalNoticeText("allow_session"); !strings.Contains(got, "for this session") {
		t.Fatalf("unexpected session notice: %q", got)
	}
	if got := m.approvalNoticeText("deny"); !strings.Contains(got, "You canceled the request to run go test ./...") {
		t.Fatalf("unexpected deny notice: %q", got)
	}
}

func TestTurnInterruptedNoticeText(t *testing.T) {
	m := newModel(nil, "", "", "")
	got := m.turnInterruptedNoticeText()
	if !strings.Contains(got, "Conversation interrupted") {
		t.Fatalf("unexpected interrupt notice: %q", got)
	}
}

func TestEscWhileBusyKeepsTurnBusyUntilTurnDone(t *testing.T) {
	m := model{assembler: tuirender.NewAssembler(), mode: modeChat, width: 80, height: 24, busy: true}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = next.(model)
	if !m.busy {
		t.Fatal("expected interrupted turn to remain busy until EventTurnDone")
	}
	if !m.stopping {
		t.Fatal("expected stopping state after interrupt")
	}
	if m.status != "stopping" {
		t.Fatalf("unexpected status: %q", m.status)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if !m.busy || !m.stopping {
		t.Fatalf("enter during stopping should not start another turn, busy=%v stopping=%v", m.busy, m.stopping)
	}

	next, _ = m.Update(svcMsg(service.Event{Kind: service.EventTurnDone}))
	m = next.(model)
	if m.busy || m.stopping {
		t.Fatalf("expected turn done to clear busy/stopping, busy=%v stopping=%v", m.busy, m.stopping)
	}
}

func TestPlanCompletedReplacesPartialPlanAndTurnDoneShowsPicker(t *testing.T) {
	m := model{
		assembler: tuirender.NewAssembler(),
		mode:      modeChat,
		chatMode:  "plan",
		busy:      true,
		status:    "working",
	}
	next, _ := m.Update(svcMsg(service.Event{Kind: service.EventPlanDelta, Text: "partial"}))
	m = next.(model)
	next, cmd := m.Update(svcMsg(service.Event{Kind: service.EventPlanCompleted, Text: "complete final plan"}))
	m = next.(model)
	if cmd == nil {
		t.Fatal("expected wait-event command")
	}
	if snap := m.assembler.Snapshot(); len(snap) != 0 {
		t.Fatalf("expected completed plan to leave live assembler empty, got %+v", snap)
	}
	if len(m.transcript) != 1 || m.transcript[0].Kind != tuirender.KindPlan {
		t.Fatalf("expected completed plan in transcript, got %+v", m.transcript)
	}
	next, _ = m.Update(svcMsg(service.Event{Kind: service.EventTurnDone, LastResponse: "done"}))
	m = next.(model)

	if m.mode != modePlanImplementation {
		t.Fatalf("expected implementation picker, got mode %v", m.mode)
	}
	if m.busy {
		t.Fatal("expected busy=false after turn done")
	}
	snap := m.assembler.Snapshot()
	if len(snap) != 0 {
		t.Fatalf("expected completed turn to move plan out of live assembler, got %+v", snap)
	}
}

func TestStalePlanCompletionDoesNotOpenPickerOnLaterTurnDone(t *testing.T) {
	m := model{
		assembler: tuirender.NewAssembler(),
		mode:      modeChat,
		chatMode:  "plan",
		busy:      false,
		status:    "ready",
	}
	next, _ := m.Update(svcMsg(service.Event{Kind: service.EventPlanCompleted, Text: "complete final plan"}))
	m = next.(model)
	next, _ = m.Update(svcMsg(service.Event{Kind: service.EventTurnDone, LastResponse: "status"}))
	m = next.(model)

	if m.mode == modePlanImplementation {
		t.Fatal("stale plan completion opened implementation picker")
	}
	if m.sawPlanThisTurn {
		t.Fatal("expected stale plan flag to reset after turn done")
	}
}

func TestPlanCompletedWithoutDeltasStillRendersPlan(t *testing.T) {
	m := model{
		assembler: tuirender.NewAssembler(),
		mode:      modeChat,
		chatMode:  "plan",
		busy:      true,
	}
	plan := strings.Repeat("final plan\n", 100)
	next, cmd := m.Update(svcMsg(service.Event{Kind: service.EventPlanCompleted, Text: plan}))
	m = next.(model)
	if cmd == nil {
		t.Fatal("expected wait-event command")
	}
	if snap := m.assembler.Snapshot(); len(snap) != 0 {
		t.Fatalf("expected final plan to leave live assembler empty, got %+v", snap)
	}
	if len(m.transcript) != 1 || m.transcript[0].Kind != tuirender.KindPlan {
		t.Fatalf("expected final plan in transcript, got %+v", m.transcript)
	}
	next, _ = m.Update(svcMsg(service.Event{Kind: service.EventTurnDone, LastResponse: "done"}))
	m = next.(model)
	snap := m.assembler.Snapshot()
	if len(snap) != 0 {
		t.Fatalf("expected final plan to be committed and live assembler cleared, got %+v", snap)
	}
	if m.mode != modePlanImplementation {
		t.Fatalf("expected implementation picker, got mode %v", m.mode)
	}
}

func TestScrollbackTextRendersUserMessage(t *testing.T) {
	m := model{width: 80, height: 24}
	got := m.scrollbackText([]tuirender.UIMessage{{
		Role: "you",
		Kind: tuirender.KindText,
		Text: "hello whale",
	}})
	if !strings.Contains(got, "hello whale") {
		t.Fatalf("expected user text in scrollback output, got %q", got)
	}
}

func TestCommitLiveTranscriptClearsAssembler(t *testing.T) {
	m := model{assembler: tuirender.NewAssembler(), width: 80, height: 24}
	m.append("assistant", "streamed answer")
	m.commitLiveTranscript(true)
	if got := len(m.assembler.Snapshot()); got != 0 {
		t.Fatalf("expected live assembler cleared after commit, got %d entries", got)
	}
	if len(m.transcript) != 1 || m.transcript[0].Text != "streamed answer" {
		t.Fatalf("expected committed transcript entry, got %+v", m.transcript)
	}
}

func TestAssistantDeltaKeepsMultilineBlockLiveUntilBoundary(t *testing.T) {
	m := newModel(nil, "", "", "")
	m.width = 80
	m.height = 24
	next, _ := m.Update(svcMsg(service.Event{Kind: service.EventAssistantDelta, Text: "stable line\nlive tail"}))
	m = next.(model)
	snap := m.assembler.Snapshot()
	if len(snap) != 1 || snap[0].Text != "stable line\nlive tail" {
		t.Fatalf("expected newline-delimited assistant content to stay in one live message, got %+v", snap)
	}
	view := m.View()
	if !strings.Contains(view, "stable line") {
		t.Fatalf("expected first line to remain in the same live block:\n%s", view)
	}
	if !strings.Contains(view, "live tail") {
		t.Fatalf("expected tail to remain in the same live block:\n%s", view)
	}
}

func TestReasoningDeltaKeepsSingleThinkingCardAcrossNewlines(t *testing.T) {
	m := newModel(nil, "", "", "")
	m.width = 80
	m.height = 24
	next, _ := m.Update(svcMsg(service.Event{Kind: service.EventReasoningDelta, Text: "first thought\n\nsecond thought\nthird thought"}))
	m = next.(model)
	snap := m.assembler.Snapshot()
	if len(snap) != 1 || snap[0].Role != "think" {
		t.Fatalf("expected reasoning content to stay in one live thinking message, got %+v", snap)
	}
	lines := m.renderChatLines(80)
	joined := strings.Join(lines, "\n")
	if got := strings.Count(joined, "Thinking"); got != 1 {
		t.Fatalf("expected one thinking card, got %d:\n%s", got, joined)
	}
	for _, want := range []string{"first thought", "second thought", "third thought"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected %q in thinking card:\n%s", want, joined)
		}
	}
}

func TestSessionHydrationCommitsTranscriptAndClearsLiveAssembler(t *testing.T) {
	m := model{assembler: tuirender.NewAssembler(), mode: modeChat, width: 80, height: 24}
	next, cmd := m.Update(svcMsg(service.Event{
		Kind: service.EventSessionHydrated,
		Messages: []core.Message{
			{Role: core.RoleUser, Text: "hi"},
			{Role: core.RoleAssistant, Text: "hello"},
		},
	}))
	m = next.(model)
	if cmd == nil {
		t.Fatal("expected hydration to return wait-event command")
	}
	if got := len(m.assembler.Snapshot()); got != 0 {
		t.Fatalf("expected hydrated transcript committed out of live assembler, got %d entries", got)
	}
	if got := strings.Join(tuirender.ChatLines(m.transcript, 80), "\n"); !strings.Contains(got, "hi") || !strings.Contains(got, "hello") {
		t.Fatalf("expected hydrated messages in transcript:\n%s", got)
	}
}

func TestChatIdleViewDoesNotRenderEmptyViewportFrame(t *testing.T) {
	m := newModel(nil, "", "", "")
	m.width = 80
	m.height = 24
	view := m.View()
	if strings.Contains(view, "┌") || strings.Contains(view, "│\n│") {
		t.Fatalf("idle chat view should not render an empty bordered viewport:\n%s", view)
	}
	if !strings.Contains(view, "Type message or command") {
		t.Fatalf("expected composer placeholder in idle view:\n%s", view)
	}
	if strings.Contains(view, "status: ready") {
		t.Fatalf("idle view should not render ready status in footer:\n%s", view)
	}
	if strings.Contains(view, "Working (") || strings.Contains(view, "Stopping (") {
		t.Fatalf("idle view should not render busy status line:\n%s", view)
	}
}

func TestChatFooterStaysPinnedAfterSlashSuggestionsClose(t *testing.T) {
	m := newModel(nil, "deepseek-v4-pro", "normal", "on")
	m.width = 80
	m.height = 24
	m.cwd = "~/Engineer/ai/dsk/whale"

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = next.(model)
	withSlash := m.View()
	assertFooterLastLine(t, withSlash, "model: deepseek-v4-pro")
	assertFooterLastLine(t, withSlash, "whale")
	assertFooterLastLineNotContains(t, withSlash, "dir:")
	if !strings.Contains(withSlash, "Tab/Enter pick") {
		t.Fatalf("expected slash suggestions while / is present:\n%s", withSlash)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = next.(model)
	afterDelete := m.View()
	assertFooterLastLine(t, afterDelete, "model: deepseek-v4-pro")
	assertFooterLastLine(t, afterDelete, "whale")
	assertFooterLastLineNotContains(t, afterDelete, "dir:")
	if strings.Contains(afterDelete, "Tab/Enter pick") {
		t.Fatalf("expected slash suggestions to disappear after deleting /:\n%s", afterDelete)
	}
	if got := strings.Count(afterDelete, "\n") + 1; got != m.height {
		t.Fatalf("expected view to keep terminal height %d after slash closes, got %d:\n%s", m.height, got, afterDelete)
	}
}

func TestChatTranscriptRetainsLocalCommandResultsAcrossSubmits(t *testing.T) {
	m, _ := newModelWithDispatchSpy()
	m.width = 80
	m.height = 14
	m.appendTranscript("you", tuirender.KindText, "/mcp")
	next, _ := m.Update(svcMsg(service.Event{Kind: service.EventInfo, Text: "MCP\n\nconfig: /tmp/mcp.json servers: none"}))
	m = next.(model)
	next, _ = m.Update(svcMsg(service.Event{Kind: service.EventTurnDone, LastResponse: "MCP"}))
	m = next.(model)

	m.input.SetValue("/status")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	next, _ = m.Update(svcMsg(service.Event{Kind: service.EventInfo, Text: "Status\n\nsession: test-session"}))
	m = next.(model)
	next, _ = m.Update(svcMsg(service.Event{Kind: service.EventTurnDone, LastResponse: "Status"}))
	m = next.(model)

	got := strings.Join(tuirender.ChatLines(m.transcript, 80), "\n")
	for _, want := range []string{"/mcp", "config: /tmp/mcp.json", "/status", "session: test-session"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected transcript to retain %q:\n%s", want, got)
		}
	}
}

func TestChatStartupHeaderRendersInsideViewportHeight(t *testing.T) {
	m := newModel(nil, "deepseek-v4-flash", "high", "on")
	m.width = 80
	m.height = 10
	view := m.View()
	if !strings.Contains(view, "▸ Whale") {
		t.Fatalf("expected startup header in chat view:\n%s", view)
	}
	if got := strings.Count(strings.TrimRight(view, "\n"), "\n") + 1; got != m.height {
		t.Fatalf("expected view to keep terminal height %d, got %d:\n%s", m.height, got, view)
	}
}

func TestChatViewportScrollsTranscript(t *testing.T) {
	m := newModel(nil, "", "", "")
	m.width = 80
	m.height = 8
	m.transcript = nil
	for i := 0; i < 20; i++ {
		m.appendTranscript("info", tuirender.KindText, fmt.Sprintf("entry-%02d", i))
	}
	m.refreshViewportContentFollow(true)
	atBottom := m.View()
	if strings.Contains(atBottom, "entry-00") || !strings.Contains(atBottom, "entry-19") {
		t.Fatalf("expected bottom view to show tail only:\n%s", atBottom)
	}

	m.handleViewportScrollKey("home")
	atTop := m.View()
	if !strings.Contains(atTop, "entry-00") {
		t.Fatalf("expected home to scroll chat transcript to top:\n%s", atTop)
	}
}

func TestChatLiveViewRendersWithoutViewportFrame(t *testing.T) {
	m := newModel(nil, "", "", "")
	m.width = 80
	m.height = 24
	m.append("assistant", "streamed answer")
	view := m.View()
	if !strings.Contains(view, "streamed answer") {
		t.Fatalf("expected live assistant text in view:\n%s", view)
	}
	if strings.Contains(view, "┌") {
		t.Fatalf("live chat view should not render bordered viewport:\n%s", view)
	}
}

func assertFooterLastLine(t *testing.T, view, want string) {
	t.Helper()
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) == 0 {
		t.Fatal("empty view")
	}
	if got := lines[len(lines)-1]; !strings.Contains(got, want) {
		t.Fatalf("expected footer %q on last line, got %q in view:\n%s", want, got, view)
	}
}

func assertFooterLastLineNotContains(t *testing.T, view, unwanted string) {
	t.Helper()
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	if len(lines) == 0 {
		t.Fatal("empty view")
	}
	if got := lines[len(lines)-1]; strings.Contains(got, unwanted) {
		t.Fatalf("expected footer not to contain %q, got %q in view:\n%s", unwanted, got, view)
	}
}

func TestChatBusyViewShowsWorkingAboveComposer(t *testing.T) {
	m := newModel(nil, "", "", "")
	m.width = 80
	m.height = 24
	m.startBusy()
	m.busySince = time.Now().Add(-12 * time.Second)
	view := m.View()
	if !strings.Contains(view, "Working (12s)") {
		t.Fatalf("expected working status line with elapsed time:\n%s", view)
	}
	if strings.Contains(view, "status: working") {
		t.Fatalf("busy view should not render footer status:\n%s", view)
	}
	if strings.Index(view, "Working (12s)") > strings.Index(view, "Type message or command") {
		t.Fatalf("working status line should appear above composer:\n%s", view)
	}
}

func TestChatStoppingViewShowsStoppingAboveComposer(t *testing.T) {
	m := newModel(nil, "", "", "")
	m.width = 80
	m.height = 24
	m.startBusy()
	m.stopping = true
	m.busySince = time.Now().Add(-(time.Minute + 5*time.Second))
	view := m.View()
	if !strings.Contains(view, "Stopping (1m 05s)") {
		t.Fatalf("expected stopping status line with continued elapsed time:\n%s", view)
	}
}

func TestApprovalViewHidesToolCallID(t *testing.T) {
	m := newModel(nil, "", "", "")
	m.width = 80
	m.height = 24
	m.mode = modeApproval
	m.approval.toolCallID = "tc-123"
	m.approval.toolName = "edit"
	m.approval.reason = "edit: internal/tui/model.go"
	view := m.View()
	if !strings.Contains(view, "approval: edit") {
		t.Fatalf("expected approval header in view:\n%s", view)
	}
	if strings.Contains(view, "id: tc-123") {
		t.Fatalf("approval view should not expose tool call id:\n%s", view)
	}
}

func TestApprovalViewShowsDiffMetadata(t *testing.T) {
	m := newModel(nil, "", "", "")
	m.width = 100
	m.height = 30
	m.mode = modeApproval
	m.approval.toolName = "edit"
	m.approval.reason = "edit: a.txt"
	m.approval.metadata = testFileDiffMetadata()
	view := m.View()
	if !strings.Contains(view, "--- a/a.txt") || !strings.Contains(view, "+whale") {
		t.Fatalf("expected approval diff metadata in view:\n%s", view)
	}
}

func TestToolResultShowsDiffMetadata(t *testing.T) {
	m := model{assembler: tuirender.NewAssembler(), mode: modeChat, width: 100, height: 30}
	next, _ := m.Update(svcMsg(service.Event{
		Kind:       service.EventToolCall,
		ToolCallID: "tc-1",
		ToolName:   "edit",
		Text:       `edit: a.txt`,
	}))
	m = next.(model)
	next, cmd := m.Update(svcMsg(service.Event{
		Kind:       service.EventToolResult,
		ToolCallID: "tc-1",
		ToolName:   "edit",
		Text:       `{"success":true,"data":{"payload":{"file_path":"a.txt","replacements":1}}}`,
		Metadata:   testFileDiffMetadata(),
	}))
	m = next.(model)
	if cmd == nil {
		t.Fatal("expected wait-event command")
	}
	snap := m.assembler.Snapshot()
	if len(snap) != 0 {
		t.Fatalf("expected completed tool cell to leave live assembler empty, got %+v", snap)
	}
	if got := strings.Join(tuirender.ChatLines(m.transcript, 100), "\n"); !strings.Contains(got, "Edited a.txt") {
		t.Fatalf("expected completed tool cell in transcript:\n%s", got)
	}
	if got := strings.Join(m.renderDiffs(), "\n"); !strings.Contains(got, "+whale") {
		t.Fatalf("expected /diff content from metadata:\n%s", got)
	}
}

func testFileDiffMetadata() map[string]any {
	return map[string]any{
		"kind": "file_diff",
		"files": []any{
			map[string]any{
				"path":         "a.txt",
				"unified_diff": "--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-world\n+whale",
				"additions":    1,
				"deletions":    1,
			},
		},
	}
}

func TestChatLiveViewUsesViewportForLongOutput(t *testing.T) {
	m := newModel(nil, "", "", "")
	m.width = 80
	m.height = 8
	m.append("assistant", strings.Repeat("line\n", 80))
	view := m.View()
	if !strings.Contains(view, "Type message or command") {
		t.Fatalf("expected composer to remain visible with long live output:\n%s", view)
	}
	if got := strings.Count(strings.TrimRight(view, "\n"), "\n") + 1; got != m.height {
		t.Fatalf("expected view to keep terminal height %d, got %d:\n%s", m.height, got, view)
	}
}

func TestFormatElapsedCompact(t *testing.T) {
	cases := []struct {
		elapsed time.Duration
		want    string
	}{
		{elapsed: 0, want: "0s"},
		{elapsed: 12 * time.Second, want: "12s"},
		{elapsed: time.Minute + 5*time.Second, want: "1m 05s"},
		{elapsed: time.Hour + 2*time.Minute + 9*time.Second, want: "1h 02m 09s"},
	}
	for _, tc := range cases {
		if got := formatElapsedCompact(tc.elapsed); got != tc.want {
			t.Fatalf("formatElapsedCompact(%v) = %q, want %q", tc.elapsed, got, tc.want)
		}
	}
}

func TestSummarizeToolResultForChat_ExecShellSuccessShowsOutputSummary(t *testing.T) {
	raw := `{"success":true,"code":"ok","data":{"status":"ok","metrics":{"exit_code":0,"duration_ms":29},"payload":{"command":"date","stdout":"Sun May 3\n","stderr":""}}}`
	role, got := summarizeToolResultForChat("exec_shell", raw)
	if role != "result_ok" {
		t.Fatalf("expected result_ok role, got %q", role)
	}
	want := "✓ · 29ms\nSun May 3"
	if got != want {
		t.Fatalf("unexpected summary text:\nwant: %q\ngot:  %q", want, got)
	}
}

func TestSummarizeToolResultForChat_ExecShellFailureShowsReason(t *testing.T) {
	raw := `{"success":false,"code":"exec_failed","message":"command failed","data":{"status":"error","summary":"command failed","metrics":{"exit_code":2,"duration_ms":1210},"payload":{"stderr":"ls: cannot access x: No such file or directory\n","stdout":""}}}`
	role, got := summarizeToolResultForChat("exec_shell", raw)
	if role != "result_failed" {
		t.Fatalf("expected result_failed role, got %q", role)
	}
	want := "✗ (exit 2) · 1.2s · ls: cannot access x: No such file or directory"
	if got != want {
		t.Fatalf("unexpected summary text:\nwant: %q\ngot:  %q", want, got)
	}
}

func TestSummarizeToolResultForChat_RequestReplanHidesInternalRecoveryText(t *testing.T) {
	raw := `{"success":false,"code":"request_replan","error":"recovery exhausted, replan required","data":{"tool_name":"mcp__fs__search_files","last_error":"{\"success\":false,\"code\":\"mcp_tool_error\",\"error\":\"Error: Access denied - path outside allowed directories: /workspace not in /tmp\"}"}}`
	role, got := summarizeToolResultForChat("mcp__fs__search_files", raw)
	if role != "result_failed" {
		t.Fatalf("expected result_failed role, got %q", role)
	}
	if strings.Contains(got, "recovery exhausted") || strings.Contains(got, "replan required") {
		t.Fatalf("summary leaked internal recovery text: %q", got)
	}
	if !strings.Contains(got, "DENIED") || !strings.Contains(got, "outside allowed directories") {
		t.Fatalf("expected user-facing access denial, got %q", got)
	}
}

func TestSummarizeToolResultForChat_PermissionDeniedShowsDenied(t *testing.T) {
	raw := `{"success":false,"code":"permission_denied","message":"path outside MCP fs allowed directories: /workspace not in /tmp"}`
	role, got := summarizeToolResultForChat("mcp__fs__search_files", raw)
	if role != "result_denied" {
		t.Fatalf("expected result_denied role, got %q", role)
	}
	want := "DENIED · path outside MCP fs allowed directories: /workspace not in /tmp"
	if got != want {
		t.Fatalf("unexpected summary:\nwant: %q\ngot:  %q", want, got)
	}
}

func TestSummarizeToolResultForChat_NonShellSummarized(t *testing.T) {
	raw := `{"success":true,"data":{"metrics":{"total_matches":3},"payload":{"items":["a.go","b.go","c.go"]}}}`
	role, got := summarizeToolResultForChat("search_files", raw)
	if role != "result_ok" {
		t.Fatalf("expected result_ok role for non-shell, got: %q", role)
	}
	if got != "✓ · 3 matches" {
		t.Fatalf("expected summarized non-shell payload, got: %q", got)
	}
	if strings.Contains(got, "{") || strings.Contains(got, "payload") {
		t.Fatalf("summary must not expose raw json: %q", got)
	}
}

func TestSummarizeToolResultForChat_Denied(t *testing.T) {
	raw := `{"success":false,"code":"approval_denied","message":"tool approval denied"}`
	role, got := summarizeToolResultForChat("exec_shell", raw)
	if role != "result_denied" || got != "DENIED · tool approval denied" {
		t.Fatalf("unexpected denied summary: role=%q text=%q", role, got)
	}
}

func TestSummarizeToolResultForChat_AskModeBlockedShowsProductCommands(t *testing.T) {
	raw := `{"success":false,"code":"ask_mode_blocked","message":"tool unavailable in ask mode","summary":"Current mode: ask. Ask mode only allows read-only tools. To execute or modify files, switch to agent mode. To propose a reviewed approach first, switch to plan mode.","data":{"current_mode":"ask","suggested_modes":["agent","plan"]}}`
	role, got := summarizeToolResultForChat("exec_shell", raw)
	if role != "result_failed" {
		t.Fatalf("expected result_failed role, got %q", role)
	}
	want := "✗ · Current mode: ask. Ask mode only allows read-only tools. To execute or modify files, switch to agent mode. To propose a reviewed approach first, switch to plan mode."
	if got != want {
		t.Fatalf("unexpected ask-mode summary:\nwant: %q\ngot:  %q", want, got)
	}
}

func TestSummarizeToolResultForChat_Timeout(t *testing.T) {
	raw := `{"success":false,"code":"timeout","message":"command timed out","data":{"metrics":{"duration_ms":15000}}}`
	role, got := summarizeToolResultForChat("exec_shell", raw)
	if role != "result_timeout" || got != "TIMEOUT · 15s" {
		t.Fatalf("unexpected timeout summary: role=%q text=%q", role, got)
	}
}

func TestSummarizeToolResultForChat_Canceled(t *testing.T) {
	raw := `{"success":false,"code":"cancelled","message":"context canceled"}`
	role, got := summarizeToolResultForChat("exec_shell", raw)
	if role != "result_canceled" || got != "CANCELED" {
		t.Fatalf("unexpected canceled summary: role=%q text=%q", role, got)
	}
}

func TestToolDeniedDoesNotAddNoFinalAnswerNotice(t *testing.T) {
	m := model{assembler: tuirender.NewAssembler(), mode: modeChat, width: 80, height: 24, busy: true}
	next, _ := m.Update(svcMsg(service.Event{Kind: service.EventReasoningDelta, Text: "I should edit the file."}))
	m = next.(model)
	next, _ = m.Update(svcMsg(service.Event{
		Kind:       service.EventToolCall,
		ToolCallID: "tc-1",
		ToolName:   "edit",
		Text:       `edit: internal/tui/model.go`,
	}))
	m = next.(model)
	next, _ = m.Update(svcMsg(service.Event{
		Kind:       service.EventToolResult,
		ToolCallID: "tc-1",
		ToolName:   "edit",
		Text:       `{"success":false,"code":"approval_denied","message":"tool approval denied"}`,
	}))
	m = next.(model)
	next, _ = m.Update(svcMsg(service.Event{Kind: service.EventTurnDone}))
	m = next.(model)
	snap := m.assembler.Snapshot()
	for _, entry := range snap {
		if strings.Contains(entry.Text, "No final answer was produced") {
			t.Fatalf("unexpected no-final-answer notice after tool denial: %+v", snap)
		}
	}
}

func TestSummarizeToolResultForChat_FailedNoExitCodeDoesNotShowZero(t *testing.T) {
	raw := `{"success":false,"code":"exec_failed","message":"command failed","data":{"metrics":{"duration_ms":41},"payload":{"stderr":"unknown flag: --bad"}}}`
	role, got := summarizeToolResultForChat("exec_shell", raw)
	if role != "result_failed" {
		t.Fatalf("expected result_failed role, got %q", role)
	}
	if got == "✗ (exit 0) · 41ms · unknown flag: --bad" {
		t.Fatalf("must not show fake exit 0: %q", got)
	}
	if got != "✗ · 41ms · unknown flag: --bad" {
		t.Fatalf("unexpected failed summary: %q", got)
	}
}

func TestSummarizeToolResultForChat_OkWithoutSuccessField(t *testing.T) {
	raw := `{"code":"ok","data":{"status":"ok","metrics":{"exit_code":0,"duration_ms":237},"payload":{"stdout":"142.251.214.110","stderr":""}}}`
	role, got := summarizeToolResultForChat("exec_shell", raw)
	if role != "result_ok" {
		t.Fatalf("expected result_ok role, got %q", role)
	}
	if got != "✓ · 237ms\n142.251.214.110" {
		t.Fatalf("unexpected summary: %q", got)
	}
}

func TestSummarizeToolResultForChat_ShellOutputTruncated(t *testing.T) {
	stdout := strings.Join([]string{
		"l1", "l2", "l3", "l4", "l5", "l6", "l7", "l8", "l9", "l10", "l11", "l12", "l13", "l14",
	}, `\n`) + `\n`
	raw := `{"success":true,"data":{"status":"ok","payload":{"stdout":"` + stdout + `"}}}`
	role, got := summarizeToolResultForChat("exec_shell", raw)
	if role != "result_ok" {
		t.Fatalf("expected result_ok role, got %q", role)
	}
	for _, want := range []string{"l1", "l2", "l13", "l14"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected compact output to keep %q, got: %q", want, got)
		}
	}
	if strings.Contains(got, "l3") || strings.Contains(got, "l12") {
		t.Fatalf("expected middle output to be omitted, got: %q", got)
	}
	if !strings.Contains(got, "10 lines omitted; use /tool for full output") {
		t.Fatalf("expected omitted output marker, got: %q", got)
	}
}

func TestToolResultUpdatesToolCellWithoutRawJSON(t *testing.T) {
	m := model{assembler: tuirender.NewAssembler(), mode: modeChat}
	next, _ := m.Update(svcMsg(service.Event{Kind: service.EventToolCall, ToolCallID: "tc-1", ToolName: "read_file", Text: `read_file: {"file_path":"internal/tui/model.go"}`}))
	m = next.(model)
	raw := `{"success":true,"data":{"status":"ok","metrics":{"returned_lines":24,"total_lines":100},"payload":{"file_path":"internal/tui/model.go","content":"package tui"}}}`
	next, cmd := m.Update(svcMsg(service.Event{Kind: service.EventToolResult, ToolCallID: "tc-1", ToolName: "read_file", Text: raw}))
	m = next.(model)
	if cmd == nil {
		t.Fatal("expected wait-event command")
	}
	snap := m.assembler.Snapshot()
	if len(snap) != 0 {
		t.Fatalf("expected completed read cell to leave live assembler empty, got %+v", snap)
	}
	if got := strings.Join(tuirender.ChatLines(m.transcript, 80), "\n"); !strings.Contains(got, "Read internal/tui/model.go") {
		t.Fatalf("expected completed read cell in transcript:\n%s", got)
	}
}

func TestToolCallShowsSearchPatternAndPath(t *testing.T) {
	m := model{assembler: tuirender.NewAssembler(), mode: modeChat}
	next, _ := m.Update(svcMsg(service.Event{
		Kind:       service.EventToolCall,
		ToolCallID: "tc-search",
		ToolName:   "grep",
		Text:       `grep: assistant_delta in internal/tui (*.go)`,
	}))
	m = next.(model)
	snap := m.assembler.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected one tool cell, got %+v", snap)
	}
	want := "Exploring\nSearch assistant_delta in internal/tui (*.go)"
	if snap[0].Text != want {
		t.Fatalf("unexpected search tool call text:\nwant: %q\ngot:  %q", want, snap[0].Text)
	}
}

func TestToolResultKeepsSearchDetailAndAddsSummary(t *testing.T) {
	m := model{assembler: tuirender.NewAssembler(), mode: modeChat}
	next, _ := m.Update(svcMsg(service.Event{
		Kind:       service.EventToolCall,
		ToolCallID: "tc-search",
		ToolName:   "grep",
		Text:       `grep: assistant_delta in internal/tui (*.go)`,
	}))
	m = next.(model)
	raw := `{"success":true,"data":{"status":"ok","metrics":{"total_matches":1,"files_matched":1},"payload":{"matches":[]}}}`
	next, cmd := m.Update(svcMsg(service.Event{
		Kind:       service.EventToolResult,
		ToolCallID: "tc-search",
		ToolName:   "grep",
		Text:       raw,
	}))
	m = next.(model)
	if cmd == nil {
		t.Fatal("expected wait-event command")
	}
	snap := m.assembler.Snapshot()
	if len(snap) != 0 {
		t.Fatalf("expected completed search cell to leave live assembler empty, got %+v", snap)
	}
	if got := strings.Join(tuirender.ChatLines(m.transcript, 80), "\n"); !strings.Contains(got, "Search assistant_delta in internal/tui") {
		t.Fatalf("expected completed search cell in transcript:\n%s", got)
	}
}

func TestToolCallShowsWebSearchQuery(t *testing.T) {
	m := model{assembler: tuirender.NewAssembler(), mode: modeChat}
	next, _ := m.Update(svcMsg(service.Event{
		Kind:       service.EventToolCall,
		ToolCallID: "tc-web",
		ToolName:   "web_search",
		Text:       `web_search: F1 pit strategy tools`,
	}))
	m = next.(model)
	snap := m.assembler.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected one web search cell, got %+v", snap)
	}
	want := "Exploring\nSearch web for F1 pit strategy tools"
	if snap[0].Text != want {
		t.Fatalf("unexpected web search tool call text:\nwant: %q\ngot:  %q", want, snap[0].Text)
	}
}

func TestClearScreenResetsStateAndShowsHeader(t *testing.T) {
	m := model{
		assembler: tuirender.NewAssembler(),
		mode:      modeChat,
		width:     80,
		height:    24,
		model:     "deepseek-v4-flash",
		effort:    "high",
		cwd:       "~/work",
		version:   "v0.1.0",
	}
	// Add some state
	m.assembler.AddNotice("old notice")
	m.logs = []logEntry{{Kind: "info", Summary: "old"}}
	m.diffs = []diffEntry{{Source: "x", Line: "old"}}
	m.status = "ready"

	next, cmd := m.Update(svcMsg(service.Event{Kind: service.EventClearScreen}))
	m2 := next.(model)

	if cmd == nil {
		t.Fatal("expected clear screen to return a command")
	}
	if m2.status != "terminal cleared" {
		t.Fatalf("expected status 'terminal cleared', got %q", m2.status)
	}
	if len(m2.logs) != 0 {
		t.Fatalf("expected logs cleared, got %d", len(m2.logs))
	}
	if len(m2.diffs) != 0 {
		t.Fatalf("expected diffs cleared, got %d", len(m2.diffs))
	}
	// The transcript should keep only the header banner after clear.
	snap := m2.assembler.Snapshot()
	if len(snap) != 0 {
		t.Fatalf("expected empty live assembler, got %+v", snap)
	}
	if len(m2.transcript) != 1 {
		t.Fatalf("expected 1 transcript header, got %d: %+v", len(m2.transcript), m2.transcript)
	}
	if m2.transcript[0].Role != "info" {
		t.Fatalf("expected info role, got %q", m2.transcript[0].Role)
	}
	if !strings.Contains(m2.transcript[0].Text, "▸ Whale") {
		t.Fatalf("expected header banner, got: %q", m2.transcript[0].Text)
	}
}

func containsString(xs []string, target string) bool {
	for _, x := range xs {
		if x == target {
			return true
		}
	}
	return false
}
