//go:build integration && reliable_messaging && reliable_messaging_m4 && reliable_messaging_m4_integration

package transaction

import (
	"context"
	"database/sql"
	"os"
	"strconv"
	"testing"
	"time"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	appintake "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/intake"
	assessmentcache "github.com/FangcunMount/qs-server/internal/apiserver/cache/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	assessmentmysql "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	mysqlstandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	errorcode "github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestStandardAssessmentOriginalTransaction(t *testing.T) {
	dsn := os.Getenv("RM_QS_ASSESSMENT_DSN")
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil || parsed.Net != "tcp" || parsed.Addr != "mysql:3306" || parsed.DBName != "m4_qs_assessment" {
		t.Fatal("disposable m4_qs_assessment MySQL required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer sqlDB.Close()
	require.NoError(t, db.AutoMigrate(&assessmentmysql.AssessmentPO{}))
	_, err = sqlDB.ExecContext(ctx, sdkmysql.Schema)
	require.NoError(t, err)
	_, err = sqlDB.ExecContext(ctx, `ALTER TABLE rm_outbox ADD KEY ix_rm_outbox_message_id (message_id,id)`)
	require.NoError(t, err)
	require.NoError(t, createEvaluationRequestRefTable(ctx, sqlDB))
	require.NoError(t, createGapRecoveryRequestTable(ctx, sqlDB))
	require.NoError(t, db.AutoMigrate(&checkpoint.RuntimeCheckpointPO{}))
	config, err := eventcatalog.Parse([]byte(`version: "1"
topics:
  evaluation:
    name: qs.evaluation.lifecycle
events:
  evaluation.requested:
    topic: evaluation
    delivery: durable_outbox
    aggregate: Assessment
    domain: evaluation
    handler: evaluation_requested_handler
`))
	require.NoError(t, err)
	stager, err := mysqlstandard.NewStager(eventcatalog.NewCatalog(config), eventruntime.SourceAPIServer)
	require.NoError(t, err)
	repo := assessmentcache.NewInvalidatingAssessmentRepository(assessmentmysql.NewAssessmentRepository(db), nil)
	runner := NewMySQLRunner(db)
	service := appintake.NewService(repo, proofModelValidator{}, runner, stager)
	kind, code, version := "scale", "MODEL-1", "1.0.0"
	command := appintake.CreateCommand{OrgID: 1, TesteeID: 2, AnswerSheetID: 3, QuestionnaireCode: "Q-001", QuestionnaireVersion: "v1", OriginType: "adhoc", ModelKind: &kind, ModelCode: &code, ModelVersion: &version}
	created, err := service.CreateForAnswerSheet(ctx, command)
	require.NoError(t, err)
	require.Equal(t, "pending", created.Status)
	var n int64
	require.NoError(t, db.Table("rm_outbox").Count(&n).Error)
	require.Zero(t, n)

	// The database rejects the intent after the domain state update; the host
	// transaction must roll the Assessment back to pending.
	require.NoError(t, db.Exec(`CREATE TRIGGER rm_reject_standard_event BEFORE INSERT ON rm_outbox FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'proof standard event write failure'`).Error)
	_, err = service.SubmitForEvaluation(ctx, created.ID)
	require.Error(t, err)
	found, err := service.FindByAnswerSheetID(ctx, command.AnswerSheetID)
	require.NoError(t, err)
	require.Equal(t, "pending", found.Status)
	require.NoError(t, db.Table("rm_outbox").Count(&n).Error)
	require.Zero(t, n)
	require.NoError(t, db.Table("qs_rm_evaluation_request_ref").Count(&n).Error)
	require.Zero(t, n)
	require.NoError(t, db.Exec("DROP TRIGGER rm_reject_standard_event").Error)

	// The identity index is part of the same transaction, not a best-effort
	// follow-up. Its failure must roll back the Assessment and SDK Outbox too.
	rejectedCommand := command
	rejectedCommand.AnswerSheetID = 4
	rejected, err := service.CreateForAnswerSheet(ctx, rejectedCommand)
	require.NoError(t, err)
	require.NoError(t, db.Exec(`CREATE TRIGGER rm_reject_request_ref BEFORE INSERT ON qs_rm_evaluation_request_ref FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'proof request identity write failure'`).Error)
	_, err = service.SubmitForEvaluation(ctx, rejected.ID)
	require.Error(t, err)
	found, err = service.FindByAnswerSheetID(ctx, rejectedCommand.AnswerSheetID)
	require.NoError(t, err)
	require.Equal(t, "pending", found.Status)
	require.NoError(t, db.Table("rm_outbox").Count(&n).Error)
	require.Zero(t, n)
	require.NoError(t, db.Table("qs_rm_evaluation_request_ref").Count(&n).Error)
	require.Zero(t, n)
	require.NoError(t, db.Exec("DROP TRIGGER rm_reject_request_ref").Error)

	barrier := &proofPendingBarrier{Repository: repo, arrived: make(chan struct{}, 2), release: make(chan struct{})}
	concurrent := appintake.NewService(barrier, proofModelValidator{}, runner, stager)
	results := make(chan error, 2)
	for range 2 {
		go func() { _, e := concurrent.SubmitForEvaluation(ctx, created.ID); results <- e }()
	}
	for range 2 {
		select {
		case <-barrier.arrived:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	close(barrier.release)
	var successes int
	for range 2 {
		if resultErr := <-results; resultErr == nil {
			successes++
		} else {
			require.True(t, cberrors.IsCode(resultErr, errorcode.ErrConflict), "unexpected competing error: %v", resultErr)
		}
	}
	require.Equal(t, 1, successes)
	require.NoError(t, db.Table("rm_outbox").Count(&n).Error)
	require.EqualValues(t, 1, n)
	require.NoError(t, db.Table("qs_rm_evaluation_request_ref").Count(&n).Error)
	require.EqualValues(t, 1, n)
	store, err := sdkmysql.New(sqlDB)
	require.NoError(t, err)
	claims, err := store.ClaimDue(ctx, 2, time.Minute)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	require.Equal(t, "evaluation.requested", claims[0].Message.Input().EventType)
	require.Equal(t, "org:1", claims[0].Message.Input().Scope)
	require.Equal(t, "qs.evaluation.lifecycle", claims[0].Message.Input().Destination)
	var refAssessmentID uint64
	var refOrgID int64
	var refEventID string
	require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT assessment_id,org_id,event_id FROM qs_rm_evaluation_request_ref WHERE assessment_id=?`, created.ID).
		Scan(&refAssessmentID, &refOrgID, &refEventID))
	require.Equal(t, created.ID, refAssessmentID)
	require.EqualValues(t, 1, refOrgID)
	require.Equal(t, claims[0].Message.Input().ID, refEventID)
	foreignScope := event.New(eventcatalog.EvaluationRequested, "Evaluation", strconv.FormatUint(created.ID, 10), map[string]any{"org_id": 999})
	require.Error(t, runner.WithinTransaction(ctx, func(txCtx context.Context) error {
		return stager.Stage(txCtx, foreignScope)
	}))
	require.NoError(t, db.Table("rm_outbox").Count(&n).Error)
	require.EqualValues(t, 1, n)
	require.NoError(t, db.Table("qs_rm_evaluation_request_ref").Count(&n).Error)
	require.EqualValues(t, 1, n)
	require.NoError(t, store.Confirm(ctx, claims[0]))
	found, err = service.FindByAnswerSheetID(ctx, command.AnswerSheetID)
	require.NoError(t, err)
	require.Equal(t, "submitted", found.Status)
	// Only the disposable test record is aged past the review cutoff.
	_, err = sqlDB.ExecContext(ctx, `UPDATE assessment SET submitted_at=DATE_SUB(NOW(),INTERVAL 20 MINUTE) WHERE id=?`, created.ID)
	require.NoError(t, err)
	inspection, err := mysqlstandard.InspectOriginalRequest(ctx, sqlDB, int64(command.OrgID), created.ID, time.Now().Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, "candidate_never_claimed", inspection.State)
	require.Equal(t, refEventID, inspection.EventID)
	var originalVersion uint64
	var originalPayload []byte
	require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT version,payload FROM rm_outbox WHERE message_id=?`, refEventID).Scan(&originalVersion, &originalPayload))
	ledger, err := mysqlstandard.NewGapRecoveryLedger(sqlDB)
	require.NoError(t, err)
	review := mysqlstandard.GapRecoveryRequest{OrgID: int64(command.OrgID), RequestID: "qs04-reviewed-1", ActorID: 99,
		AssessmentID: created.ID, EventID: refEventID, ExpectedVersion: originalVersion, Reason: "isolated post-confirm gap",
		SubmittedBefore: time.Now().Add(-10 * time.Minute)}
	stale := review
	stale.RequestID = "qs04-stale-version"
	stale.ExpectedVersion++
	staleResult, err := ledger.Authorize(ctx, stale)
	require.NoError(t, err)
	require.False(t, staleResult.Authorized)
	require.Equal(t, "version_conflict", staleResult.Code)
	var stateBefore string
	var versionBefore uint64
	require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT state,version FROM rm_outbox WHERE message_id=?`, refEventID).Scan(&stateBefore, &versionBefore))
	require.Equal(t, "published", stateBefore)
	require.Equal(t, originalVersion, versionBefore)
	require.NoError(t, db.Exec(`CREATE TRIGGER rm_reject_recovery_result BEFORE UPDATE ON qs_rm_gap_recovery_request
		FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'proof recovery ledger failure'`).Error)
	failedReview := review
	failedReview.RequestID = "qs04-ledger-failure"
	_, err = ledger.Authorize(ctx, failedReview)
	require.Error(t, err)
	require.NoError(t, db.Exec("DROP TRIGGER rm_reject_recovery_result").Error)
	require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT state,version FROM rm_outbox WHERE message_id=?`, refEventID).Scan(&stateBefore, &versionBefore))
	require.Equal(t, "published", stateBefore)
	require.Equal(t, originalVersion, versionBefore)
	require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM qs_rm_gap_recovery_request WHERE request_id=?`, failedReview.RequestID).Scan(&n))
	require.Zero(t, n)
	result, err := ledger.Authorize(ctx, review)
	require.NoError(t, err)
	require.True(t, result.Authorized)
	require.Equal(t, "authorized", result.Code)
	require.Equal(t, originalVersion, result.OutboxVersionBefore)
	require.Equal(t, originalVersion+1, result.OutboxVersionAfter)
	var state, manualRequestID string
	var versionAfter uint64
	var payloadAfter []byte
	require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT state,version,payload,manual_replay_request_id FROM rm_outbox WHERE message_id=?`, refEventID).
		Scan(&state, &versionAfter, &payloadAfter, &manualRequestID))
	require.Equal(t, "retry_wait", state)
	require.Equal(t, originalVersion+1, versionAfter)
	require.Equal(t, originalPayload, payloadAfter)
	require.Equal(t, review.RequestID, manualRequestID)
	repeated, err := ledger.Authorize(ctx, review)
	require.NoError(t, err)
	require.Equal(t, result, repeated)
	resolved, foundRequest, err := ledger.Resolve(ctx, review)
	require.NoError(t, err)
	require.True(t, foundRequest)
	require.Equal(t, result, resolved)
	changed := review
	changed.Reason = "different reason"
	_, err = ledger.Authorize(ctx, changed)
	require.ErrorIs(t, err, mysqlstandard.ErrGapRecoveryInputConflict)
	_, err = sqlDB.ExecContext(ctx, `INSERT INTO runtime_checkpoint
		(assessment_id,scope,resource_id,attempt_no,status,started_at,deleted_at)
		VALUES (?,'evaluation_run','soft-deleted-proof',1,'running',UTC_TIMESTAMP(3),UTC_TIMESTAMP(3))`, created.ID)
	require.NoError(t, err)
	inspection, err = mysqlstandard.InspectOriginalRequest(ctx, sqlDB, int64(command.OrgID), created.ID, time.Now().Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, "manual_required", inspection.State)
	require.Equal(t, "ever_claimed", inspection.Reason)
	blocked := review
	blocked.RequestID = "qs04-reviewed-2"
	blocked.ExpectedVersion = versionAfter
	denied, err := ledger.Authorize(ctx, blocked)
	require.NoError(t, err)
	require.False(t, denied.Authorized)
	require.Equal(t, "ever_claimed", denied.Code)
	require.NoError(t, sqlDB.QueryRowContext(ctx, `SELECT state,version FROM rm_outbox WHERE message_id=?`, refEventID).Scan(&state, &versionAfter))
	require.Equal(t, "retry_wait", state)
	require.Equal(t, originalVersion+1, versionAfter)
	// The actual Claim implementation must not insert its first Run while a
	// recovery transaction holds the missing assessment_id index range.
	lockTx, err := sqlDB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	require.NoError(t, err)
	defer func() { _ = lockTx.Rollback() }()
	probeID := uint64(9001)
	rows, err := lockTx.QueryContext(ctx, `SELECT id FROM runtime_checkpoint
		FORCE INDEX (idx_runtime_checkpoint_assessment_id)
		WHERE assessment_id=? FOR UPDATE`, probeID)
	require.NoError(t, err)
	require.False(t, rows.Next())
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	type claimOutcome struct {
		result evaluationrun.ClaimResult
		err    error
	}
	claimed := make(chan claimOutcome, 1)
	go func() {
		now := time.Now()
		result, claimErr := checkpoint.NewRepository(db).Claim(ctx, evaluationrun.ClaimRequest{
			AssessmentID: probeID, Token: "first-claim-proof", ClaimedAt: now, LeaseUntil: now.Add(time.Minute),
		})
		claimed <- claimOutcome{result: result, err: claimErr}
	}()
	select {
	case outcome := <-claimed:
		t.Fatalf("first Claim escaped gap lock: %+v, %v", outcome.result, outcome.err)
	case <-time.After(350 * time.Millisecond):
	}
	require.NoError(t, lockTx.Commit())
	select {
	case outcome := <-claimed:
		require.NoError(t, outcome.err)
		require.True(t, outcome.result.Claimed)
	case <-time.After(5 * time.Second):
		t.Fatal("first Claim did not resume after gap lock commit")
	}
}
