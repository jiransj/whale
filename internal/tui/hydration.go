package tui

import (
	"strings"

	"github.com/usewhale/whale/internal/core"
)

func (m *model) hydrateSessionMessages(msgs []core.Message) {
	for _, msg := range msgs {
		switch msg.Role {
		case core.RoleUser:
			if strings.TrimSpace(msg.Text) != "" && !msg.Hidden {
				m.append("you", msg.Text)
			}
		case core.RoleAssistant:
			hasVisibleText := strings.TrimSpace(msg.Text) != "" && !isEnvironmentInventoryBlock(msg.Text)
			if strings.TrimSpace(msg.Reasoning) != "" {
				m.append("think", msg.Reasoning)
			}
			if hasVisibleText {
				if plan, ok := core.ExtractProposedPlanText(msg.Text); ok {
					normal := strings.TrimSpace(core.StripProposedPlanBlocks(msg.Text))
					if normal != "" {
						m.append("assistant", normal)
					}
					m.assembler.AddPlan(plan)
				} else {
					m.append("assistant", msg.Text)
				}
			} else if isEnvironmentInventoryBlock(msg.Text) {
				m.addLog(logEntry{
					Kind:    "env_summary",
					Source:  "assistant",
					Summary: "environment summary captured",
					Raw:     msg.Text,
				})
			}
			for _, tc := range msg.ToolCalls {
				m.appendToolCall(tc.ID, tc.Name, summarizeHydratedToolCall(tc))
			}
		case core.RoleTool:
			for _, tr := range msg.ToolResults {
				body := strings.TrimSpace(tr.Content)
				if body == "" {
					continue
				}
				role, text := summarizeToolResultForChat(tr.Name, body)
				if !m.updateToolCallFromResult(tr.ToolCallID, tr.Name, tr.Content, role, text, tr.Metadata) {
					m.assembler.AddToolResultWithRole("", text, role)
				}
				m.captureDiffMetadata(tr.Name, tr.Metadata)
			}
		}
	}
}
