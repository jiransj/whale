package main

import (
	"strings"
	"testing"
)

func TestUsageTotalsCacheHitRatioUsesHitPlusMiss(t *testing.T) {
	u := usageTotals{CacheHitTokens: 80, CacheMissTokens: 20}
	if got := u.CacheHitRatio(); got != 0.8 {
		t.Fatalf("cache ratio = %v, want 0.8", got)
	}
}

func TestRenderMarkdownAggregatesWeightedCacheRatio(t *testing.T) {
	report := benchReport{
		Meta: benchMeta{Date: "2026-05-07T00:00:00Z", Model: "deepseek-v4-flash", Effort: "high", TaskCount: 2, RepeatsPerTask: 1, WhaleVersion: "test", LiveDeepSeek: true},
		Results: []runResult{
			{TaskID: "a", Repeat: 1, Pass: true, Turns: 1, ToolCalls: 1, CacheHitTokens: 90, CacheMissTokens: 10, CacheHitRatio: 0.9, CostUSD: 0.1},
			{TaskID: "b", Repeat: 1, Pass: false, Turns: 3, ToolCalls: 5, CacheHitTokens: 10, CacheMissTokens: 90, CacheHitRatio: 0.1, CostUSD: 0.3},
		},
	}
	md := renderMarkdown(report)
	for _, want := range []string{"| runs | 2 |", "| pass rate | 50% |", "| cache hit | 50.0% |", "live DeepSeek API usage"} {
		if !strings.Contains(md, want) {
			t.Fatalf("report missing %q:\n%s", want, md)
		}
	}
}

func TestTaskSetupsCreateExpectedFixtures(t *testing.T) {
	for _, task := range tasks {
		root := t.TempDir()
		if err := task.Setup(root); err != nil {
			t.Fatalf("%s setup failed: %v", task.ID, err)
		}
	}
}
