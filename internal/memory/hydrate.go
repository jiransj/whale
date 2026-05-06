package memory

import "github.com/usewhale/whale/internal/core"

func HydrateRuntime(prefix *ImmutablePrefix, history []core.Message) *RuntimeState {
	rt := NewRuntimeState(prefix)
	rt.Log.Extend(history)
	return rt
}

func cloneMessages(in []core.Message) []core.Message {
	out := make([]core.Message, 0, len(in))
	for _, msg := range in {
		out = append(out, cloneMessage(msg))
	}
	return out
}

func cloneMessage(msg core.Message) core.Message {
	out := msg
	if len(msg.ToolCalls) > 0 {
		out.ToolCalls = append([]core.ToolCall(nil), msg.ToolCalls...)
	}
	if len(msg.ToolResults) > 0 {
		out.ToolResults = append([]core.ToolResult(nil), msg.ToolResults...)
	}
	return out
}
