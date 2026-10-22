package container

import (
	"context"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing/retrydispatch"
	modtx "github.com/FangcunMount/qs-server/internal/apiserver/container/internal/transaction"
	mongoBase "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	mongoTx "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/eventtransaction"
	mongoAuthority "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	mongoSchedule "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/retryschedule"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	mysqlSchedule "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/retryschedule"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/retryschedule"
	mysqlBase "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/reliable-messaging/transaction"
)

type RetryDispatchers struct {
	MySQL, Mongo *app.Dispatcher
	stores       []port.Store
}

// BuildRetryDispatchers borrows the original business stores and the two process
// owners. It creates neither producer, database pool, schema nor background work.
func (c *Container) BuildRetryDispatchers(automatic bool) (*RetryDispatchers, error) {
	if !c.HasTransactionOwners() || c.mysqlDB == nil || c.mongoDB == nil || c.EvaluationTransactionOwner() == nil || c.ReportTransactionOwner() == nil {
		return nil, transaction.ErrBinding
	}
	if err := c.validateTransactionBindings(); err != nil {
		return nil, err
	}
	mysqlOwner, mongoOwner := c.EvaluationTransactionOwner(), c.ReportTransactionOwner()
	sqlStore, err := mysqlSchedule.New(c.mysqlDB, mysqlOwner.Binding(), mysqlBase.BaseRepositoryOptions{Limiter: c.MySQLLimiter()})
	if err != nil {
		return nil, err
	}
	mongoOptions := mongoBase.BaseRepositoryOptions{Limiter: c.MongoLimiter()}
	documentStore, err := mongoSchedule.New(c.mongoDB, mongoOwner.Binding(), mongoOptions)
	if err != nil {
		return nil, err
	}
	authority, err := mongoAuthority.NewRetryDispatchAuthority(c.mongoDB, mongoOptions)
	if err != nil {
		return nil, err
	}
	commit, err := mongoTx.NewCommitter(c.mongoDB, mongoOwner, modtx.NewMongoRunner(c.mongoDB, modtx.MongoRunnerOptions{Boundary: "retry_dispatch", Limiter: c.MongoLimiter()}), c.ReportTransactionRoutes())
	if err != nil {
		return nil, err
	}
	sqlDispatch, err := app.New(sqlStore, mysqlOwner, checkpoint.NewRepository(c.mysqlDB), app.WithAutomaticRetrySwitch(func() bool { return !automatic }))
	if err != nil {
		return nil, err
	}
	documentDispatch, err := app.New(documentStore, commit, authority, app.WithAutomaticRetrySwitch(func() bool { return !automatic }))
	if err != nil {
		return nil, err
	}
	return &RetryDispatchers{MySQL: sqlDispatch, Mongo: documentDispatch, stores: []port.Store{sqlStore, documentStore}}, nil
}

// Validate uses fresh bounded authoritative reads. The host explicitly installs
// schema before Start; this method never reserves or prepares any message.
func (d *RetryDispatchers) Validate(ctx context.Context) error {
	for _, store := range d.stores {
		check, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := store.Validate(check)
		cancel()
		if err != nil {
			return err
		}
	}
	return nil
}
