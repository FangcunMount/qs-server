package assembler

import (
	"context"
	"fmt"

	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

func newMySQLTransactionRunner(db *gorm.DB) apptransaction.Runner {
	uow := mysql.NewUnitOfWork(db)
	return apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		return uow.WithinTransaction(ctx, fn)
	})
}

func newMongoTransactionRunner(db *mongo.Database) apptransaction.Runner {
	return apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		if db == nil {
			return fmt.Errorf("mongo database is nil")
		}
		if fn == nil {
			return nil
		}

		session, err := db.Client().StartSession()
		if err != nil {
			return err
		}
		defer session.EndSession(ctx)

		_, err = session.WithTransaction(ctx, func(txCtx mongo.SessionContext) (interface{}, error) {
			return nil, fn(txCtx)
		})
		return err
	})
}
