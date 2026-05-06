package agent

import (
	"strings"

	"github.com/usewhale/whale/internal/core"
	"github.com/usewhale/whale/internal/session"
)

func (a *Agent) emitAssistantContentDelta(delta string, parser *core.ProposedPlanParser, planText *strings.Builder, planStarted, planCompleted *bool, events chan<- AgentEvent) {
	if a.mode != session.ModePlan {
		events <- AgentEvent{Type: AgentEventTypeAssistantDelta, Content: delta}
		return
	}
	for _, seg := range parser.Parse(delta) {
		a.emitProposedPlanSegment(seg, planText, planStarted, planCompleted, events)
	}
}

func (a *Agent) emitProposedPlanSegment(seg core.ProposedPlanSegment, planText *strings.Builder, planStarted, planCompleted *bool, events chan<- AgentEvent) {
	switch seg.Kind {
	case core.ProposedPlanSegmentNormal:
		if seg.Text != "" {
			events <- AgentEvent{Type: AgentEventTypeAssistantDelta, Content: seg.Text}
		}
	case core.ProposedPlanSegmentStart:
		*planStarted = true
		*planCompleted = false
		planText.Reset()
	case core.ProposedPlanSegmentDelta:
		if *planStarted && seg.Text != "" {
			planText.WriteString(seg.Text)
			events <- AgentEvent{Type: AgentEventTypePlanDelta, Content: seg.Text}
		}
	case core.ProposedPlanSegmentEnd:
		if *planStarted && !*planCompleted {
			*planCompleted = true
			events <- AgentEvent{Type: AgentEventTypePlanCompleted, Content: planText.String()}
		}
	}
}

func (a *Agent) emitFinalProposedPlan(text string, planText *strings.Builder, planStarted, planCompleted *bool, events chan<- AgentEvent) {
	plan, ok := core.ExtractProposedPlanText(text)
	if !ok {
		return
	}
	*planStarted = true
	*planCompleted = true
	planText.Reset()
	planText.WriteString(plan)
	if plan != "" {
		events <- AgentEvent{Type: AgentEventTypePlanDelta, Content: plan}
	}
	events <- AgentEvent{Type: AgentEventTypePlanCompleted, Content: plan}
}
