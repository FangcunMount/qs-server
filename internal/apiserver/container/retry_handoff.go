package container

import (
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing/retrydispatch"
	modtx "github.com/FangcunMount/qs-server/internal/apiserver/container/internal/transaction"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	mongoAuthority "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
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
	return app.NewHandoff(store, runner, authority, binding)
}
