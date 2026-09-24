//go:build integration && reliable_messaging && reliable_messaging_m4 && reliable_messaging_m5

package transaction

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/messaging"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	outcomecommit "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/commit"
	outcomescoring "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/scoring"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evalrun "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/run"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	"github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/checkpoint"
	assessmentmysql "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	mysqlstandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationrun"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// A canonical Outcome, score projection, Assessment/Run completion and one
// standard intent share the host MySQL transaction. Rejecting the intent must
// leave the prior submitted Assessment and running claim recoverable.
func TestM5StandardEvaluationOutcomeOriginalTransaction(t *testing.T) {
	dsn := os.Getenv("RM_QS_M5_OUTCOME_DSN")
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil || parsed.Net != "tcp" || parsed.Addr != "mysql:3306" || parsed.DBName != "m5_qs_outcome" {
		t.Fatal("disposable m5_qs_outcome MySQL required")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer sqlDB.Close()
	require.NoError(t, db.AutoMigrate(
		&assessmentmysql.AssessmentPO{}, &assessmentmysql.AssessmentScorePO{},
		&assessmentmysql.EvaluationOutcomePO{}, &checkpoint.RuntimeCheckpointPO{},
	))
	_, err = sqlDB.ExecContext(ctx, sdkmysql.Schema)
	require.NoError(t, err)
	config, err := eventcatalog.Parse([]byte(`version: "1"
topics:
  evaluation:
    name: qs.evaluation.lifecycle
events:
  evaluation.outcome.committed:
    topic: evaluation
    delivery: durable_outbox
    aggregate: Evaluation
    domain: evaluation
    handler: evaluation_outcome_committed_handler
`))
	require.NoError(t, err)
	stager, err := mysqlstandard.NewStager(eventcatalog.NewCatalog(config), eventruntime.SourceAPIServer)
	require.NoError(t, err)
	assessmentRepo := assessmentmysql.NewAssessmentRepository(db)
	outcomeRepo := assessmentmysql.NewOutcomeRepository(db)
	runRepo := checkpoint.NewRunRepository(db)
	committer := outcomecommit.NewCommitter(
		NewMySQLRunner(db), assessmentRepo, outcomeRepo, runRepo,
		outcomescoring.NewAssessmentScoreProjector(assessmentmysql.NewScoreRepository(db)), stager, nil,
	)

	prepare := func(assessmentID, answerSheetID uint64) (*assessment.Assessment, evalrun.EvaluationRun, *domainoutcome.Execution) {
		modelRef := assessment.NewScaleEvaluationModelRef(meta.ZeroID, meta.NewCode("SCALE-1"), "1.0.0", "scale")
		a, newErr := assessment.NewAssessment(
			1, testee.NewID(1001), assessment.NewQuestionnaireRefByCode(meta.NewCode("Q-1"), "1.0.0"),
			assessment.NewAnswerSheetRef(meta.FromUint64(answerSheetID)), assessment.NewAdhocOrigin(),
			assessment.WithID(assessment.NewID(assessmentID)), assessment.WithEvaluationModel(modelRef),
		)
		require.NoError(t, newErr)
		require.NoError(t, a.Submit())
		a.ClearEvents()
		// The production repository treats an assigned ID as an update. Seed
		// this pre-existing submitted Assessment through its persistence mapper.
		require.NoError(t, db.Create(assessmentmysql.NewAssessmentMapper().ToPO(a)).Error)
		at := time.Now()
		claimed, claimErr := runRepo.Claim(ctx, evaluationrun.ClaimRequest{
			AssessmentID: assessmentID, Token: "m5-outcome-claim", ClaimedAt: at, LeaseUntil: at.Add(time.Minute),
		})
		require.NoError(t, claimErr)
		require.True(t, claimed.Claimed)
		run := claimed.Run
		require.NoError(t, run.AttachInputSnapshot("isn:v2:"+strings.Repeat("a", 64)))
		require.NoError(t, runRepo.SaveClaimed(ctx, run))
		execution := domainoutcome.NewExecution(
			evaloutcome.ModelRefFromAssessment(modelRef), domainoutcome.Summary{PrimaryLabel: "high"},
			domainoutcome.Detail{Kind: modelcatalog.KindScale},
		)
		execution.Primary = &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: 12}
		execution.Level = &domainoutcome.ResultLevel{Code: "high", Label: "高风险", Severity: "high"}
		execution.Dimensions = []domainoutcome.DimensionResult{{
			Code: "total", Name: "总分", Kind: domainoutcome.DimensionKindFactor, Role: "total",
			Score: &domainoutcome.ScoreValue{Kind: domainoutcome.ScoreKindRawTotal, Value: 12},
			Level: &domainoutcome.ResultLevel{Code: "high", Label: "高风险", Severity: "high"},
		}}
		return a, run, execution
	}
	input := &evaluationinput.InputSnapshot{
		Model: &evaluationinput.ModelSnapshot{
			Kind: evaluationinput.EvaluationModelKindScale, Algorithm: string(modelcatalog.AlgorithmScaleDefault),
			DecisionKind: string(modelcatalog.DecisionKindScoreRange), Code: "SCALE-1", Version: "1.0.0",
		},
		DefinitionV2: &modeldefinition.Definition{
			Measure:              modeldefinition.MeasureSpec{Factors: []factor.Factor{{Code: "total", Title: "总分", Role: factor.FactorRoleTotal}}},
			InterpretationAssets: interpretationassets.Assets{Outcomes: []interpretationassets.OutcomePresentation{{OutcomeCode: "high", Title: "高风险"}}},
		},
	}
	request := func(a *assessment.Assessment, run *evalrun.EvaluationRun, execution *domainoutcome.Execution) outcomecommit.CommitRequest {
		return outcomecommit.CommitRequest{
			Assessment: a, Input: input, Execution: execution,
			DescriptorKey: evalpipeline.DescriptorKey{DecisionKind: modelcatalog.DecisionKindScoreRange},
			OutcomePolicy: evalpipeline.DefaultOutcomeCompletenessPolicy(modelcatalog.DecisionKindScoreRange),
			Run:           run, EvaluatedAt: time.Now(),
		}
	}
	count := func(table, column string, id uint64) int64 {
		var n int64
		require.NoError(t, db.Table(table).Where(column+" = ?", id).Count(&n).Error)
		return n
	}

	first, firstRun, firstExecution := prepare(5001, 2001)
	committed, err := committer.Commit(ctx, request(first, &firstRun, firstExecution))
	require.NoError(t, err)
	require.NotNil(t, committed)
	require.True(t, first.Status().IsEvaluated())
	require.Equal(t, evalrun.StatusSucceeded, firstRun.Attempt().Status)
	require.EqualValues(t, 1, count("evaluation_outcome", "assessment_id", 5001))
	require.Positive(t, count("assessment_score", "assessment_id", 5001))
	var score assessmentmysql.AssessmentScorePO
	require.NoError(t, db.Where("assessment_id = ?", 5001).First(&score).Error)
	require.Equal(t, 12.0, score.RawScore)
	require.NotNil(t, score.EvaluationOutcomeID)
	require.Equal(t, committed.ID().Uint64(), *score.EvaluationOutcomeID)
	persisted, err := outcomeRepo.FindByAssessmentID(ctx, meta.FromUint64(5001))
	require.NoError(t, err)
	require.NotNil(t, persisted)
	require.Equal(t, committed.ID(), persisted.ID())
	require.Equal(t, evaluationinput.CurrentReportInputSchema, evaluationinput.ReportInputSchema(persisted.ReportInput()))
	persistedAssessment, err := assessmentRepo.FindByID(ctx, meta.FromUint64(5001))
	require.NoError(t, err)
	require.True(t, persistedAssessment.Status().IsEvaluated())
	persistedRun, err := runRepo.FindLatestByAssessmentID(ctx, 5001)
	require.NoError(t, err)
	require.Equal(t, evalrun.StatusSucceeded, persistedRun.Attempt().Status)
	store, err := sdkmysql.New(sqlDB)
	require.NoError(t, err)
	claims, err := store.ClaimDue(ctx, 10, time.Minute)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	messageInput := claims[0].Message.Input()
	require.Equal(t, "evaluation.outcome.committed", messageInput.EventType)
	require.Equal(t, "org:1", messageInput.Scope)
	require.Equal(t, "qs.evaluation.lifecycle", messageInput.Destination)
	decoded, recognized, err := messaging.DecodeMessagePayload(messageInput.Payload)
	require.NoError(t, err)
	require.True(t, recognized)
	require.Equal(t, messageInput.ID, decoded.UUID)
	require.Equal(t, "evaluation.outcome.committed", decoded.Metadata["event_type"])
	var domainEvent struct {
		Data eventpayload.EvaluationOutcomeCommittedData `json:"data"`
	}
	require.NoError(t, json.Unmarshal(decoded.Payload, &domainEvent))
	require.EqualValues(t, 1, domainEvent.Data.OrgID)
	require.EqualValues(t, 5001, domainEvent.Data.AssessmentID)
	require.Equal(t, committed.ID().String(), domainEvent.Data.OutcomeID)
	require.Equal(t, firstRun.ID().String(), domainEvent.Data.EvaluationRunID)
	require.NoError(t, store.Confirm(ctx, claims[0]))

	second, secondRun, secondExecution := prepare(5002, 2002)
	require.NoError(t, db.Exec(`CREATE TRIGGER rm_reject_m5_outcome BEFORE INSERT ON rm_outbox FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'controlled outcome intent failure'`).Error)
	defer db.Exec("DROP TRIGGER IF EXISTS rm_reject_m5_outcome")
	failedResult, err := committer.Commit(ctx, request(second, &secondRun, secondExecution))
	require.Error(t, err)
	require.Nil(t, failedResult)
	require.True(t, second.Status().IsSubmitted(), "caller Assessment must retain its submitted state")
	require.Equal(t, evalrun.StatusRunning, secondRun.Attempt().Status, "caller Run must retain its active claim")
	require.Zero(t, count("evaluation_outcome", "assessment_id", 5002))
	require.Zero(t, count("assessment_score", "assessment_id", 5002))
	secondPersisted, err := assessmentRepo.FindByID(ctx, meta.FromUint64(5002))
	require.NoError(t, err)
	require.True(t, secondPersisted.Status().IsSubmitted())
	secondPersistedRun, err := runRepo.FindLatestByAssessmentID(ctx, 5002)
	require.NoError(t, err)
	require.Equal(t, evalrun.StatusRunning, secondPersistedRun.Attempt().Status)
	var outboxCount int64
	require.NoError(t, db.Table("rm_outbox").Count(&outboxCount).Error)
	require.EqualValues(t, 1, outboxCount, "the rejected second intent must leave no durable row")
}
