package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/usewhale/whale/internal/app"
	"github.com/usewhale/whale/internal/build"
	"github.com/usewhale/whale/internal/defaults"
	"github.com/usewhale/whale/internal/tui"
)

type cliOptions struct {
	cfg app.Config
}

func Execute() error {
	opts := &cliOptions{cfg: app.DefaultConfig()}
	root := newRootCmd(opts)
	return root.Execute()
}

func bindPersistentFlags(c *cobra.Command, opts *cliOptions) {
	c.PersistentFlags().StringVarP(&opts.cfg.Model, "model", "m", opts.cfg.Model, "Model to use ("+strings.Join(defaults.SupportedModels(), "|")+")")
	c.PersistentFlags().BoolVar(&opts.cfg.ThinkingEnabled, "thinking", opts.cfg.ThinkingEnabled, "Override thinking for this run only")
	c.PersistentFlags().StringVar(&opts.cfg.ReasoningEffort, "effort", opts.cfg.ReasoningEffort, "Override reasoning effort for this run only (high|max)")
	c.Flags().BoolP("version", "V", false, "Print version")
}

func runLoop(opts *cliOptions, start app.StartOptions) error {
	return tui.Run(opts.cfg, start)
}

func prepareCLIConfig(cmd *cobra.Command, opts *cliOptions) error {
	flagCfg := opts.cfg
	workspaceRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get workspace: %w", err)
	}
	cfg, err := app.LoadAndApplyConfig(flagCfg, workspaceRoot)
	if err != nil {
		return err
	}
	if flagChanged(cmd, "model") {
		cfg.Model = flagCfg.Model
		cfg.ModelExplicit = true
	}
	opts.cfg = cfg
	return validateModel(opts.cfg.Model)
}

func flagChanged(cmd *cobra.Command, name string) bool {
	f := cmd.Flag(name)
	return f != nil && f.Changed
}

func validateModel(v string) error {
	if !defaults.IsSupportedModel(v) {
		return fmt.Errorf("unsupported model: %s", v)
	}
	return nil
}

func newRootCmd(opts *cliOptions) *cobra.Command {
	root := &cobra.Command{
		Use:     "whale",
		Short:   "Whale: DeepSeek-native coding agent for the terminal.",
		Version: build.CurrentVersion(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unknown command: %s", args[0])
			}
			if err := prepareCLIConfig(cmd, opts); err != nil {
				return err
			}
			return runLoop(opts, app.StartOptions{NewSession: true})
		},
	}
	root.SetVersionTemplate("{{.Version}}\n")
	bindPersistentFlags(root, opts)
	root.AddCommand(newExecCmd(opts))
	root.AddCommand(newDoctorCmd(opts))
	root.AddCommand(newMigrateConfigCmd(opts))
	root.AddCommand(newSetupCmd(opts))
	root.AddCommand(newResumeCmd(opts))
	return root
}
