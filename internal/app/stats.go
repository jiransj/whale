package app

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/usewhale/whale/internal/telemetry"
)

const statsRecentLimit = 5

type usageStats struct {
	Turns            int
	Sessions         map[string]bool
	PromptTokens     int
	CompletionTokens int
	CacheHit         int
	CacheMiss        int
	CostUSD          float64
	Last7CostUSD     float64
	ByModel          map[string]*usageModelStats
	Recent           []telemetry.UsageRecord
}

type usageModelStats struct {
	Model            string
	Turns            int
	Tokens           int
	CostUSD          float64
	CacheHit         int
	CacheMiss        int
	PromptTokens     int
	CompletionTokens int
}

type toolInputStats struct {
	Repaired     int
	Invalid      int
	ByRepairKind map[string]int
	ByTool       map[string]*toolInputToolStats
	ByModel      map[string]*toolInputModelStats
	ByErrorCode  map[string]int
	Recent       []telemetry.ToolInputEvent
}

type toolInputToolStats struct {
	Tool     string
	Repaired int
	Invalid  int
}

type toolInputModelStats struct {
	Model    string
	Repaired int
	Invalid  int
}

func (a *App) buildStats() string {
	return a.buildStatsViewAt("overview", time.Now())
}

func (a *App) buildStatsView(view string) string {
	return a.buildStatsViewAt(view, time.Now())
}

func (a *App) buildStatsViewAt(view string, now time.Time) string {
	usage := readUsageStats(filepath.Join(a.cfg.DataDir, "usage.jsonl"), now)
	toolInput := readToolInputStats(a.sessionsDir)

	var lines []string
	switch view {
	case "usage":
		lines = []string{"Stats", "", "Usage"}
		lines = append(lines, formatUsageStats(usage)...)
	case "tools", "repair":
		lines = []string{"Stats", "", "Tool input"}
		lines = append(lines, formatToolInputStats(toolInput)...)
	case "recent":
		lines = []string{"Stats"}
		lines = append(lines, formatRecentStats(usage, toolInput)...)
	case "all":
		lines = []string{"Stats", "", "Usage"}
		lines = append(lines, formatUsageStats(usage)...)
		lines = append(lines, "", "Tool input")
		lines = append(lines, formatToolInputStats(toolInput)...)
		lines = append(lines, formatRecentStats(usage, toolInput)...)
	default:
		lines = []string{"Stats"}
		lines = append(lines, formatStatsOverview(usage, toolInput)...)
	}
	return strings.Join(lines, "\n")
}

func readUsageStats(path string, now time.Time) usageStats {
	stats := usageStats{
		Sessions: map[string]bool{},
		ByModel:  map[string]*usageModelStats{},
	}
	f, err := os.Open(path)
	if err != nil {
		return stats
	}
	defer f.Close()

	cutoff := now.Add(-7 * 24 * time.Hour).UnixMilli()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var rec telemetry.UsageRecord
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue
		}
		stats.Turns++
		if rec.Session != "" {
			stats.Sessions[rec.Session] = true
		}
		stats.PromptTokens += rec.PromptTokens
		stats.CompletionTokens += rec.CompletionTokens
		stats.CacheHit += rec.PromptCacheHit
		stats.CacheMiss += rec.PromptCacheMiss
		stats.CostUSD += rec.CostUSD
		if rec.TS >= cutoff {
			stats.Last7CostUSD += rec.CostUSD
		}
		model := strings.TrimSpace(rec.Model)
		if model == "" {
			model = "(unknown)"
		}
		ms := stats.ByModel[model]
		if ms == nil {
			ms = &usageModelStats{Model: model}
			stats.ByModel[model] = ms
		}
		ms.Turns++
		ms.PromptTokens += rec.PromptTokens
		ms.CompletionTokens += rec.CompletionTokens
		ms.Tokens += rec.PromptTokens + rec.CompletionTokens
		ms.CostUSD += rec.CostUSD
		ms.CacheHit += rec.PromptCacheHit
		ms.CacheMiss += rec.PromptCacheMiss
		stats.Recent = appendRecentUsage(stats.Recent, rec)
	}
	return stats
}

func readToolInputStats(sessionsDir string) toolInputStats {
	stats := toolInputStats{
		ByRepairKind: map[string]int{},
		ByTool:       map[string]*toolInputToolStats{},
		ByModel:      map[string]*toolInputModelStats{},
		ByErrorCode:  map[string]int{},
	}
	entries, err := os.ReadDir(strings.TrimSpace(sessionsDir))
	if err != nil {
		return stats
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), telemetry.ToolInputEventsSuffix) {
			continue
		}
		readToolInputEventFile(filepath.Join(sessionsDir, entry.Name()), &stats)
	}
	return stats
}

func readToolInputEventFile(path string, stats *toolInputStats) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var rec telemetry.ToolInputEvent
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue
		}
		switch rec.Event {
		case "tool_input_repaired":
			stats.Repaired++
			if rec.RepairKind != "" {
				stats.ByRepairKind[rec.RepairKind]++
			}
			updateToolInputToolStats(stats, rec.Tool, true)
			updateToolInputModelStats(stats, rec.Model, true)
		case "tool_input_invalid":
			stats.Invalid++
			if rec.ErrorCode != "" {
				stats.ByErrorCode[rec.ErrorCode]++
			}
			updateToolInputToolStats(stats, rec.Tool, false)
			updateToolInputModelStats(stats, rec.Model, false)
		default:
			continue
		}
		stats.Recent = appendRecentToolInput(stats.Recent, rec)
	}
}

func updateToolInputToolStats(stats *toolInputStats, tool string, repaired bool) {
	tool = nonEmpty(tool, "(unknown)")
	ts := stats.ByTool[tool]
	if ts == nil {
		ts = &toolInputToolStats{Tool: tool}
		stats.ByTool[tool] = ts
	}
	if repaired {
		ts.Repaired++
	} else {
		ts.Invalid++
	}
}

func updateToolInputModelStats(stats *toolInputStats, model string, repaired bool) {
	model = nonEmpty(model, "(unknown)")
	ms := stats.ByModel[model]
	if ms == nil {
		ms = &toolInputModelStats{Model: model}
		stats.ByModel[model] = ms
	}
	if repaired {
		ms.Repaired++
	} else {
		ms.Invalid++
	}
}

func formatUsageStats(stats usageStats) []string {
	totalTokens := stats.PromptTokens + stats.CompletionTokens
	lines := []string{
		fmt.Sprintf("- turns: %d", stats.Turns),
		fmt.Sprintf("- sessions: %d", len(stats.Sessions)),
		fmt.Sprintf("- tokens: %s total · %s input · %s output", formatCount(totalTokens), formatCount(stats.PromptTokens), formatCount(stats.CompletionTokens)),
		fmt.Sprintf("- cache: %s hit · %s miss · %.1f%%", formatCount(stats.CacheHit), formatCount(stats.CacheMiss), ratioPercent(stats.CacheHit, stats.CacheHit+stats.CacheMiss)),
		fmt.Sprintf("- estimated cost: $%.4f total · $%.4f last 7d", stats.CostUSD, stats.Last7CostUSD),
	}
	if len(stats.ByModel) > 0 {
		lines = append(lines, "", "By model")
		for _, ms := range topUsageModels(stats.ByModel, statsRecentLimit) {
			lines = append(lines, fmt.Sprintf("- %s: %d turns · %s tokens · %.1f%% cache · $%.4f", ms.Model, ms.Turns, formatCount(ms.Tokens), ratioPercent(ms.CacheHit, ms.CacheHit+ms.CacheMiss), ms.CostUSD))
		}
	}
	return lines
}

func formatToolInputStats(stats toolInputStats) []string {
	total := stats.Repaired + stats.Invalid
	lines := []string{
		fmt.Sprintf("- repaired: %d", stats.Repaired),
		fmt.Sprintf("- invalid: %d", stats.Invalid),
		fmt.Sprintf("- repair rate: %.1f%%", ratioPercent(stats.Repaired, total)),
	}
	if len(stats.ByRepairKind) > 0 {
		lines = append(lines, "", "Repair kinds")
		for _, kv := range topCounts(stats.ByRepairKind, statsRecentLimit) {
			lines = append(lines, fmt.Sprintf("- %s: %d", kv.Key, kv.Value))
		}
	}
	if len(stats.ByErrorCode) > 0 {
		lines = append(lines, "", "Invalid codes")
		for _, kv := range topCounts(stats.ByErrorCode, statsRecentLimit) {
			lines = append(lines, fmt.Sprintf("- %s: %d", kv.Key, kv.Value))
		}
	}
	if len(stats.ByTool) > 0 {
		lines = append(lines, "", "Top tools")
		for _, ts := range topToolInputTools(stats.ByTool, statsRecentLimit) {
			lines = append(lines, fmt.Sprintf("- %s: %d repaired · %d invalid", ts.Tool, ts.Repaired, ts.Invalid))
		}
	}
	if len(stats.ByModel) > 0 {
		lines = append(lines, "", "By model")
		for _, ms := range topToolInputModels(stats.ByModel, statsRecentLimit) {
			lines = append(lines, fmt.Sprintf("- %s: %d repaired · %d invalid", ms.Model, ms.Repaired, ms.Invalid))
		}
	}
	return lines
}

func formatStatsOverview(usage usageStats, toolInput toolInputStats) []string {
	totalTokens := usage.PromptTokens + usage.CompletionTokens
	lines := []string{
		"",
		"Usage",
		fmt.Sprintf("- turns: %d", usage.Turns),
		fmt.Sprintf("- tokens: %s total", formatCount(totalTokens)),
		fmt.Sprintf("- estimated cost: $%.4f total · $%.4f last 7d", usage.CostUSD, usage.Last7CostUSD),
	}
	if model := topUsageModel(usage.ByModel); model != nil {
		lines = append(lines, fmt.Sprintf("- top model: %s · %d turns · $%.4f", model.Model, model.Turns, model.CostUSD))
	}

	totalToolInput := toolInput.Repaired + toolInput.Invalid
	lines = append(lines,
		"",
		"Tool input",
		fmt.Sprintf("- repaired: %d", toolInput.Repaired),
		fmt.Sprintf("- invalid: %d", toolInput.Invalid),
		fmt.Sprintf("- repair rate: %.1f%%", ratioPercent(toolInput.Repaired, totalToolInput)),
	)
	if repair := topCount(toolInput.ByRepairKind); repair != nil {
		lines = append(lines, fmt.Sprintf("- top repair: %s · %d", repair.Key, repair.Value))
	}
	if tool := topInvalidTool(toolInput.ByTool); tool != nil {
		lines = append(lines, fmt.Sprintf("- top invalid tool: %s · %d", tool.Tool, tool.Invalid))
	}
	lines = append(lines, "", "More: /stats usage, /stats tools, /stats recent, /stats all")
	return lines
}

func formatRecentStats(usage usageStats, toolInput toolInputStats) []string {
	lines := []string{}
	if len(usage.Recent) > 0 {
		lines = append(lines, "", "Recent turns")
		for _, rec := range reverseUsage(usage.Recent) {
			lines = append(lines, fmt.Sprintf("- %s · %s · %s · %s tokens · $%.4f · %.1f%% cache", formatTS(rec.TS), nonEmpty(rec.Session, "(unknown)"), nonEmpty(rec.Model, "(unknown)"), formatCount(rec.PromptTokens+rec.CompletionTokens), rec.CostUSD, ratioPercent(rec.PromptCacheHit, rec.PromptCacheHit+rec.PromptCacheMiss)))
		}
	}
	if len(toolInput.Recent) > 0 {
		lines = append(lines, "", "Recent tool-input events")
		for _, rec := range reverseToolInput(toolInput.Recent) {
			detail := nonEmpty(rec.RepairKind, rec.ErrorCode)
			if rec.Path != "" {
				detail += " · " + rec.Path
			}
			lines = append(lines, fmt.Sprintf("- %s · %s · %s · %s · %s", formatTS(rec.TS), nonEmpty(rec.Model, "(unknown)"), nonEmpty(rec.Tool, "(unknown)"), eventDisplay(rec.Event), detail))
		}
	}
	if len(lines) == 0 {
		return []string{"", "Recent", "- no recent stats"}
	}
	return lines
}

type countKV struct {
	Key   string
	Value int
}

func topCounts(in map[string]int, limit int) []countKV {
	out := make([]countKV, 0, len(in))
	for k, v := range in {
		out = append(out, countKV{Key: k, Value: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Value == out[j].Value {
			return out[i].Key < out[j].Key
		}
		return out[i].Value > out[j].Value
	})
	return limitSlice(out, limit)
}

func topCount(in map[string]int) *countKV {
	top := topCounts(in, 1)
	if len(top) == 0 {
		return nil
	}
	return &top[0]
}

func topUsageModels(in map[string]*usageModelStats, limit int) []*usageModelStats {
	out := make([]*usageModelStats, 0, len(in))
	for _, v := range in {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CostUSD == out[j].CostUSD {
			return out[i].Model < out[j].Model
		}
		return out[i].CostUSD > out[j].CostUSD
	})
	return limitSlice(out, limit)
}

func topUsageModel(in map[string]*usageModelStats) *usageModelStats {
	top := topUsageModels(in, 1)
	if len(top) == 0 {
		return nil
	}
	return top[0]
}

func topToolInputTools(in map[string]*toolInputToolStats, limit int) []*toolInputToolStats {
	out := make([]*toolInputToolStats, 0, len(in))
	for _, v := range in {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		left := out[i].Repaired + out[i].Invalid
		right := out[j].Repaired + out[j].Invalid
		if left == right {
			return out[i].Tool < out[j].Tool
		}
		return left > right
	})
	return limitSlice(out, limit)
}

func topInvalidTool(in map[string]*toolInputToolStats) *toolInputToolStats {
	out := make([]*toolInputToolStats, 0, len(in))
	for _, v := range in {
		if v.Invalid > 0 {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Invalid == out[j].Invalid {
			return out[i].Tool < out[j].Tool
		}
		return out[i].Invalid > out[j].Invalid
	})
	if len(out) == 0 {
		return nil
	}
	return out[0]
}

func topToolInputModels(in map[string]*toolInputModelStats, limit int) []*toolInputModelStats {
	out := make([]*toolInputModelStats, 0, len(in))
	for _, v := range in {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		left := out[i].Repaired + out[i].Invalid
		right := out[j].Repaired + out[j].Invalid
		if left == right {
			return out[i].Model < out[j].Model
		}
		return left > right
	})
	return limitSlice(out, limit)
}

func appendRecentUsage(recent []telemetry.UsageRecord, rec telemetry.UsageRecord) []telemetry.UsageRecord {
	recent = append(recent, rec)
	sort.Slice(recent, func(i, j int) bool { return recent[i].TS > recent[j].TS })
	return limitSlice(recent, statsRecentLimit)
}

func appendRecentToolInput(recent []telemetry.ToolInputEvent, rec telemetry.ToolInputEvent) []telemetry.ToolInputEvent {
	recent = append(recent, rec)
	sort.Slice(recent, func(i, j int) bool { return recent[i].TS > recent[j].TS })
	return limitSlice(recent, statsRecentLimit)
}

func reverseUsage(in []telemetry.UsageRecord) []telemetry.UsageRecord {
	out := append([]telemetry.UsageRecord(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].TS > out[j].TS })
	return out
}

func reverseToolInput(in []telemetry.ToolInputEvent) []telemetry.ToolInputEvent {
	out := append([]telemetry.ToolInputEvent(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].TS > out[j].TS })
	return out
}

func limitSlice[T any](in []T, limit int) []T {
	if limit <= 0 || len(in) <= limit {
		return in
	}
	return in[:limit]
}

func ratioPercent(num, denom int) float64 {
	if denom <= 0 || num <= 0 {
		return 0
	}
	return float64(num) * 100 / float64(denom)
}

func formatCount(v int) string {
	switch {
	case v >= 1_000_000:
		return trimFloat(float64(v)/1_000_000) + "M"
	case v >= 1_000:
		return trimFloat(float64(v)/1_000) + "K"
	default:
		return fmt.Sprintf("%d", v)
	}
}

func trimFloat(v float64) string {
	s := fmt.Sprintf("%.1f", v)
	return strings.TrimSuffix(s, ".0")
}

func formatTS(ts int64) string {
	if ts <= 0 {
		return "(unknown time)"
	}
	return time.UnixMilli(ts).Format("2006-01-02 15:04")
}

func nonEmpty(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return strings.TrimSpace(v)
}

func eventDisplay(event string) string {
	switch event {
	case "tool_input_repaired":
		return "repaired"
	case "tool_input_invalid":
		return "invalid"
	default:
		return nonEmpty(event, "event")
	}
}
