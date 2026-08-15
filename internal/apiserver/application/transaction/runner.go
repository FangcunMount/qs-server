package transaction

import "context"

// Runner is the application-facing transaction boundary.
//
// Implementations backed by MongoDB may execute fn more than once after a
// transient transaction error. Therefore callers must establish IDs,
// timestamps and domain events before entering fn; fn may only perform
// transaction-aware database work and must not call Redis, MQ, HTTP or any
// irreversible side effect. Caller-owned aggregates must be copied before a
// state transition. Post-commit effects belong after WithinTransaction returns
// nil and must execute at most once.
type Runner interface {
	WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error
}

// RunnerFunc 适配函数 到 Runner。
type RunnerFunc func(ctx context.Context, fn func(txCtx context.Context) error) error

func (f RunnerFunc) WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error {
	return f(ctx, fn)
}
