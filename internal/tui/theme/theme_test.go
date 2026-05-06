package theme

import "testing"

func TestRoleBorderColors(t *testing.T) {
	cases := map[string]string{
		"you":            "63",
		"assistant":      "39",
		"plan":           "45",
		"tool":           "220",
		"result_ok":      "42",
		"result_failed":  "203",
		"result_running": "117",
		"error":          "203",
		"unknown":        "240",
	}

	for role, want := range cases {
		if got := string(RoleBorder(role)); got != want {
			t.Fatalf("role %q: want %s, got %s", role, want, got)
		}
	}
}

func TestDefaultSemanticColors(t *testing.T) {
	if string(Default.Accent) != "63" {
		t.Fatalf("accent: want 63, got %s", Default.Accent)
	}
	if string(Default.Muted) != "245" {
		t.Fatalf("muted: want 245, got %s", Default.Muted)
	}
	if string(Default.Border) != "240" {
		t.Fatalf("border: want 240, got %s", Default.Border)
	}
	if string(Default.Info) != "111" {
		t.Fatalf("info: want 111, got %s", Default.Info)
	}
	if string(Default.Success) != "42" {
		t.Fatalf("success: want 42, got %s", Default.Success)
	}
	if string(Default.Warn) != "220" {
		t.Fatalf("warn: want 220, got %s", Default.Warn)
	}
	if string(Default.Error) != "203" {
		t.Fatalf("error: want 203, got %s", Default.Error)
	}
}
