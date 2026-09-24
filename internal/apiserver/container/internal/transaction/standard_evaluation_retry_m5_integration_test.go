//go:build integration && reliable_messaging && reliable_messaging_m4 && reliable_messaging_m5

package transaction

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	appexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	appintake "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/intake"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	assessmentmysql "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	mysqlstandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type m5UnavailableInput struct{}

func (m5UnavailableInput) Resolve(context.Context, evaluationinput.InputRef) (*evaluationinput.InputSnapshot, error) {
	return nil, evaluationinput.NewDependencyResolveError(
		evaluationinput.DependencyCategoryModelCatalog, errors.New("controlled outage"), "model catalog unavailable", "model catalog unavailable",
	)
}

// A real failed Evaluation claim must persist its failure and scheduled retry
// in the same MySQL transaction. Rejecting the second Outbox insert rolls back
// both business updates and the first event, while the earlier claim remains.
func TestM5StandardEvaluationFailureAndScheduledRetryTransaction(t *testing.T) {
	dsn := os.Getenv("RM_QS_M5_MYSQL_DSN")
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil || parsed.Net != "tcp" || parsed.Addr != "mysql:3306" || parsed.DBName != "m5_qs_retry" {
		t.Fatal("disposable m5_qs_retry MySQL required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer sqlDB.Close()
	require.NoError(t, db.AutoMigrate(&assessmentmysql.AssessmentPO{}, &checkpoint.RuntimeCheckpointPO{}))
	_, err = sqlDB.ExecContext(ctx, sdkmysql.Schema)
	require.NoError(t, err)

	config, err := eventcatalog.Parse([]byte(`version: "1"
topics:
  evaluation:
    name: qs.evaluation.lifecycle
events:
  evaluation.requested:
    topic: evaluation
    delivery: durable_outbox
    aggregate: Evaluation
    domain: evaluation
    handler: evaluation_requested_handler
  evaluation.failed:
    topic: evaluation
    delivery: durable_outbox
    aggregate: Evaluation
    domain: evaluation
    handler: evaluation_failed_handler
  evaluation.retry.requested:
    topic: evaluation
    delivery: durable_outbox
    aggregate: Evaluation
    domain: evaluation
    handler: evaluation_retry_requested_handler
`))
	require.NoError(t, err)
	stager, err := mysqlstandard.NewStager(eventcatalog.NewCatalog(config), eventruntime.SourceAPIServer)
	require.NoError(t, err)
	assessmentRepo := assessmentmysql.NewAssessmentRepository(db)
	runRepo := checkpoint.NewRunRepository(db)
	runner := NewMySQLRunner(db)
	intake := appintake.NewService(assessmentRepo, proofModelValidator{}, runner, stager)
	engine := appexecute.NewEngine(assessmentRepo, m5UnavailableInput{},
		appexecute.WithRunRepository(runRepo), appexecute.WithTransactionalOutbox(runner, stager))

	createSubmitted := func(answerSheetID uint64) uint64 {
		kind, code, version := "scale", "MODEL-1", "1.0.0"
		created, createErr := intake.CreateForAnswerSheet(ctx, appintake.CreateCommand{
			OrgID: 1, TesteeID: 2, AnswerSheetID: answerSheetID,
			QuestionnaireCode: "Q-001", QuestionnaireVersion: "v1", OriginType: "adhoc",
			ModelKind: &kind, ModelCode: &code, ModelVersion: &version,
		})
		require.NoError(t, createErr)
		_, submitErr := intake.SubmitForEvaluation(ctx, created.ID)
		require.NoError(t, submitErr)
		return created.ID
	}

	firstID := createSubmitted(601)
	require.Error(t, engine.Evaluate(ctx, firstID), "the controlled input outage must reach the failure finalizer")
	first, err := assessmentRepo.FindByID(ctx, meta.FromUint64(firstID))
	require.NoError(t, err)
	require.True(t, first.Status().IsFailed())
	failedRun, err := runRepo.FindLatestByAssessmentID(ctx, firstID)
	require.NoError(t, err)
	require.NotNil(t, failedRun)
	require.Equal(t, "failed", failedRun.Attempt().Status.String())
	decision := failedRun.RetryDecision()
	require.NotNil(t, decision)
	require.Equal(t, retrygovernance.DispositionAutomatic, decision.Disposition)
	require.NotNil(t, decision.NextAttemptAt)
	require.NotEmpty(t, decision.RetryEventID)
	var rows []struct {
		EventType     string
		MessageID     string
		NextAttemptAt time.Time
		State         string
	}
	require.NoError(t, db.Table("rm_outbox").Select("event_type,message_id,next_attempt_at,state").Order("id").Scan(&rows).Error)
	require.Len(t, rows, 3)
	require.Equal(t, "evaluation.requested", rows[0].EventType)
	require.Equal(t, "evaluation.failed", rows[1].EventType)
	require.Equal(t, "evaluation.retry.requested", rows[2].EventType)
	require.Equal(t, decision.RetryEventID, rows[2].MessageID)
	require.WithinDuration(t, *decision.NextAttemptAt, rows[2].NextAttemptAt, 2*time.Millisecond)
	require.Greater(t, rows[2].NextAttemptAt.Sub(time.Now()), 15*time.Second)
	require.Equal(t, "pending", rows[2].State)
	store, err := sdkmysql.New(sqlDB)
	require.NoError(t, err)
	claimable, err := store.ClaimDue(ctx, 10, time.Minute)
	require.NoError(t, err)
	require.Len(t, claimable, 2, "the scheduled retry must not be claimed before its due time")
	for _, claim := range claimable {
		require.NotEqual(t, "evaluation.retry.requested", claim.Message.Input().EventType)
		require.NoError(t, store.Confirm(ctx, claim))
	}

	secondID := createSubmitted(602)
	require.NoError(t, db.Exec(`CREATE TRIGGER rm_reject_m5_retry BEFORE INSERT ON rm_outbox FOR EACH ROW BEGIN IF NEW.event_type = 'evaluation.retry.requested' THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'controlled retry insert failure'; END IF; END`).Error)
	defer db.Exec("DROP TRIGGER IF EXISTS rm_reject_m5_retry")
	require.Error(t, engine.Evaluate(ctx, secondID))
	second, err := assessmentRepo.FindByID(ctx, meta.FromUint64(secondID))
	require.NoError(t, err)
	require.True(t, second.Status().IsSubmitted(), "failure fact must roll back when retry intent cannot be saved")
	claim, err := runRepo.FindLatestByAssessmentID(ctx, secondID)
	require.NoError(t, err)
	require.NotNil(t, claim)
	require.Equal(t, "running", claim.Attempt().Status.String(), "the earlier execution claim remains for recovery")
	var count int64
	require.NoError(t, db.Table("rm_outbox").Count(&count).Error)
	require.EqualValues(t, 4, count, "only two requested events and the first committed failure/retry pair remain")
}
