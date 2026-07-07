package transaction

import "context"

// Runner 是application-facing transaction boundary。
type Runner interface {
	WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error
}

// RunnerFunc 适配函数 到 Runner。
type RunnerFunc func(ctx context.Context, fn func(txCtx context.Context) error) error

func (f RunnerFunc) WithinTransaction(ctx context.Context, fn func(txCtx context.Context) error) error {
	return f(ctx, fn)
}
