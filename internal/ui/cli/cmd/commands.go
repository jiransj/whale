package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/usewhale/whale/internal/app"
)

func newExecCmd(opts *cliOptions) *cobra.Command {
	var jsonOutput bool
	var timeoutSec int
	c := &cobra.Command{
		Use:   "exec [prompt]",
		Short: "Run a single prompt non-interactively",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := prepareCLIConfig(cmd, opts); err != nil {
				return err
			}
			return runExec(cmd.OutOrStdout(), cmd.ErrOrStderr(), cmd.InOrStdin(), opts, args, jsonOutput, timeoutSec)
		},
	}
	c.Flags().BoolVar(&jsonOutput, "json", false, "Emit machine-readable JSON output")
	c.Flags().IntVar(&timeoutSec, "timeout-sec", 0, "Optional timeout in seconds for this exec run")
	return c
}

func newResumeCmd(opts *cliOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "resume [id]",
		Short: "Resume a session by id (or open picker when id is omitted)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := prepareCLIConfig(cmd, opts); err != nil {
				return err
			}
			start := app.StartOptions{ResumeMenu: true}
			if len(args) == 1 {
				start.SessionID = args[0]
				start.ResumeMenu = false
			}
			return runLoop(opts, start)
		},
	}
}

func newDoctorCmd(opts *cliOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Run Whale health checks",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(cmd.OutOrStdout(), opts.cfg)
		},
	}
}

func newSetupCmd(opts *cliOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Save your DeepSeek API key for future Whale sessions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetup(cmd.OutOrStdout(), cmd.InOrStdin(), opts.cfg.DataDir)
		},
	}
}

func runSetup(out io.Writer, in io.Reader, dataDir string) error {
	reader := bufio.NewReader(in)
	envKey := strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
	fmt.Fprintln(out, "Whale setup")
	if envKey != "" {
		fmt.Fprintln(out, "DEEPSEEK_API_KEY is set in the current environment.")
		fmt.Fprint(out, "DeepSeek API key (press enter to reuse current env value): ")
	} else {
		fmt.Fprint(out, "DeepSeek API key: ")
	}
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return fmt.Errorf("read api key: %w", err)
	}
	key := strings.TrimSpace(line)
	if key == "" {
		key = envKey
	}
	if err := app.ValidateDeepSeekAPIKey(key); err != nil {
		return err
	}
	if err := app.SaveCredentials(dataDir, app.Credentials{DeepSeekAPIKey: key}); err != nil {
		return err
	}
	fmt.Fprintf(out, "saved DeepSeek API key to %s\n", filepath.Join(dataDir, "credentials.json"))
	fmt.Fprintln(out, "Run `whale` to start a session.")
	return nil
}

func runDoctor(out io.Writer, cfg app.Config) error {
	workspaceRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get workspace: %w", err)
	}
	report, err := app.RunDoctor(context.Background(), cfg, workspaceRoot)
	if err != nil {
		return err
	}

	fmt.Fprintln(out, "whale doctor")
	fmt.Fprintf(out, "  workspace: %s\n", report.Workspace)
	fmt.Fprintf(out, "  data dir: %s\n", report.DataDir)
	fmt.Fprintln(out)
	for _, check := range report.Checks {
		fmt.Fprintf(out, "  %s  %-12s %s\n", doctorBadge(check.Level), check.Label, check.Detail)
	}
	fmt.Fprintln(out)
	ok, warn, fail := report.Summary()
	fmt.Fprintf(out, "%d ok · %d warn · %d fail\n", ok, warn, fail)
	if fail > 0 {
		return ExitError{Code: 1}
	}
	return nil
}

func doctorBadge(level app.DoctorLevel) string {
	switch level {
	case app.DoctorOK:
		return "ok"
	case app.DoctorWarn:
		return "warn"
	default:
		return "fail"
	}
}

func runExec(out io.Writer, errOut io.Writer, in io.Reader, opts *cliOptions, args []string, jsonOutput bool, timeoutSec int) error {
	prompt, err := readExecPrompt(in, args)
	if err != nil {
		return err
	}
	start := app.StartOptions{NewSession: true}
	if strings.TrimSpace(opts.session) != "" {
		start.SessionID = opts.session
	}
	if strings.TrimSpace(opts.mode) != "" {
		start.ModeOverride = opts.mode
	}

	ctx := context.Background()
	if timeoutSec > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
		defer cancel()
	}

	res, execErr := app.RunExec(ctx, opts.cfg, start, prompt)
	if jsonOutput {
		if err := writeExecJSON(out, res); err != nil {
			return err
		}
		if execErr != nil {
			return ExitError{Code: 1}
		}
		return nil
	}
	if txt := res.TextOutput(); txt != "" {
		if _, err := io.WriteString(out, txt); err != nil {
			return err
		}
		if !strings.HasSuffix(txt, "\n") {
			if _, err := io.WriteString(out, "\n"); err != nil {
				return err
			}
		}
	}
	if execErr != nil {
		if strings.TrimSpace(res.Error) != "" {
			if _, err := fmt.Fprintln(errOut, res.Error); err != nil {
				return err
			}
		}
		return ExitError{Code: 1}
	}
	return nil
}

func readExecPrompt(in io.Reader, args []string) (string, error) {
	if len(args) == 1 {
		prompt := strings.TrimSpace(args[0])
		if prompt == "" {
			return "", fmt.Errorf("prompt is empty")
		}
		return prompt, nil
	}
	if f, ok := in.(*os.File); ok {
		if info, err := f.Stat(); err == nil && (info.Mode()&os.ModeCharDevice) != 0 {
			return "", fmt.Errorf("prompt is required")
		}
	}
	b, err := io.ReadAll(in)
	if err != nil {
		return "", fmt.Errorf("read prompt from stdin: %w", err)
	}
	prompt := strings.TrimSpace(string(b))
	if prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}
	return prompt, nil
}

func writeExecJSON(out io.Writer, res app.ExecResult) error {
	if err := res.Validate(); err != nil {
		return err
	}
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(res)
}
