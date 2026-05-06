package app

import "github.com/usewhale/whale/internal/defaults"

const (
	defaultContextWindow  = defaults.DefaultContextWindow
	deepSeekV4ContextSize = defaults.DeepSeekV4ContextWindow
)

func inferredContextWindowForModel(model string) int {
	if model == "" {
		return defaultContextWindow
	}
	if defaults.IsDeepSeekV4Model(model) {
		return deepSeekV4ContextSize
	}
	return defaultContextWindow
}

func resolveContextWindow(configWindow int, model string) int {
	if configWindow > 0 && configWindow != defaultContextWindow {
		return configWindow
	}
	return inferredContextWindowForModel(model)
}
