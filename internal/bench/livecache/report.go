package main

import (
	"fmt"
	"sort"
	"strings"
)

func renderMarkdown(report benchReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Whale live prefix-cache benchmark\n\n")
	fmt.Fprintf(&b, "**Date:** %s\n", report.Meta.Date)
	fmt.Fprintf(&b, "**Model:** `%s`\n", report.Meta.Model)
	fmt.Fprintf(&b, "**Effort:** `%s`\n", report.Meta.Effort)
	fmt.Fprintf(&b, "**Tasks:** %d, repeats x %d\n", report.Meta.TaskCount, report.Meta.RepeatsPerTask)
	fmt.Fprintf(&b, "**Whale version:** `%s`\n", report.Meta.WhaleVersion)
	fmt.Fprintf(&b, "**Source:** live DeepSeek API usage, not mock usage\n\n")
	fmt.Fprintf(&b, "## Summary\n\n")
	fmt.Fprintf(&b, "| metric | whale |\n")
	fmt.Fprintf(&b, "|---|---:|\n")
	fmt.Fprintf(&b, "| runs | %d |\n", len(report.Results))
	fmt.Fprintf(&b, "| pass rate | %s |\n", pct(passCount(report.Results), len(report.Results)))
	fmt.Fprintf(&b, "| cache hit | %s |\n", pctFloat(aggregateUsage(report.Results).CacheHitRatio()))
	fmt.Fprintf(&b, "| mean cost / task | $%.6f |\n", meanCost(report.Results))
	fmt.Fprintf(&b, "| mean turns | %.1f |\n", meanInt(report.Results, func(r runResult) int { return r.Turns }))
	fmt.Fprintf(&b, "| mean tool calls | %.1f |\n", meanInt(report.Results, func(r runResult) int { return r.ToolCalls }))
	fmt.Fprintf(&b, "\n## Per-task breakdown\n\n")
	fmt.Fprintf(&b, "| task | repeat | pass | turns | tools | cache | cost |\n")
	fmt.Fprintf(&b, "|---|---:|:---:|---:|---:|---:|---:|\n")
	rows := append([]runResult(nil), report.Results...)
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].TaskID == rows[j].TaskID {
			return rows[i].Repeat < rows[j].Repeat
		}
		return rows[i].TaskID < rows[j].TaskID
	})
	for _, r := range rows {
		pass := "no"
		if r.Pass {
			pass = "yes"
		}
		fmt.Fprintf(&b, "| %s | %d | %s | %d | %d | %s | $%.6f |\n",
			r.TaskID, r.Repeat, pass, r.Turns, r.ToolCalls, pctFloat(r.CacheHitRatio), r.CostUSD)
	}
	fmt.Fprintf(&b, "\n## Reproduce\n\n")
	fmt.Fprintf(&b, "```bash\nDEEPSEEK_API_KEY=sk-... scripts/bench/live_cache.sh --repeats %d --model %s\n```\n", report.Meta.RepeatsPerTask, report.Meta.Model)
	return b.String()
}

func aggregateUsage(results []runResult) usageTotals {
	var out usageTotals
	for _, r := range results {
		out.PromptTokens += r.PromptTokens
		out.CompletionTokens += r.CompletionTokens
		out.CacheHitTokens += r.CacheHitTokens
		out.CacheMissTokens += r.CacheMissTokens
		out.CostUSD += r.CostUSD
	}
	return out
}

func passCount(results []runResult) int {
	n := 0
	for _, r := range results {
		if r.Pass {
			n++
		}
	}
	return n
}

func meanCost(results []runResult) float64 {
	if len(results) == 0 {
		return 0
	}
	var sum float64
	for _, r := range results {
		sum += r.CostUSD
	}
	return sum / float64(len(results))
}

func meanInt(results []runResult, fn func(runResult) int) float64 {
	if len(results) == 0 {
		return 0
	}
	sum := 0
	for _, r := range results {
		sum += fn(r)
	}
	return float64(sum) / float64(len(results))
}

func pct(num, denom int) string {
	if denom == 0 {
		return "-"
	}
	return fmt.Sprintf("%.0f%%", (float64(num)/float64(denom))*100)
}

func pctFloat(v float64) string {
	return fmt.Sprintf("%.1f%%", v*100)
}
