package assembler

import (
	"context"

	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

func newMySQLTransactionRunner(db *gorm.DB) apptransaction.Runner {
	uow := mysql.NewUnitOfWork(db)
	return apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		return uow.WithinTransaction(ctx, fn)
	})
}
