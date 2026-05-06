package compact

import (
	"strings"

	"github.com/usewhale/whale/internal/core"
)

func EstimateMessagesTokens(msgs []core.Message) int {
	total := 0
	for _, m := range msgs {
		total += EstimateTokens(m.Text)
		total += EstimateTokens(m.Reasoning)
		for _, tc := range m.ToolCalls {
			total += EstimateTokens(tc.Name) + EstimateTokens(tc.Input)
		}
		for _, tr := range m.ToolResults {
			total += EstimateTokens(tr.Name) + EstimateTokens(tr.Content)
		}
	}
	return total
}

func EstimateTokens(s string) int {
	if strings.TrimSpace(s) == "" {
		return 0
	}
	// Rough estimator: ASCII runs are compressed, CJK/non-ASCII is near 1 rune/token.
	asciiRunes := 0
	nonASCII := 0
	for _, r := range s {
		if r < 128 {
			asciiRunes++
		} else {
			nonASCII++
		}
	}
	asciiTok := (asciiRunes + 3) / 4
	return asciiTok + nonASCII
}
