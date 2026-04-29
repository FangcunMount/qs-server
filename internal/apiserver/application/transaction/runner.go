package transaction

import "context"

// Runner is the application-facing transaction boundary.
type Runner interface {
	WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error
}

// RunnerFunc adapts a function to Runner.
type RunnerFunc func(ctx context.Context, fn func(txCtx context.Context) error) error

func (f RunnerFunc) WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error {
	return f(ctx, fn)
}
