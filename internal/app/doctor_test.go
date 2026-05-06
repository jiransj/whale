package app

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunDoctorReportsHealthyWorkspace(t *testing.T) {
	dataDir := t.TempDir()
	workspace := t.TempDir()
	if err := SaveCredentials(dataDir, Credentials{DeepSeekAPIKey: "sk-1234567890abcdef1234"}); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}
	thinking := true
	if err := SavePreferences(dataDir, Preferences{Model: "deepseek-v4-flash", ThinkingEnabled: &thinking}); err != nil {
		t.Fatalf("SavePreferences: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "AGENTS.md"), []byte("hello"), 0o600); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, ".whale"), 0o755); err != nil {
		t.Fatalf("mkdir .whale: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".whale", "settings.json"), []byte(`{"hooks":{"PreToolUse":[{"command":"echo ok"}]}}`), 0o600); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}

	t.Setenv("DEEPSEEK_BASE_URL", newDoctorServer(t, http.StatusOK).URL)
	report, err := RunDoctor(context.Background(), Config{DataDir: dataDir, MemoryFileOrder: "AGENTS.md"}, workspace)
	if err != nil {
		t.Fatalf("RunDoctor: %v", err)
	}
	if report.HasFailures() {
		t.Fatalf("expected no failures: %+v", report.Checks)
	}
	if got := findDoctorCheck(report.Checks, "api key"); got.Level != DoctorOK {
		t.Fatalf("api key level: %+v", got)
	}
	if got := findDoctorCheck(report.Checks, "api reach"); got.Level != DoctorOK {
		t.Fatalf("api reach level: %+v", got)
	}
	if got := findDoctorCheck(report.Checks, "memory"); got.Level != DoctorOK {
		t.Fatalf("memory level: %+v", got)
	}
	if got := findDoctorCheck(report.Checks, "hooks"); got.Level != DoctorOK {
		t.Fatalf("hooks level: %+v", got)
	}
}

func TestRunDoctorFlagsBrokenFiles(t *testing.T) {
	dataDir := t.TempDir()
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "credentials.json"), []byte("{"), 0o600); err != nil {
		t.Fatalf("write credentials: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "preferences.json"), []byte("{"), 0o600); err != nil {
		t.Fatalf("write preferences: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, ".whale"), 0o755); err != nil {
		t.Fatalf("mkdir .whale: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, ".whale", "settings.json"), []byte("{"), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	report, err := RunDoctor(context.Background(), Config{DataDir: dataDir, MemoryFileOrder: "AGENTS.md"}, workspace)
	if err != nil {
		t.Fatalf("RunDoctor: %v", err)
	}
	if got := findDoctorCheck(report.Checks, "credentials"); got.Level != DoctorFail {
		t.Fatalf("credentials level: %+v", got)
	}
	if got := findDoctorCheck(report.Checks, "preferences"); got.Level != DoctorFail {
		t.Fatalf("preferences level: %+v", got)
	}
	if got := findDoctorCheck(report.Checks, "hooks"); got.Level != DoctorWarn {
		t.Fatalf("hooks level: %+v", got)
	}
	if got := findDoctorCheck(report.Checks, "memory"); got.Level != DoctorWarn {
		t.Fatalf("memory level: %+v", got)
	}
}

func TestRunDoctorUsesEnvKeyWhenPresent(t *testing.T) {
	dataDir := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("DEEPSEEK_API_KEY", "sk-env1234567890abcdef")
	report, err := RunDoctor(context.Background(), Config{DataDir: dataDir}, workspace)
	if err != nil {
		t.Fatalf("RunDoctor: %v", err)
	}
	got := findDoctorCheck(report.Checks, "api key")
	if got.Level != DoctorOK || !strings.Contains(got.Detail, "env DEEPSEEK_API_KEY") {
		t.Fatalf("api key check: %+v", got)
	}
}

func TestCheckDeepSeekAPIReachabilityClassifiesResponses(t *testing.T) {
	t.Setenv("DEEPSEEK_BASE_URL", newDoctorServer(t, http.StatusUnauthorized).URL)
	msg, err := CheckDeepSeekAPIReachability(context.Background(), "sk-1234567890abcdef1234")
	if err == nil || !strings.Contains(msg, "unauthorized") {
		t.Fatalf("want unauthorized, got msg=%q err=%v", msg, err)
	}

	t.Setenv("DEEPSEEK_BASE_URL", newDoctorServer(t, http.StatusForbidden).URL)
	msg, err = CheckDeepSeekAPIReachability(context.Background(), "sk-1234567890abcdef1234")
	if err == nil || !strings.Contains(msg, "forbidden") {
		t.Fatalf("want forbidden, got msg=%q err=%v", msg, err)
	}
}

func TestClassifyDoctorHTTPError(t *testing.T) {
	msg := classifyDoctorHTTPError(&net.DNSError{Err: "no such host", Name: "api.deepseek.com"})
	if !strings.Contains(msg, "DNS resolution failed") {
		t.Fatalf("dns msg: %q", msg)
	}
	msg = classifyDoctorHTTPError(context.DeadlineExceeded)
	if !strings.Contains(msg, "timeout") {
		t.Fatalf("timeout msg: %q", msg)
	}
	msg = classifyDoctorHTTPError(errors.New("net/http: TLS handshake timeout"))
	if !strings.Contains(msg, "TLS handshake timed out") {
		t.Fatalf("tls msg: %q", msg)
	}
}

func newDoctorServer(t *testing.T, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{}`))
	}))
}

func findDoctorCheck(checks []DoctorCheck, label string) DoctorCheck {
	for _, check := range checks {
		if check.Label == label {
			return check
		}
	}
	return DoctorCheck{}
}
