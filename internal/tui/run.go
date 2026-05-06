package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/usewhale/whale/internal/app"
	"github.com/usewhale/whale/internal/app/service"
	"github.com/usewhale/whale/internal/build"
	"github.com/usewhale/whale/internal/defaults"
)

func Run(cfg app.Config, start app.StartOptions) error {
	ctx := context.Background()
	svc, err := service.New(ctx, cfg, start)
	if err != nil {
		return err
	}
	printStartupBanner(cfg)
	p := tea.NewProgram(newModel(svc))
	_, err = p.Run()
	if err == nil {
		fmt.Printf("To resume this session, run: whale resume %s\n", svc.SessionID())
	}
	return err
}

func printStartupBanner(cfg app.Config) {
	version := build.CurrentVersion()
	cwd := "."
	if wd, err := os.Getwd(); err == nil {
		cwd = wd
		if home, hErr := os.UserHomeDir(); hErr == nil {
			if filepath.Clean(cwd) == filepath.Clean(home) {
				cwd = "~"
			} else if rel, rErr := filepath.Rel(home, cwd); rErr == nil && rel != "." && !strings.HasPrefix(rel, "..") {
				cwd = "~/" + rel
			}
		}
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = defaults.DefaultModel
	}
	effort := strings.TrimSpace(cfg.ReasoningEffort)
	if effort == "" {
		effort = defaults.DefaultReasoningEffort
	}
	fmt.Println("╭────────────────────────────────────────────────────╮")
	fmt.Printf("│ %-50s │\n", fmt.Sprintf(">_ Whale (%s)", version))
	fmt.Printf("│ %-50s │\n", "")
	fmt.Printf("│ %-50s │\n", fmt.Sprintf("model:     %s %s", model, effort))
	fmt.Printf("│ %-50s │\n", fmt.Sprintf("directory: %s", cwd))
	fmt.Println("╰────────────────────────────────────────────────────╯")
}
