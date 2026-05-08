package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/usewhale/whale/internal/app"
	"github.com/usewhale/whale/internal/app/service"
)

func Run(cfg app.Config, start app.StartOptions) error {
	ctx := context.Background()
	svc, err := service.New(ctx, cfg, start)
	if err != nil {
		return err
	}
	defer svc.Close()
	modelName := svc.Model()
	effort := svc.ReasoningEffort()
	thinking := "on"
	if !svc.ThinkingEnabled() {
		thinking = "off"
	}
	p := tea.NewProgram(newModel(svc, modelName, effort, thinking))
	_, err = p.Run()
	if err == nil {
		fmt.Printf("To resume this session, run: whale resume %s\n", svc.SessionID())
	}
	return err
}
