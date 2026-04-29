package mysql

import (
	"context"

	gormuow "github.com/FangcunMount/component-base/pkg/uow/gorm"
	"gorm.io/gorm"
)

type TxOptions = gormuow.TxOptions
type UnitOfWork = gormuow.UnitOfWork

var (
	ErrUnitOfWorkUnavailable     = gormuow.ErrUnitOfWorkUnavailable
	ErrActiveTransactionRequired = gormuow.ErrActiveTransactionRequired
)

func NewUnitOfWork(db *gorm.DB) *UnitOfWork {
	return gormuow.NewUnitOfWork(db)
}

func WithTx(ctx context.Context, tx *gorm.DB) context.Context {
	return gormuow.WithTx(ctx, tx)
}

func TxFromContext(ctx context.Context) (*gorm.DB, bool) {
	return gormuow.TxFromContext(ctx)
}

func RequireTx(ctx context.Context) (*gorm.DB, error) {
	return gormuow.RequireTx(ctx)
}

func AfterCommit(ctx context.Context, hook func(context.Context) error) error {
	return gormuow.AfterCommit(ctx, hook)
}
