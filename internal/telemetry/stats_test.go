package telemetry

import (
	"testing"

	"github.com/usewhale/whale/internal/llm"
)

func TestEstimateTurnUSD_FlashVsPro(t *testing.T) {
	u := llm.Usage{
		PromptTokens:         1_000_000,
		PromptCacheHitTokens: 100_000,
		CompletionTokens:     100_000,
	}
	flash := EstimateTurnUSD("deepseek-v4-flash", u)
	pro := EstimateTurnUSD("deepseek-v4-pro", u)
	if flash <= 0 {
		t.Fatalf("expected flash cost > 0, got %f", flash)
	}
	if pro <= flash {
		t.Fatalf("expected pro cost > flash cost, pro=%f flash=%f", pro, flash)
	}
}

func TestBuildTurnStats_ReasoningReplayAndCacheRatio(t *testing.T) {
	u := llm.Usage{
		PromptCacheHitTokens:  300,
		PromptCacheMissTokens: 100,
		ReasoningReplayTokens: 42,
	}
	st := BuildTurnStats(3, "deepseek-v4-flash", u)
	if st.Turn != 3 {
		t.Fatalf("unexpected turn: %d", st.Turn)
	}
	if st.ReasoningReplayTok != 42 {
		t.Fatalf("unexpected replay tokens: %d", st.ReasoningReplayTok)
	}
	if st.CacheHitRatio != 0.75 {
		t.Fatalf("unexpected cache hit ratio: %f", st.CacheHitRatio)
	}
}
