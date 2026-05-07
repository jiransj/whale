package agent

import (
	"context"

	"github.com/usewhale/whale/internal/core"
)

func (a *Agent) previewTool(ctx context.Context, call core.ToolCall) map[string]any {
	if a == nil || a.tools == nil {
		return nil
	}
	tool := a.tools.Get(call.Name)
	previewer, ok := tool.(core.ToolPreviewer)
	if !ok {
		return nil
	}
	metadata, err := previewer.Preview(ctx, call)
	if err != nil {
		return map[string]any{
			"kind":          "file_diff",
			"preview_error": err.Error(),
		}
	}
	return metadata
}
