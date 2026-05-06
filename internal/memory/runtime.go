package memory

import "github.com/usewhale/whale/internal/core"

type RuntimeState struct {
	Prefix  *ImmutablePrefix
	Log     *AppendOnlyLog
	Scratch *VolatileScratch
}

func NewRuntimeState(prefix *ImmutablePrefix) *RuntimeState {
	if prefix == nil {
		prefix = NewImmutablePrefix(nil)
	}
	return &RuntimeState{
		Prefix:  prefix,
		Log:     NewAppendOnlyLog(),
		Scratch: NewVolatileScratch(),
	}
}

func (r *RuntimeState) BuildProviderHistory() []core.Message {
	out := make([]core.Message, 0, 1+r.Log.Len())
	out = append(out, r.Prefix.ToMessages()...)
	out = append(out, r.Log.Entries()...)
	return out
}
