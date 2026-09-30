package evaluation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// AcceptanceFailureScope limits the temporary pre-model fault to one dedicated
// test identity and a short window. An absent configuration creates no gate.
type AcceptanceFailureScope struct {
	Token     string    `json:"token"`
	OrgID     int64     `json:"org_id"`
	TesteeID  uint64    `json:"testee_id"`
	ModelCode string    `json:"model_code"`
	StartsAt  time.Time `json:"starts_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type acceptanceClaimInserter interface {
	InsertOne(context.Context, interface{}, ...*options.InsertOneOptions) (*mongo.InsertOneResult, error)
}

// AcceptanceFailureGate uses Mongo's unique _id to claim one failure across
// API instances and restarts. The row remains as audit evidence.
type AcceptanceFailureGate struct {
	claims acceptanceClaimInserter
	scope  AcceptanceFailureScope
	now    func() time.Time
}

func NewAcceptanceFailureGate(claims *mongo.Collection, scope AcceptanceFailureScope) (*AcceptanceFailureGate, error) {
	if claims == nil {
		return nil, fmt.Errorf("evaluation acceptance claim collection is required")
	}
	if err := validateAcceptanceFailureScope(scope, time.Now()); err != nil {
		return nil, err
	}
	return &AcceptanceFailureGate{claims: claims, scope: scope, now: time.Now}, nil
}

func validateAcceptanceFailureScope(scope AcceptanceFailureScope, now time.Time) error {
	if len(scope.Token) < 16 || len(scope.Token) > 80 || strings.Trim(scope.Token, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-") != "" {
		return fmt.Errorf("evaluation acceptance token must be 16-80 alphanumeric/hyphen characters")
	}
	if scope.OrgID <= 0 || scope.TesteeID == 0 || strings.TrimSpace(scope.ModelCode) == "" || strings.TrimSpace(scope.ModelCode) != scope.ModelCode {
		return fmt.Errorf("evaluation acceptance requires exact organization, testee and model code")
	}
	if scope.StartsAt.IsZero() || scope.ExpiresAt.IsZero() || !scope.StartsAt.Before(scope.ExpiresAt) ||
		scope.ExpiresAt.Sub(scope.StartsAt) > 10*time.Minute || !scope.ExpiresAt.After(now) || scope.ExpiresAt.After(now.Add(10*time.Minute)) {
		return fmt.Errorf("evaluation acceptance window must be active or upcoming and at most ten minutes")
	}
	return nil
}

func (g *AcceptanceFailureGate) TryFail(ctx context.Context, a *assessment.Assessment, input *evaluationinput.InputSnapshot, run *evalrun.EvaluationRun) (bool, error) {
	if g == nil || g.claims == nil || a == nil || input == nil || input.Model == nil || run == nil {
		return false, nil
	}
	now := g.now()
	if now.Before(g.scope.StartsAt) || !now.Before(g.scope.ExpiresAt) ||
		run.Attempt().Number != 1 || run.Origin() != retrygovernance.AttemptOriginInitial ||
		a.ID().IsZero() || a.OrgID() != g.scope.OrgID || a.TesteeID().Uint64() != g.scope.TesteeID ||
		input.Model.Code != g.scope.ModelCode {
		return false, nil
	}
	_, err := g.claims.InsertOne(ctx, bson.M{
		"_id":                g.scope.Token,
		"org_id":             g.scope.OrgID,
		"testee_id":          g.scope.TesteeID,
		"model_code":         g.scope.ModelCode,
		"assessment_id":      a.ID().String(),
		"run_id":             run.ID().String(),
		"input_snapshot_ref": run.InputSnapshotRef(),
		"claimed_at":         now.UTC(),
		"expires_at":         g.scope.ExpiresAt.UTC(),
	})
	if mongo.IsDuplicateKeyError(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("persist evaluation acceptance claim: %w", err)
	}
	return true, nil
}
