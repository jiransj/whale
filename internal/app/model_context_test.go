package app

import "testing"

func TestInferredContextWindowForModel(t *testing.T) {
	tests := []struct {
		model string
		want  int
	}{
		{model: "deepseek-v4-flash", want: 1_000_000},
		{model: "deepseek-v4-pro", want: 1_000_000},
		{model: "deepseek-chat", want: 128_000},
		{model: "unknown-model", want: 128_000},
		{model: "", want: 128_000},
	}
	for _, tt := range tests {
		if got := inferredContextWindowForModel(tt.model); got != tt.want {
			t.Fatalf("inferredContextWindowForModel(%q) = %d, want %d", tt.model, got, tt.want)
		}
	}
}

func TestResolveContextWindow(t *testing.T) {
	if got := resolveContextWindow(128_000, "deepseek-v4-pro"); got != 1_000_000 {
		t.Fatalf("resolveContextWindow(default, v4-pro) = %d, want 1000000", got)
	}
	if got := resolveContextWindow(256_000, "deepseek-v4-pro"); got != 256_000 {
		t.Fatalf("resolveContextWindow(explicit override) = %d, want 256000", got)
	}
}
