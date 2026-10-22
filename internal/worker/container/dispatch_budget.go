package container

import (
	"context"
	"errors"
	"sync/atomic"

	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
)

// One existing worker execution budget covers all physical inputs and the
// single deferred resume loop. At most one deferred callback waits in addition
// to the SDK's already bounded physical deliveries; execution never exceeds N.
type dispatchBudget struct {
	slots   chan struct{}
	waiting atomic.Int64
}

// DispatchBudgetSnapshot reports only bounded process counts. Waiting includes
// admitted callers until they acquire a slot or actually return cancellation.
// It contains no message, tenant or payload identity.
type DispatchBudgetSnapshot struct{ Limit, Active, Waiting int }

func (c *Container) DispatchBudgetSnapshot() DispatchBudgetSnapshot {
	if c == nil || c.dispatchBudget == nil {
		return DispatchBudgetSnapshot{}
	}
	b := c.dispatchBudget
	return DispatchBudgetSnapshot{Limit: cap(b.slots), Active: len(b.slots), Waiting: int(b.waiting.Load())}
}

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
	b.waiting.Add(1)
	select {
	case b.slots <- struct{}{}:
	case <-ctx.Done():
		b.waiting.Add(-1)
		return eventruntime.DispatchResult{}, ctx.Err()
	}
	b.waiting.Add(-1)
	defer func() { <-b.slots }()
	if err := ctx.Err(); err != nil {
		return eventruntime.DispatchResult{}, err
	}
	return call()
}
