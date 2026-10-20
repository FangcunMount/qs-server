package interpretation

import (
	"context"
	"strconv"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/retryschedule"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type retryAuthorizationGuard struct {
	db          *mongo.Database
	generations *GenerationRepository
}

func NewRetryAuthorizationGuard(db *mongo.Database, opts ...base.BaseRepositoryOptions) (port.AuthorizationGuard, error) {
	if db == nil {
		return nil, transaction.ErrBinding
	}
	return &retryAuthorizationGuard{db: db, generations: NewGenerationRepositoryWithoutSchema(db, opts...)}, nil
}

func (g *retryAuthorizationGuard) GuardAuthorization(ctx context.Context, f model.Frozen) error {
	if !f.Valid() || f.Authority().Owner != "qs.mongo" || f.Authority().Kind != model.Interpretation {
		return transaction.ErrBinding
	}
	session := mongo.SessionFromContext(ctx)
	if session == nil {
		return transaction.ErrTransactionRequired
	}
	if session.Client() != g.db.Client() {
		return transaction.ErrBinding
	}
	bridge, ok := session.(mongo.XSession) //nolint:staticcheck // Pinned active original session bridge.
	if !ok || bridge.ClientSession() == nil || !bridge.ClientSession().TransactionRunning() {
		return transaction.ErrTransactionRequired
	}
	id, err := strconv.ParseUint(f.Authority().SubjectID, 10, 64)
	if err != nil {
		return err
	}
	run, err := strconv.ParseUint(f.Authority().RunID, 10, 64)
	if err != nil {
		return err
	}
	var po ReportGenerationPO
	if err := g.generations.FindOne(ctx, bson.M{"domain_id": id}, &po); err != nil {
		return err
	}
	if po.LatestRunID != run || po.OutcomeID != f.Authority().OutcomeID || po.Status != string(generation.StatusFailed) {
		return transaction.ErrStaleExecution
	}
	changed, err := g.generations.UpdateOne(ctx, bson.M{"domain_id": id, "latest_run_id": run, "status": po.Status, "version": po.Version}, bson.M{"$inc": bson.M{"retry_authorization_guard": int64(1)}})
	if err != nil {
		return err
	}
	if changed.MatchedCount != 1 {
		return transaction.ErrStaleExecution
	}
	return nil
}
