package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/usewhale/whale/internal/app"

	"github.com/spf13/cobra"
)

func TestRunSetupSavesCredentials(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	in := strings.NewReader("sk-1234567890abcdef1234\n")

	if err := runSetup(&out, in, dir); err != nil {
		t.Fatalf("runSetup: %v", err)
	}

	creds, err := app.LoadCredentials(dir)
	if err != nil {
		t.Fatalf("LoadCredentials: %v", err)
	}
	if creds.DeepSeekAPIKey != "sk-1234567890abcdef1234" {
		t.Fatalf("deepseek_api_key: got %q", creds.DeepSeekAPIKey)
	}
	if !strings.Contains(out.String(), filepath.Join(dir, "credentials.json")) {
		t.Fatalf("expected output to mention credentials path, got %q", out.String())
	}
}

func TestRunSetupRejectsInvalidKey(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	in := strings.NewReader("invalid\n")

	if err := runSetup(&out, in, dir); err == nil {
		t.Fatal("expected invalid key error")
	}
}

func TestRunDoctorReturnsExitErrorOnFailures(t *testing.T) {
	dir := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("DEEPSEEK_BASE_URL", "http://127.0.0.1:1")
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(workspace); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(oldwd) }()

	var out bytes.Buffer
	err = runDoctor(&out, app.Config{DataDir: filepath.Join(dir, ".whale")})
	var exitErr ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 1 {
		t.Fatalf("expected ExitError{1}, got %v", err)
	}
	if !strings.Contains(out.String(), "0 fail") && !strings.Contains(out.String(), "1 fail") {
		t.Fatalf("expected summary output, got %q", out.String())
	}
}

func TestDoctorBadge(t *testing.T) {
	if got := doctorBadge(app.DoctorOK); got != "ok" {
		t.Fatalf("doctorBadge ok = %q", got)
	}
	if got := doctorBadge(app.DoctorWarn); got != "warn" {
		t.Fatalf("doctorBadge warn = %q", got)
	}
	if got := doctorBadge(app.DoctorFail); got != "fail" {
		t.Fatalf("doctorBadge fail = %q", got)
	}
}

func TestReadExecPromptPrefersArg(t *testing.T) {
	got, err := readExecPrompt(strings.NewReader("stdin prompt"), []string{"arg prompt"})
	if err != nil {
		t.Fatalf("readExecPrompt: %v", err)
	}
	if got != "arg prompt" {
		t.Fatalf("prompt = %q", got)
	}
}

func TestReadExecPromptFallsBackToStdin(t *testing.T) {
	got, err := readExecPrompt(strings.NewReader("stdin prompt\n"), nil)
	if err != nil {
		t.Fatalf("readExecPrompt: %v", err)
	}
	if got != "stdin prompt" {
		t.Fatalf("prompt = %q", got)
	}
}

func TestRunExecTextOutput(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "sk-1234567890abcdef1234")
	srv := newExecTestServer(t, "hello from exec")
	defer srv.Close()
	t.Setenv("DEEPSEEK_BASE_URL", srv.URL)

	dir := t.TempDir()
	workspace := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(workspace); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(oldwd) }()

	var out bytes.Buffer
	var errOut bytes.Buffer
	opts := &cliOptions{cfg: app.DefaultConfig()}
	opts.cfg.DataDir = dir
	if err := runExec(&out, &errOut, strings.NewReader(""), opts, []string{"hi"}, false, 0); err != nil {
		t.Fatalf("runExec: %v", err)
	}
	if got := out.String(); got != "hello from exec\n" {
		t.Fatalf("stdout = %q", got)
	}
	if errOut.Len() != 0 {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestRunExecJSONOutput(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "sk-1234567890abcdef1234")
	srv := newExecTestServer(t, "hello json")
	defer srv.Close()
	t.Setenv("DEEPSEEK_BASE_URL", srv.URL)

	dir := t.TempDir()
	workspace := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(workspace); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer func() { _ = os.Chdir(oldwd) }()

	var out bytes.Buffer
	var errOut bytes.Buffer
	opts := &cliOptions{cfg: app.DefaultConfig()}
	opts.cfg.DataDir = dir
	if err := runExec(&out, &errOut, strings.NewReader("stdin prompt"), opts, nil, true, 0); err != nil {
		t.Fatalf("runExec: %v", err)
	}
	var res app.ExecResult
	if err := json.Unmarshal(out.Bytes(), &res); err != nil {
		t.Fatalf("unmarshal json: %v\n%s", err, out.String())
	}
	if res.Status != "completed" || res.Output != "hello json" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.SessionID == "" {
		t.Fatalf("expected session id: %+v", res)
	}
	if errOut.Len() != 0 {
		t.Fatalf("stderr = %q", errOut.String())
	}
}

func TestPrepareCLIConfigMarksExplicitDefaultModel(t *testing.T) {
	opts := &cliOptions{cfg: app.DefaultConfig()}
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("model", opts.cfg.Model, "")
	if err := cmd.Flags().Set("model", "deepseek-v4-flash"); err != nil {
		t.Fatalf("set model: %v", err)
	}
	if err := prepareCLIConfig(cmd, opts); err != nil {
		t.Fatalf("prepareCLIConfig: %v", err)
	}
	if !opts.cfg.ModelExplicit {
		t.Fatal("expected ModelExplicit=true")
	}
}

func newExecTestServer(t *testing.T, content string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\n", content)
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
}
