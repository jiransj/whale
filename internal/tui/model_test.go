package tui

import (
	"strings"
	"testing"

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

func TestTurnDoneReasoningOnlyCommitsFallback(t *testing.T) {
	m := model{assembler: tuirender.NewAssembler(), mode: modeChat, width: 80, height: 24, busy: true}
	next, _ := m.Update(svcMsg(service.Event{Kind: service.EventReasoningDelta, Text: "I should answer."}))
	m = next.(model)
	next, cmd := m.Update(svcMsg(service.Event{Kind: service.EventTurnDone}))
	m = next.(model)
	if cmd == nil {
		t.Fatal("expected scrollback command")
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
	next, _ = m.Update(svcMsg(service.Event{Kind: service.EventPlanCompleted, Text: "complete final plan"}))
	m = next.(model)
	if snap := m.assembler.Snapshot(); len(snap) != 1 || snap[0].Text != "complete final plan" {
		t.Fatalf("expected complete plan to replace partial plan before commit, got %+v", snap)
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
	next, _ := m.Update(svcMsg(service.Event{Kind: service.EventPlanCompleted, Text: plan}))
	m = next.(model)
	if snap := m.assembler.Snapshot(); len(snap) != 1 || !strings.Contains(snap[0].Text, "final plan") {
		t.Fatalf("expected final plan rendered from completion event before commit, got %+v", snap)
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

func TestCommitLiveScrollbackClearsAssembler(t *testing.T) {
	m := model{assembler: tuirender.NewAssembler(), width: 80, height: 24}
	m.append("assistant", "streamed answer")
	cmd := m.commitLiveScrollbackCmd()
	if cmd == nil {
		t.Fatal("expected scrollback print command")
	}
	if got := len(m.assembler.Snapshot()); got != 0 {
		t.Fatalf("expected live assembler cleared after commit, got %d entries", got)
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
		t.Fatal("expected hydration to return scrollback command")
	}
	if got := len(m.assembler.Snapshot()); got != 0 {
		t.Fatalf("expected hydrated transcript committed out of live assembler, got %d entries", got)
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

func TestChatLiveViewTruncatesLongOutput(t *testing.T) {
	m := newModel(nil, "", "", "")
	m.width = 80
	m.height = 8
	m.append("assistant", strings.Repeat("line\n", 80))
	view := m.View()
	if !strings.Contains(view, "live output truncated") {
		t.Fatalf("expected truncation marker for long live output:\n%s", view)
	}
	if !strings.Contains(view, "Type message or command") {
		t.Fatalf("expected composer to remain visible after truncating live output:\n%s", view)
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
	if !strings.Contains(got, "… +") {
		t.Fatalf("expected truncated output marker, got: %q", got)
	}
}

func TestToolResultUpdatesToolCellWithoutRawJSON(t *testing.T) {
	m := model{assembler: tuirender.NewAssembler(), mode: modeChat}
	next, _ := m.Update(svcMsg(service.Event{Kind: service.EventToolCall, ToolCallID: "tc-1", ToolName: "read_file", Text: `read_file: {"file_path":"internal/tui/model.go"}`}))
	m = next.(model)
	raw := `{"success":true,"data":{"status":"ok","metrics":{"returned_lines":24,"total_lines":100},"payload":{"file_path":"internal/tui/model.go","content":"package tui"}}}`
	next, _ = m.Update(svcMsg(service.Event{Kind: service.EventToolResult, ToolCallID: "tc-1", ToolName: "read_file", Text: raw}))
	m = next.(model)
	snap := m.assembler.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected one updated tool cell, got %+v", snap)
	}
	if !strings.Contains(snap[0].Text, "Explored") || !strings.Contains(snap[0].Text, "Read internal/tui/model.go") {
		t.Fatalf("expected codex-style read summary, got: %q", snap[0].Text)
	}
	if strings.Contains(snap[0].Text, "payload") || strings.Contains(snap[0].Text, "package tui") {
		t.Fatalf("tool cell must not expose raw json/content: %q", snap[0].Text)
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
	// The assembler should have the header banner added (old content discarded, not committed)
	snap := m2.assembler.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("expected 1 info message (header), got %d: %+v", len(snap), snap)
	}
	if snap[0].Role != "info" {
		t.Fatalf("expected info role, got %q", snap[0].Role)
	}
	if !strings.Contains(snap[0].Text, "▸ Whale") {
		t.Fatalf("expected header banner, got: %q", snap[0].Text)
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
