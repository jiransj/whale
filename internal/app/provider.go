package app

import (
	"strings"

	"github.com/usewhale/whale/internal/llm"
	"github.com/usewhale/whale/internal/llm/deepseek"
)

type providerOptions struct {
	APIKey          string
	BaseURL         string
	Model           string
	ReasoningEffort string
	ThinkingEnabled bool
	MaxTokens       int
}

func newDeepSeekProvider(opts providerOptions) (llm.Provider, error) {
	dsOpts := []deepseek.Option{}
	if strings.TrimSpace(opts.APIKey) != "" {
		dsOpts = append(dsOpts, deepseek.WithAPIKey(opts.APIKey))
	}
	if strings.TrimSpace(opts.BaseURL) != "" {
		dsOpts = append(dsOpts, deepseek.WithBaseURL(opts.BaseURL))
	}
	if strings.TrimSpace(opts.Model) != "" {
		dsOpts = append(dsOpts, deepseek.WithModel(opts.Model))
	}
	dsOpts = append(dsOpts,
		deepseek.WithReasoningEffort(opts.ReasoningEffort),
		deepseek.WithThinking(opts.ThinkingEnabled),
	)
	if opts.MaxTokens > 0 {
		dsOpts = append(dsOpts, deepseek.WithMaxTokens(opts.MaxTokens))
	}
	return deepseek.New(dsOpts...)
}
