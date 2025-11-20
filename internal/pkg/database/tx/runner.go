package tx

import (
	"context"
)

// UnitOfWork 定义事务执行接口。
type UnitOfWork[T any] interface {
	WithinTx(ctx context.Context, fn func(T) error) error
}

// Runner 为写用例提供统一事务执行能力。
type Runner[T any] struct {
	UoW UnitOfWork[T]
}

// WithinTx 在事务中执行 fn；若 UoW 为空，则直接执行 fn。
func (r Runner[T]) WithinTx(ctx context.Context, fn func(T) error) error {
	if fn == nil {
		return nil
	}

	if r.UoW == nil {
		var zero T
		return fn(zero)
	}

	return r.UoW.WithinTx(ctx, fn)
}
