package transaction

import (
	"context"
	"errors"
	"fmt"
	"time"

	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readconcern"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"
	"gorm.io/gorm"
)

// MongoRunnerOptions defines the stable operational identity and admission
// policy for one Mongo transaction boundary.
type MongoRunnerOptions struct {
	Boundary string
	Limiter  backpressure.Acquirer
}

// NewMySQLRunner returns a MySQL unit-of-work transaction runner.
func NewMySQLRunner(db *gorm.DB) apptransaction.Runner {
	uow := mysql.NewUnitOfWork(db)
	return apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		return uow.WithinTransaction(ctx, fn)
	})
}

// NewMongoRunner holds one Mongo backpressure slot for the entire transaction.
// Repositories recognize the attached SessionContext and must not acquire the
// same dependency limiter again from inside fn.
func NewMongoRunner(db *mongo.Database, opts MongoRunnerOptions) apptransaction.Runner {
	return apptransaction.RunnerFunc(func(ctx context.Context, fn func(context.Context) error) error {
		if db == nil {
			return fmt.Errorf("mongo database is nil")
		}
		if opts.Boundary == "" {
			return fmt.Errorf("mongo transaction boundary is required")
		}
		if opts.Limiter == nil {
			return fmt.Errorf("mongo transaction limiter is required for boundary %q", opts.Boundary)
		}
		if fn == nil {
			return nil
		}

		transactionStarted := time.Now()
		admissionStarted := transactionStarted
		var release func()
		ctx, release, err := opts.Limiter.Acquire(ctx)
		if err != nil {
			observeMongoAdmission(opts.Boundary, "rejected", time.Since(admissionStarted))
			observeMongoTransaction(opts.Boundary, "admission_rejected", time.Since(transactionStarted))
			return err
		}
		observeMongoAdmission(opts.Boundary, "acquired", time.Since(admissionStarted))
		defer release()

		session, err := db.Client().StartSession()
		if err != nil {
			observeMongoCallbackAttempts(opts.Boundary, 0)
			observeMongoTransaction(opts.Boundary, "session_error", time.Since(transactionStarted))
			return err
		}
		defer session.EndSession(ctx)

		callbackAttempts := 0
		_, err = session.WithTransaction(ctx, func(txCtx mongo.SessionContext) (interface{}, error) {
			callbackAttempts++
			return nil, fn(txCtx)
		}, mongoTransactionOptions())
		observeMongoCallbackAttempts(opts.Boundary, callbackAttempts)
		observeMongoTransaction(opts.Boundary, mongoTransactionOutcome(err), time.Since(transactionStarted))
		return err
	})
}

type mongoErrorLabeler interface {
	HasErrorLabel(string) bool
}

func mongoTransactionOutcome(err error) string {
	if err == nil {
		return "committed"
	}
	var labeled mongoErrorLabeler
	if errors.As(err, &labeled) && labeled.HasErrorLabel("UnknownTransactionCommitResult") {
		return "commit_unknown"
	}
	return "rolled_back"
}

// mongoTransactionOptions makes the cross-document publish contract explicit.
// Snapshot reads and majority writes are only supported by a replica set (or
// sharded deployment), so a standalone Mongo deployment must fail instead of
// silently weakening atomic publication semantics.
func mongoTransactionOptions() *options.TransactionOptions {
	return options.Transaction().
		SetReadPreference(readpref.Primary()).
		SetReadConcern(readconcern.Snapshot()).
		SetWriteConcern(writeconcern.Majority())
}
