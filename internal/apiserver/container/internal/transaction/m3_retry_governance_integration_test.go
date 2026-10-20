//go:build integration && m3_retry_store

package transaction_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	operator "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/operator"
	automation "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/automation"
	apptransaction "github.com/FangcunMount/qs-server/internal/apiserver/application/transaction"
	modtx "github.com/FangcunMount/qs-server/internal/apiserver/container/internal/transaction"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	interpretationrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/run"
	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	mongorepo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	mongostore "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/retryschedule"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	evaluationrepo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	mysqlstore "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/retryschedule"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	events "github.com/FangcunMount/qs-server/internal/apiserver/port/eventtransaction"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"gorm.io/gorm"
)

type retryGovAccess struct{}

func (retryGovAccess) ResolveStoreRange(context.Context, int64, int64, string, string) (appauthz.StoreRange, error) {
	return appauthz.StoreRange{}, nil
}
func (retryGovAccess) ValidateTesteeStoreAccess(_ context.Context, org, user int64, id uint64, _ string, _ string) error {
	if org != 1 || user != 9 || id != 9 {
		return errors.New("fixture actor outside original scope")
	}
	return nil
}

func governedNativeCases() map[string]bool {
	return map[string]bool{"governed_manual_original_root": true, "governed_force_original_root": true, "governed_root_rollback_atomic": true, "governed_commit_reply_discard_recovered": true, "governed_first_plan_and_authorization_reused": true, "governed_changed_input_conflict": true, "governed_wrong_scope_rejected": true, "governed_no_outbox_or_business_attempt": true}
}

func verifyGovernedMySQL(t *testing.T, db *gorm.DB, store *mysqlstore.Store) map[string]bool {
	// These are controlled pre-existing failed facts, not an upstream execution
	// or model-acceptance claim. All new authorizations use the actual host service.
	require.NoError(t, db.AutoMigrate(&evaluationrepo.AssessmentPO{}))
	assessments := evaluationrepo.NewAssessmentRepository(db)
	runs := checkpoint.NewRunRepository(db)
	root := modtx.NewMySQLRunner(db)
	pool, err := db.DB()
	require.NoError(t, err)
	for index, origin := range []retrygovernance.AttemptOrigin{retrygovernance.AttemptOriginManual, retrygovernance.AttemptOriginForce} {
		id := uint64(60001 + index)
		attempt, disposition, action, retryable := 3, retrygovernance.DispositionManualRequired, "retry", true
		if origin == retrygovernance.AttemptOriginForce {
			attempt, disposition, action, retryable = 1, retrygovernance.DispositionTerminal, "force_retry", false
		}
		at := time.Now().UTC().Add(-time.Minute).Truncate(time.Millisecond)
		a, err := assessment.NewAssessment(1, testee.NewID(9), assessment.NewQuestionnaireRefByCode(meta.NewCode("Q-GOV"), "v1"), assessment.NewAnswerSheetRef(meta.FromUint64(id+1000)), assessment.NewAdhocOrigin(), assessment.WithID(meta.FromUint64(id)), assessment.WithEvaluationModel(assessment.NewScaleEvaluationModelRef(meta.ZeroID, meta.NewCode("S"), "v1", "fixture")))
		require.NoError(t, err)
		require.NoError(t, a.SubmitAt(at.Add(-time.Second)))
		failed, err := a.PrepareFailure("controlled original failure", at)
		require.NoError(t, err)
		status, kind, reason, policyVersion, maxAttempts := string(disposition), string(evalrun.FailureKindCalculation), "controlled original failure", "business-retry/v1", uint(3)
		po := checkpoint.RuntimeCheckpointPO{Scope: "evaluation_run", ResourceID: fmt.Sprintf("%d:%d", id, attempt), AssessmentID: &id, AttemptNo: uint(attempt), Status: evalrun.StatusFailed.String(), StartedAt: at.Add(-time.Second), FinishedAt: &at, ErrorCode: &kind, ErrorMessage: &reason, Retryable: retryable, RetryDisposition: &status, PolicyMaxAttempts: &maxAttempts, RetryPolicyVersion: &policyVersion}
		require.NoError(t, root.WithinTransaction(t.Context(), func(ctx context.Context) error {
			// This is a controlled pre-existing fact with an assigned ID. Save
			// intentionally updates such aggregates; seed it with explicit Create.
			tx, err := hostmysql.RequireTx(ctx)
			if err != nil {
				return err
			}
			if err := tx.Create(evaluationrepo.NewAssessmentMapper().ToPO(failed)).Error; err != nil {
				return err
			}
			return tx.Create(&po).Error
		}))
		stored, err := assessments.FindByID(t.Context(), failed.ID())
		require.NoError(t, err)
		require.True(t, stored.Status().IsFailed(), "initial aggregate must exist before authorization")
		before, err := runs.FindLatestByAssessmentID(t.Context(), id)
		require.NoError(t, err)
		require.Equal(t, disposition, before.RetryDecision().Disposition)
		abort := true
		rootCalls := 0
		runner := apptransaction.RunnerFunc(func(ctx context.Context, write func(context.Context) error) error {
			rootCalls++
			err := root.WithinTransaction(ctx, func(local context.Context) error {
				if err := write(local); err != nil {
					return err
				}
				if abort {
					return errors.New("controlled original root rollback")
				}
				return nil
			})
			if err == nil {
				return errors.New("client discards actual root commit reply")
			}
			return err
		})
		service := operator.NewGovernedRetryService(assessments, runs, runner, nil, retryGovAccess{}, operator.WithRetrySchedules(store, model.Builder{Resolver: retryRoutes(t)}))
		ctx := appauthz.WithSnapshot(t.Context(), &appauthz.Snapshot{Permissions: []appauthz.Permission{{Resource: appauthz.AssessmentResource, Action: action, Mode: appauthz.AuthorizationModeUnconditional}}})
		command := operator.GovernedRetryCommand{AssessmentID: id, ExpectedAttempt: attempt, Origin: origin, RequestID: fmt.Sprintf("native-%d", id), Reason: "explicit native original authorization", AuthorizationSubject: "user:9", AuthorizationAction: action}
		_, err = service.Authorize(ctx, operator.Actor{OrgID: 1, OperatorUserID: 9}, command)
		require.ErrorIs(t, err, events.ErrUnresolved)
		unchanged, err := runs.FindLatestByAssessmentID(t.Context(), id)
		require.NoError(t, err)
		require.Equal(t, disposition, unchanged.RetryDecision().Disposition)
		var count int
		require.NoError(t, pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM qs_retry_schedule WHERE subject_id=?", []byte(failed.ID().String())).Scan(&count))
		require.Zero(t, count)
		abort = false
		accepted, err := service.Authorize(ctx, operator.Actor{OrgID: 1, OperatorUserID: 9}, command)
		require.NoError(t, err)
		require.Equal(t, attempt, accepted.Attempt().Number)
		require.Equal(t, retrygovernance.DispositionAutomatic, accepted.RetryDecision().Disposition)
		var raw []byte
		require.NoError(t, pool.QueryRowContext(t.Context(), "SELECT frozen_plan FROM qs_retry_schedule WHERE subject_id=?", []byte(failed.ID().String())).Scan(&raw))
		plan, err := model.Decode(raw)
		require.NoError(t, err)
		require.NoError(t, plan.MatchDecision(accepted.RetryDecision(), plan.BusinessDue()))
		require.Contains(t, string(plan.Authority().Authorization), command.Reason)
		repeated, err := service.Authorize(ctx, operator.Actor{OrgID: 1, OperatorUserID: 9}, command)
		require.NoError(t, err)
		require.Equal(t, accepted.ID(), repeated.ID())
		require.Equal(t, 2, rootCalls)
		record, err := store.Find(t.Context(), plan)
		require.NoError(t, err)
		require.Equal(t, raw, record.Plan.Bytes())
		command.Reason = "changed authorization"
		_, err = service.Authorize(ctx, operator.Actor{OrgID: 1, OperatorUserID: 9}, command)
		require.ErrorIs(t, err, model.ErrConflict)
		require.Equal(t, 2, rootCalls)
		_, err = service.Authorize(ctx, operator.Actor{OrgID: 2, OperatorUserID: 9}, command)
		require.Error(t, err)
		require.Equal(t, 2, rootCalls)
		require.NoError(t, pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM qs_retry_schedule WHERE subject_id=?", []byte(failed.ID().String())).Scan(&count))
		require.Equal(t, 1, count)
	}
	var requests int
	require.NoError(t, pool.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM rm_tx_request").Scan(&requests))
	require.Zero(t, requests)
	require.False(t, db.Migrator().HasTable("rm_outbox"))
	return governedNativeCases()
}

type retryGovFacts struct{ record *evaluationfact.Record }

func (r retryGovFacts) FindByID(context.Context, meta.ID) (*evaluationfact.Record, error) {
	return r.record, nil
}
func (r retryGovFacts) FindByAssessmentID(context.Context, meta.ID) (*evaluationfact.Record, error) {
	return r.record, nil
}

func verifyGovernedMongo(t *testing.T, db *mongo.Database, store *mongostore.Store) map[string]bool {
	root := modtx.NewMongoRunner(db, modtx.MongoRunnerOptions{Boundary: "interpretation_commit", Limiter: backpressure.NewLimiter(2, time.Second)})
	generations := mongorepo.NewGenerationRepositoryWithoutSchema(db)
	runs := mongorepo.NewRunRepositoryWithoutSchema(db)
	guard, err := mongorepo.NewRetryAuthorizationGuard(db)
	require.NoError(t, err)
	for index, origin := range []retrygovernance.AttemptOrigin{retrygovernance.AttemptOriginManual, retrygovernance.AttemptOriginForce} {
		id := uint64(101 + index*100)
		attempt, retryable, disposition := 3, true, retrygovernance.DispositionManualRequired
		if origin == retrygovernance.AttemptOriginForce {
			attempt, retryable, disposition = 1, false, retrygovernance.DispositionTerminal
		}
		at := time.Now().UTC().Add(-time.Minute).Truncate(time.Millisecond)
		g, err := generation.New(meta.FromUint64(id), generation.Key{OutcomeID: meta.FromUint64(id + 10), ReportType: policy.ReportTypeStandard, TemplateVersion: "v1"}, at.Add(-time.Minute))
		require.NoError(t, err)
		original, err := interpretationrun.NewPending(meta.FromUint64(id+1), g.ID(), attempt)
		require.NoError(t, err)
		require.NoError(t, original.StartWithLease(at.Add(-time.Second), "controlled-original", at.Add(time.Minute)))
		require.NoError(t, g.Begin(original.ID(), *original.StartedAt()))
		require.NoError(t, original.Fail(at, interpretationrun.Failure{Kind: interpretationrun.FailureKindBuild, Code: "controlled", SafeMessage: "fixture", Retryable: retryable}))
		require.NoError(t, g.Fail(original.ID(), at))
		require.NoError(t, root.WithinTransaction(t.Context(), func(ctx context.Context) error {
			if err := generations.Create(ctx, g); err != nil {
				return err
			}
			return runs.Create(ctx, original)
		}))
		abort := true
		rootCalls := 0
		runner := apptransaction.RunnerFunc(func(ctx context.Context, write func(context.Context) error) error {
			rootCalls++
			err := root.WithinTransaction(ctx, func(local context.Context) error {
				if err := write(local); err != nil {
					return err
				}
				if abort {
					return errors.New("controlled original root rollback")
				}
				return nil
			})
			if err == nil {
				return errors.New("client discards actual root commit reply")
			}
			return err
		})
		facts := retryGovFacts{record: evaluationfact.NewRecord(evaluationfact.NewRecordInput{ID: g.Key().OutcomeID, OrgID: 7, AssessmentID: meta.FromUint64(id + 20), TesteeID: 9})}
		service := automation.NewGovernedRetryService(generations, runs, facts, runner, nil, automation.WithRetrySchedules(store, model.Builder{Resolver: retryRoutes(t)}, guard))
		command := automation.GovernedRetryCommand{OrgID: 7, GenerationID: g.ID(), ExpectedAttempt: attempt, Origin: origin, RequestID: fmt.Sprintf("native-%d", id), Reason: "explicit native original authorization"}
		_, err = service.Authorize(t.Context(), command)
		require.ErrorIs(t, err, events.ErrUnresolved)
		unchanged, err := runs.FindByID(t.Context(), original.ID())
		require.NoError(t, err)
		require.Equal(t, disposition, unchanged.RetryDecision().Disposition)
		count, err := db.Collection("qs_retry_schedule").CountDocuments(t.Context(), bson.M{"subject_id": g.ID().String()})
		require.NoError(t, err)
		require.Zero(t, count)
		abort = false
		accepted, err := service.Authorize(t.Context(), command)
		require.NoError(t, err)
		require.Equal(t, attempt, accepted.Attempt())
		require.Equal(t, retrygovernance.DispositionAutomatic, accepted.RetryDecision().Disposition)
		var persisted struct {
			Raw []byte `bson:"frozen_plan"`
		}
		require.NoError(t, db.Collection("qs_retry_schedule").FindOne(t.Context(), bson.M{"subject_id": g.ID().String()}).Decode(&persisted))
		plan, err := model.Decode(persisted.Raw)
		require.NoError(t, err)
		require.NoError(t, plan.MatchDecision(accepted.RetryDecision(), plan.BusinessDue()))
		require.Contains(t, string(plan.Authority().Authorization), command.Reason)
		proof := accepted.RetryEventEvidence()
		require.NotNil(t, proof)
		require.NoError(t, proof.Validate())
		repeated, err := service.Authorize(t.Context(), command)
		require.NoError(t, err)
		require.Equal(t, proof, repeated.RetryEventEvidence())
		require.Equal(t, 2, rootCalls)
		record, err := store.Find(t.Context(), plan)
		require.NoError(t, err)
		require.Equal(t, persisted.Raw, record.Plan.Bytes())
		command.Reason = "different authorization"
		_, err = service.Authorize(t.Context(), command)
		require.ErrorIs(t, err, model.ErrConflict)
		require.Equal(t, 2, rootCalls)
		command.OrgID = 8
		_, err = service.Authorize(t.Context(), command)
		require.Error(t, err)
		require.Equal(t, 2, rootCalls)
		count, err = db.Collection("qs_retry_schedule").CountDocuments(t.Context(), bson.M{"subject_id": g.ID().String()})
		require.NoError(t, err)
		require.EqualValues(t, 1, count)
	}
	requests, err := db.Collection("rm_tx_request").CountDocuments(t.Context(), bson.D{})
	require.NoError(t, err)
	require.Zero(t, requests)
	names, err := db.ListCollectionNames(t.Context(), bson.D{})
	require.NoError(t, err)
	require.NotContains(t, names, "rm_outbox")
	return governedNativeCases()
}
