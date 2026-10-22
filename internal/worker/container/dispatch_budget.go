package container

import (
	"context"
	"errors"

	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
)

// One existing worker execution budget covers all physical inputs and the
// single deferred resume loop. At most one deferred callback waits in addition
// to the SDK's already bounded physical deliveries; execution never exceeds N.
type dispatchBudget struct{ slots chan struct{} }

func newDispatchBudget(limit int) *dispatchBudget {
	return &dispatchBudget{slots: make(chan struct{}, limit)}
}

func (b *dispatchBudget) run(ctx context.Context, call func() (eventruntime.DispatchResult, error)) (eventruntime.DispatchResult, error) {
	if b == nil {
		return eventruntime.DispatchResult{}, errors.New("explicit worker dispatch budget required")
	}
	if err := ctx.Err(); err != nil {
		return eventruntime.DispatchResult{}, err
	}
	select {
	case b.slots <- struct{}{}:
	case <-ctx.Done():
		return eventruntime.DispatchResult{}, ctx.Err()
	}
	defer func() { <-b.slots }()
	if err := ctx.Err(); err != nil {
		return eventruntime.DispatchResult{}, err
	}
	return call()
}
