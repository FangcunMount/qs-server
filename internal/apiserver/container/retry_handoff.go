package container

import (
	"context"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing/retrydispatch"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	modtx "github.com/FangcunMount/qs-server/internal/apiserver/container/internal/transaction"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	mongoAuthority "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/outboxhistory"
	mongoSchedule "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/retryschedule"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	mysqlSchedule "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/retryschedule"
	mysqlBase "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

// NewMySQLRetryHandoff borrows a maintenance host's explicitly configured
// primary. Unlike BuildRetryDispatchers it needs no online Container, owner
// runtime, producer or Broker. It never installs schema or opens connections.
func NewMySQLRetryHandoff(db *gorm.DB, binding transaction.Binding, opts mysqlBase.BaseRepositoryOptions) (*app.Handoff, error) {
	store, err := mysqlSchedule.New(db, binding, opts)
	if err != nil {
		return nil, err
	}
	return app.NewHandoff(store, modtx.NewMySQLRunner(db), checkpoint.NewRepository(db), binding)
}

// NewMongoRetryHandoff owns no client or worker. The original host's explicit
// limiter remains held for each full session, including driver callback re-entry.
func NewMongoRetryHandoff(db *mongo.Database, binding transaction.Binding, opts mongoBase.BaseRepositoryOptions) (*app.Handoff, error) {
	if opts.Limiter == nil {
		return nil, transaction.ErrBinding
	}
	store, err := mongoSchedule.New(db, binding, opts)
	if err != nil {
		return nil, err
	}
	authority, err := mongoAuthority.NewRetryDispatchAuthority(db, opts)
	if err != nil {
		return nil, err
	}
	runner := modtx.NewMongoRunner(db, modtx.MongoRunnerOptions{Boundary: "retry_handoff", Limiter: opts.Limiter})
	guarded := apptransaction.RunnerFunc(func(ctx context.Context, transfer func(context.Context) error) error {
		// listCollections uses the outer context because MongoDB forbids it inside
		// a transaction. The original name must resolve to the same physical
		// collection before the transaction and before its callback commits.
		source := outboxhistory.NewOriginalReader(ctx, db)
		if _, err := source.Collection(); err != nil {
			return err
		}
		return runner.WithinTransaction(ctx, func(tx context.Context) error {
			if err := transfer(tx); err != nil {
				return err
			}
			return source.Verify()
		})
	})
	return app.NewHandoff(store, guarded, authority, binding)
}
