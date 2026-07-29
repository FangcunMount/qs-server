//go:build integration

package historicalbackfill

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/messaging"
	answersheetpb "github.com/FangcunMount/qs-server/api/grpc/gen/answersheet"
	evaluationpb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	interpretationpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	assessmentintakejourney "github.com/FangcunMount/qs-server/internal/apiserver/application/journey/assessmentintake"
	planapp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	statisticsapp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	statisticsmodule "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/statistics"
	actorDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	modelDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelFactor "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	statisticsDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	questionnaireDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	mongoModelCatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	mongoQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/questionnaire"
	mysqlActor "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	historicalstageinfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/historicalseedstage"
	rulesetInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset"
	stageport "github.com/FangcunMount/qs-server/internal/apiserver/port/historicalseedstage"
	modelport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	grpctransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc"
	grpcservice "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc/service"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/historicalseed"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	migrationpkg "github.com/FangcunMount/qs-server/internal/pkg/migration"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	locksubsystem "github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease/subsystem"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	drivermysql "github.com/go-sql-driver/mysql"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

const (
	historicalClosureBatch    = "hist-e2e-production-closure"
	historicalClosureScenario = "2026-07-27/1/submit_answer/scale-e2e/task-1"
	historicalClosureModel    = "SCALE_HIST_E2E"
	historicalClosureQNR      = "Q_HIST_E2E"
	historicalClosureVersion  = "1.0.0"
)

// TestHistoricalBackfillProductionClosure is the required cross-storage proof
// for the historical backfill path. It deliberately uses the real container,
// durable outboxes and worker handlers; only the external MQ transport is
// replaced by an in-memory capture at the process boundary.
func TestHistoricalBackfillProductionClosure(t *testing.T) {
	mysqlDSN := os.Getenv("MYSQL_DSN")
	if mysqlDSN == "" {
		t.Fatal("MYSQL_DSN is required; the production-closure integration test must not skip")
	}
	redisURL := os.Getenv("QS_SERVER_TEST_REDIS_URL")
	if redisURL == "" {
		t.Fatal("QS_SERVER_TEST_REDIS_URL is required; the production-closure integration test must not skip")
	}

	sqlDB, databaseName := openHistoricalClosureDatabase(t, mysqlDSN)
	mysqlMigrator := migrationpkg.NewMigrator(sqlDB, &migrationpkg.Config{Enabled: true, Database: databaseName})
	version, changed, err := mysqlMigrator.Run()
	if err != nil || !changed || version == 0 {
		t.Fatalf("migrate empty MySQL: version=%d changed=%v err=%v", version, changed, err)
	}
	if repeatedVersion, repeatedChanged, repeatErr := mysqlMigrator.Run(); repeatErr != nil || repeatedChanged || repeatedVersion != version {
		t.Fatalf("empty MySQL did not reach embedded latest migration: first=%d repeated=%d changed=%v err=%v", version, repeatedVersion, repeatedChanged, repeatErr)
	}
	gormDB, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	mongoClient, mongoDB := mongodbtest.ReplicaSetDatabase(t)
	mongoMigrator := migrationpkg.NewMongoMigrator(mongoClient, &migrationpkg.Config{Enabled: true, Database: mongoDB.Name()})
	if version, changed, err := mongoMigrator.Run(); err != nil || !changed || version == 0 {
		t.Fatalf("migrate empty MongoDB: version=%d changed=%v err=%v", version, changed, err)
	} else if repeatedVersion, repeatedChanged, repeatErr := mongoMigrator.Run(); repeatErr != nil || repeatedChanged || repeatedVersion != version {
		t.Fatalf("empty MongoDB did not reach embedded latest migration: first=%d repeated=%d changed=%v err=%v", version, repeatedVersion, repeatedChanged, repeatErr)
	}

	redisOptions, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatalf("parse QS_SERVER_TEST_REDIS_URL: %v", err)
	}
	redisClient := redis.NewClient(redisOptions)
	t.Cleanup(func() { _ = redisClient.Close() })
	if err := redisClient.Ping(t.Context()).Err(); err != nil {
		t.Fatalf("ping test Redis: %v", err)
	}
	if err := redisClient.FlushDB(t.Context()).Err(); err != nil {
		t.Fatalf("flush dedicated test Redis DB: %v", err)
	}
	t.Cleanup(func() { _ = redisClient.FlushDB(context.Background()).Err() })

	events, err := eventcatalog.Load("../../../../configs/events.yaml")
	if err != nil {
		t.Fatalf("load event catalog: %v", err)
	}
	capture := newCapturedMQPublisher()
	eventing, err := eventsubsystem.New(eventsubsystem.Options{
		MySQLDB: gormDB, MongoDB: mongoDB, OpsRedis: redisClient,
		Catalog: eventcatalog.NewCatalog(events), MQPublisher: capture, PublisherMode: eventruntime.PublishModeMQ,
		Mongo:      eventsubsystem.ProfileOptions{BatchSize: 20, PublishWorkers: 1, ImmediateMaxConcurrent: 1},
		Assessment: eventsubsystem.ProfileOptions{BatchSize: 20, PublishWorkers: 1, ImmediateMaxConcurrent: 1},
		Consumers: map[string]eventsubsystem.ConsumerOptions{
			"modelcatalog.hot_rank_projection": {Enabled: false},
		},
	})
	if err != nil {
		t.Fatalf("build event subsystem: %v", err)
	}
	lockHandle := &redisruntime.Handle{
		Family: redisruntime.FamilyLock, Client: redisClient,
		Builder:    keyspace.NewBuilderWithNamespace("test:historical-backfill-e2e:lock"),
		Configured: true, Available: true,
	}
	locks := locksubsystem.New(locksubsystem.Options{Component: "apiserver", Handle: lockHandle})
	workerLockHandle := &redisruntime.Handle{
		Family: redisruntime.FamilyLock, Client: redisClient,
		Builder:    keyspace.NewBuilderWithNamespace("test:historical-backfill-e2e:worker-lock"),
		Configured: true, Available: true,
	}
	workerLocks := locksubsystem.New(locksubsystem.Options{Component: "worker", Handle: workerLockHandle})
	c := container.NewContainerWithOptions(gormDB, mongoDB, redisClient, container.ContainerOptions{EventSubsystem: eventing, LockSubsystem: locks, Silent: true})
	if err := c.Initialize(); err != nil {
		t.Fatalf("initialize real application container: %v", err)
	}
	realStatistics, err := statisticsmodule.New(statisticsmodule.Deps{
		MySQLDB: gormDB, MongoDB: mongoDB, RedisClient: redisClient, LockRunner: locks,
	})
	if err != nil {
		t.Fatalf("build Statistics module with real Redis publisher: %v", err)
	}
	c.SetStatisticsModule(realStatistics)
	if err := c.StartEventSubsystem(t.Context()); err != nil {
		t.Fatalf("start real durable outbox relays: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Cleanup(); err != nil {
			t.Errorf("cleanup container: %v", err)
		}
	})

	seedHistoricalClosureCatalog(t, mongoDB)
	orgID := uint64(820000 + time.Now().UnixNano()%10000)
	testee := actorDomain.NewTestee(int64(orgID), "historical-closure-testee", actorDomain.GenderUnknown, nil)
	if err := mysqlActor.NewTesteeRepository(gormDB).Save(t.Context(), testee); err != nil {
		t.Fatalf("seed Testee: %v", err)
	}

	historical, verifier := signedHistoricalClosureContext(t, orgID)
	parentHistorical := historical.Clone()
	parentHistorical.ScenarioID = "2026-07-27/1/submit_answer/scale-e2e"
	parentCtx := historicalseed.WithContext(t.Context(), parentHistorical)
	childCtx := historicalseed.WithContext(t.Context(), historical)

	planResult, err := c.PlanModule.CommandService.CreatePlan(t.Context(), planapp.CreatePlanDTO{
		OrgID: int64(orgID), ScaleCode: historicalClosureModel, ScheduleType: "by_day", TriggerTime: "09:00:00", Interval: 1, TotalTimes: 1,
	})
	if err != nil {
		t.Fatalf("create Plan: %v", err)
	}
	enrollment, err := c.PlanModule.CommandService.EnrollTestee(parentCtx, planapp.EnrollTesteeDTO{
		OrgID: int64(orgID), PlanID: planResult.ID, TesteeID: testee.ID().String(), StartDate: "2026-07-27",
	})
	if err != nil || len(enrollment.Tasks) != 1 {
		t.Fatalf("historical Plan enrollment: result=%+v err=%v", enrollment, err)
	}
	taskID := enrollment.Tasks[0].ID
	opened, err := c.PlanModule.CommandService.OpenTask(childCtx, int64(orgID), taskID)
	if err != nil || opened == nil || opened.Status != "opened" {
		t.Fatalf("historical OpenTask: result=%+v err=%v", opened, err)
	}

	grpcDeps := c.BuildGRPCDeps(nil)
	answerService := grpcservice.NewAnswerSheetService(grpcDeps.Survey.AnswerSheetSubmissionService, grpcDeps.Survey.AnswerSheetManagementService, grpcDeps.HistoricalStageRecorder)
	answerService.SetHistoricalSeedVerifier(verifier)
	assessmentService := newHistoricalAssessmentIntakeService(grpcDeps, verifier)
	evaluationService := grpcservice.NewEvaluationWorkerService(grpcDeps.Evaluation.WorkerService)
	evaluationService.SetHistoricalSeedVerifier(verifier)
	interpretationService := grpcservice.NewInterpretationAutomationService(grpcDeps.Interpretation.AutomationService, grpcDeps.HistoricalStageRecorder)
	interpretationService.SetHistoricalSeedVerifier(verifier)

	workerDeps := &handlers.Dependencies{
		Logger:                         slog.New(slog.NewTextHandler(io.Discard, nil)),
		AssessmentIntakeClient:         assessmentService,
		EvaluationWorkerClient:         evaluationWorkerAdapter{service: evaluationService},
		InterpretationAutomationClient: interpretationWorkerAdapter{service: interpretationService},
		ReportStatusReporter:           grpcDeps.Interpretation.ReportStatusReporter,
		LockManager:                    workerLocks,
		LockRunner:                     workerLocks,
		LockKeyBuilder:                 workerLocks.Builder(),
	}
	registry := handlers.NewRegistry()
	answerHandler := mustWorkerHandler(t, registry, "answersheet_submitted_handler", workerDeps)
	evaluationHandler := mustWorkerHandler(t, registry, "evaluation_requested_handler", workerDeps)
	outcomeHandler := mustWorkerHandler(t, registry, "evaluation_outcome_committed_handler", workerDeps)
	reportHandler := mustWorkerHandler(t, registry, "interpretation_report_generated_handler", workerDeps)

	request := &answersheetpb.SaveAnswerSheetRequest{
		QuestionnaireCode: historicalClosureQNR, QuestionnaireVersion: historicalClosureVersion,
		IdempotencyKey: "hist-e2e-answer-0001", WriterId: testee.ID().Uint64(), TesteeId: testee.ID().Uint64(), OrgId: orgID,
		TaskId: taskID, OriginRef: &answersheetpb.OriginRef{Type: "plan_task", Id: taskID},
		Answers:           []*answersheetpb.Answer{{QuestionCode: "Q1", QuestionType: "Radio", Value: "A"}},
		HistoricalContext: historicalseed.ToProto(historical),
	}
	answerResponse, err := answerService.SaveAnswerSheet(t.Context(), request)
	if err != nil || answerResponse.GetId() == 0 {
		t.Fatalf("submit historical AnswerSheet: response=%+v err=%v", answerResponse, err)
	}
	answerMessage := capture.Wait(t, eventcatalog.AnswerSheetSubmitted)
	if err := answerHandler(t.Context(), eventcatalog.AnswerSheetSubmitted, answerMessage.Payload); err != nil {
		t.Fatalf("consume answersheet.submitted: %v", err)
	}

	evaluationMessage := capture.Wait(t, eventcatalog.EvaluationRequested)
	if err := evaluationHandler(t.Context(), eventcatalog.EvaluationRequested, evaluationMessage.Payload); err != nil {
		t.Fatalf("consume evaluation.requested: %v", err)
	}
	outcomeMessage := capture.Wait(t, eventcatalog.EvaluationOutcomeCommitted)

	triggerName := fmt.Sprintf("fail_report_stage_%d", time.Now().UnixNano())
	if err := gormDB.Exec("CREATE TRIGGER `" + triggerName + "` BEFORE INSERT ON seed_backfill_stage FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'injected report stage failure'").Error; err != nil {
		t.Fatalf("install report-stage failure trigger: %v", err)
	}
	if err := outcomeHandler(t.Context(), eventcatalog.EvaluationOutcomeCommitted, outcomeMessage.Payload); err == nil {
		t.Fatal("first report worker attempt unexpectedly succeeded")
	}
	if err := gormDB.Exec("DROP TRIGGER `" + triggerName + "`").Error; err != nil {
		t.Fatalf("drop report-stage failure trigger: %v", err)
	}
	if err := outcomeHandler(t.Context(), eventcatalog.EvaluationOutcomeCommitted, outcomeMessage.Payload); err != nil {
		t.Fatalf("retry report worker after cross-storage failure: %v", err)
	}
	reportMessage := capture.Wait(t, eventcatalog.InterpretationReportGenerated)
	if err := reportHandler(t.Context(), eventcatalog.InterpretationReportGenerated, reportMessage.Payload); err != nil {
		t.Fatalf("consume interpretation.report.generated: %v", err)
	}

	stages := assertHistoricalClosureStages(t, gormDB, orgID, parentHistorical, historical, taskID, answerResponse.GetId())
	assessmentID := stages[stageport.StageAssessmentCreated].ResourceID
	assertFailedThenCompletedReportAttempts(t, gormDB, orgID, historical)

	replayedAnswer, err := answerService.SaveAnswerSheet(t.Context(), request)
	if err != nil || replayedAnswer.GetId() != answerResponse.GetId() {
		t.Fatalf("replay AnswerSheet: first=%d replay=%+v err=%v", answerResponse.GetId(), replayedAnswer, err)
	}
	if err := answerHandler(t.Context(), eventcatalog.AnswerSheetSubmitted, answerMessage.Payload); err != nil {
		t.Fatalf("replay answersheet worker: %v", err)
	}
	if _, err := c.PlanModule.CommandService.OpenTask(childCtx, int64(orgID), taskID); err != nil {
		t.Fatalf("replay OpenTask: %v", err)
	}
	if _, err := c.PlanModule.CommandService.CompleteTask(childCtx, int64(orgID), taskID, assessmentID); err != nil {
		t.Fatalf("replay CompleteTask: %v", err)
	}
	if err := evaluationHandler(t.Context(), eventcatalog.EvaluationRequested, evaluationMessage.Payload); err != nil {
		t.Fatalf("replay evaluation worker: %v", err)
	}
	if err := outcomeHandler(t.Context(), eventcatalog.EvaluationOutcomeCommitted, outcomeMessage.Payload); err != nil {
		t.Fatalf("replay report worker: %v", err)
	}

	assertHistoricalClosureUniqueFacts(t, gormDB, mongoDB, orgID, answerResponse.GetId(), stages)
	assertHistoricalClosureOutboxes(t, gormDB, mongoDB)
	runHistoricalClosureStatistics(t, c, orgID)
}

func openHistoricalClosureDatabase(t *testing.T, dsn string) (*sql.DB, string) {
	t.Helper()
	cfg, err := drivermysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	gotLocation := "<nil>"
	if cfg.Loc != nil {
		gotLocation = cfg.Loc.String()
	}
	if gotLocation != statisticsDomain.Shanghai.String() {
		t.Fatalf(
			"MYSQL_DSN loc=%q, want %q for the Shanghai business-date contract",
			gotLocation,
			statisticsDomain.Shanghai.String(),
		)
	}
	databaseName := fmt.Sprintf("qs_historical_closure_%d", time.Now().UnixNano())
	cfg.DBName = ""
	cfg.MultiStatements = true
	server, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.ExecContext(t.Context(), "CREATE DATABASE `"+databaseName+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		_ = server.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = server.ExecContext(context.Background(), "DROP DATABASE IF EXISTS `"+databaseName+"`")
		_ = server.Close()
	})

	cfg.DBName = databaseName
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, databaseName
}

func seedHistoricalClosureCatalog(t *testing.T, mongoDB *mongo.Database) {
	t.Helper()
	question, err := questionnaireDomain.NewQuestion(
		questionnaireDomain.WithCode(meta.NewCode("Q1")), questionnaireDomain.WithStem("Historical E2E question"),
		questionnaireDomain.WithQuestionType(questionnaireDomain.TypeRadio), questionnaireDomain.WithOption("A", "A", 1), questionnaireDomain.WithOption("B", "B", 0), questionnaireDomain.WithRequired(),
	)
	if err != nil {
		t.Fatal(err)
	}
	publishedAt := time.Date(2026, 7, 27, 7, 30, 0, 0, statisticsDomain.Shanghai)
	questionnaire, err := questionnaireDomain.NewQuestionnaire(meta.NewCode(historicalClosureQNR), "Historical E2E questionnaire",
		questionnaireDomain.WithVersion(questionnaireDomain.Version(historicalClosureVersion)), questionnaireDomain.WithStatus(questionnaireDomain.STATUS_PUBLISHED),
		questionnaireDomain.WithRecordRole(questionnaireDomain.RecordRolePublishedSnapshot), questionnaireDomain.WithActivePublished(true), questionnaireDomain.WithReleaseStatus(questionnaireDomain.ReleaseStatusActive),
		questionnaireDomain.WithPublishedAt(&publishedAt), questionnaireDomain.WithQuestions([]questionnaireDomain.Question{question}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := mongoQuestionnaire.NewRepository(mongoDB).CreatePublishedSnapshot(t.Context(), questionnaire, true); err != nil {
		t.Fatalf("seed published questionnaire: %v", err)
	}
	model := &modelport.PublishedModel{
		SchemaVersion: modelDomain.SchemaVersionV2, ProductChannel: modelDomain.ProductChannelMedicalScale,
		Kind: modelDomain.KindScale, Algorithm: modelDomain.AlgorithmScaleDefault, AlgorithmFamily: modelDomain.AlgorithmFamilyFactorScoring,
		Code: historicalClosureModel, Version: historicalClosureVersion, Title: "Historical E2E scale", Status: "published", ReleaseStatus: modelDomain.ReleaseStatusActive,
		DecisionKind: modelDomain.DecisionKindScoreRange, QuestionnaireCode: historicalClosureQNR, QuestionnaireVersion: historicalClosureVersion,
		DefinitionV2: &modelDomain.Definition{
			Measure:     modelDomain.MeasureSpec{Factors: []modelDomain.Factor{{Code: "TOTAL", Title: "Total", Role: modelFactor.FactorRoleTotal}}, Scoring: []modelDomain.Scoring{{FactorCode: "TOTAL", Strategy: modelFactor.ScoringStrategySum, Sources: []modelDomain.ScoringSource{{Kind: modelDomain.ScoringSourceQuestion, Code: "Q1"}}}}},
			Outcomes:    []modelDomain.Outcome{{Code: "low", Title: "Low"}},
			Conclusions: []modelDomain.Conclusion{modelDomain.RiskConclusion{FactorCode: "TOTAL", Rules: []modelDomain.ScoreRangeOutcome{{MinScore: 0, MaxScore: 10, MaxInclusive: true, OutcomeCode: "low", Level: "low"}}}},
		},
	}
	if err := mongoModelCatalog.NewRepository(mongoDB).UpsertPublishedModel(t.Context(), model); err != nil {
		t.Fatalf("seed published model: %v", err)
	}
}

func signedHistoricalClosureContext(t *testing.T, orgID uint64) (historicalseed.Context, *historicalseed.Verifier) {
	t.Helper()
	location := statisticsDomain.Shanghai
	filled := time.Date(2026, 7, 27, 10, 0, 0, 0, location)
	created, submitted := filled.Add(time.Second), filled.Add(2*time.Second)
	completed, evaluated, reported := filled.Add(3*time.Second), filled.Add(5*time.Second), filled.Add(8*time.Second)
	joined, opened := filled.Add(-time.Hour), filled.Add(-30*time.Minute)
	historical := historicalseed.Context{BatchID: historicalClosureBatch, ScenarioID: historicalClosureScenario, OrgID: orgID, Version: historicalseed.Version1, Timeline: historicalseed.Timeline{
		EnrollmentJoinedAt: &joined, TaskOpenedAt: &opened, AnswerSheetFilledAt: &filled,
		AssessmentCreatedAt: &created, AssessmentSubmittedAt: &submitted, TaskCompletedAt: &completed, EvaluatedAt: &evaluated, ReportGeneratedAt: &reported,
	}}
	now := time.Date(2026, 7, 29, 10, 0, 0, 0, location)
	secret := []byte("historical-e2e-signature-secret")
	verifier := &historicalseed.Verifier{
		Enabled: true, Secret: secret, AllowedOrgIDs: map[uint64]struct{}{orgID: {}},
		Earliest: time.Date(2025, 1, 1, 0, 0, 0, 0, location), Latest: time.Date(2026, 7, 27, 0, 0, 0, 0, location),
		Location: location, MaxSkew: 5 * time.Minute, Now: func() time.Time { return now },
	}
	encoded, err := historicalseed.Encode(historical)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"questionnaire_code":"Q_HIST_E2E","idempotency_key":"hist-e2e-answer-0001"}`)
	requestedAt := now.Format(time.RFC3339Nano)
	headers := historicalseed.Headers{EncodedContext: encoded, RequestedAt: requestedAt}
	headers.Signature = historicalseed.Sign("POST", "/internal/answersheets", body, requestedAt, encoded, secret)
	verified, present, err := verifier.Verify("POST", "/internal/answersheets", body, headers)
	if err != nil || !present {
		t.Fatalf("verify signed Historical Context: present=%v err=%v", present, err)
	}
	return verified, verifier
}

type evaluationWorkerAdapter struct {
	service *grpcservice.EvaluationWorkerService
}

func (a evaluationWorkerAdapter) ExecuteEvaluation(ctx context.Context, assessmentID uint64) (*evaluationpb.ExecuteEvaluationResponse, error) {
	historical, _ := historicalseed.FromContext(ctx)
	return a.service.ExecuteEvaluation(ctx, &evaluationpb.ExecuteEvaluationRequest{AssessmentId: assessmentID, HistoricalContext: historicalseed.ToProto(historical)})
}

type interpretationWorkerAdapter struct {
	service *grpcservice.InterpretationAutomationService
}

func (a interpretationWorkerAdapter) GenerateReportFromOutcome(ctx context.Context, outcomeID string) (*interpretationpb.GenerateReportFromAssessmentResponse, error) {
	historical, _ := historicalseed.FromContext(ctx)
	return a.service.GenerateReportFromOutcome(ctx, &interpretationpb.GenerateReportFromOutcomeRequest{OutcomeId: outcomeID, HistoricalContext: historicalseed.ToProto(historical)})
}

func newHistoricalAssessmentIntakeService(deps grpctransport.Deps, verifier *historicalseed.Verifier) *grpcservice.AssessmentIntakeService {
	journey := assessmentintakejourney.NewService(
		deps.Survey.AnswerSheetScoringService,
		rulesetInfra.NewAssessmentBindingResolver(deps.PublishedModelCatalog),
		deps.Plan.TaskAssessmentResolver,
		deps.Plan.CommandService,
		deps.Evaluation.IntakeService,
		deps.Interpretation.ReportStatusReporter,
	)
	service := grpcservice.NewAssessmentIntakeService(journey, deps.Evaluation.IntakeService, deps.Survey.AnswerSheetManagementService)
	service.SetHistoricalSeedVerifier(verifier)
	return service
}

func mustWorkerHandler(t *testing.T, registry *handlers.Registry, name string, deps *handlers.Dependencies) handlers.HandlerFunc {
	t.Helper()
	handler, ok := registry.Create(name, deps)
	if !ok {
		t.Fatalf("worker handler %q is not registered", name)
	}
	return handler
}

type capturedMQPublisher struct {
	mu       sync.Mutex
	messages []*messaging.Message
}

func newCapturedMQPublisher() *capturedMQPublisher { return &capturedMQPublisher{} }

func (p *capturedMQPublisher) Publish(_ context.Context, _ string, _ []byte) error { return nil }

func (p *capturedMQPublisher) PublishMessage(_ context.Context, topic string, message *messaging.Message) error {
	copyMessage := &messaging.Message{UUID: message.UUID, Topic: topic, Payload: append([]byte(nil), message.Payload...), Metadata: make(map[string]string, len(message.Metadata))}
	for key, value := range message.Metadata {
		copyMessage.Metadata[key] = value
	}
	p.mu.Lock()
	p.messages = append(p.messages, copyMessage)
	p.mu.Unlock()
	return nil
}

func (p *capturedMQPublisher) Close() error { return nil }

func (p *capturedMQPublisher) Wait(t *testing.T, eventType string) *messaging.Message {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		p.mu.Lock()
		for index, message := range p.messages {
			if message.Metadata["event_type"] == eventType {
				p.messages = append(p.messages[:index], p.messages[index+1:]...)
				p.mu.Unlock()
				return message
			}
		}
		p.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for event %q", eventType)
	return nil
}

func assertHistoricalClosureStages(t *testing.T, db *gorm.DB, orgID uint64, parent, child historicalseed.Context, taskID string, answerSheetID uint64) map[string]stageport.Record {
	t.Helper()
	repo := historicalstageinfra.NewRepository(db)
	parentRecords, err := repo.ListScenario(t.Context(), orgID, parent.BatchID, parent.ScenarioID)
	if err != nil {
		t.Fatal(err)
	}
	if len(parentRecords) != 1 || parentRecords[0].Stage != stageport.StagePlanEnrollment {
		t.Fatalf("parent stages=%+v, want plan_enrollment only", parentRecords)
	}
	records, err := repo.ListScenario(t.Context(), orgID, child.BatchID, child.ScenarioID)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{stageport.StageTaskOpen, stageport.StageAnswerSheetSubmit, stageport.StageAssessmentCreated, stageport.StageAssessmentSubmitted, stageport.StageTaskComplete, stageport.StageOutcomeCommitted, stageport.StageReportGenerated}
	got := make(map[string]stageport.Record, len(records))
	for _, record := range records {
		if record.Status != "completed" || record.ResourceID == "" || record.PayloadHash == "" || !json.Valid(record.PayloadJSON) {
			t.Fatalf("invalid completed stage: %+v", record)
		}
		got[record.Stage] = record
	}
	for _, stage := range want {
		if _, ok := got[stage]; !ok {
			t.Fatalf("missing completed stage %q; got=%v", stage, sortedStageNames(got))
		}
	}
	if len(got) != len(want) {
		t.Fatalf("unexpected child stages=%v", sortedStageNames(got))
	}
	if got[stageport.StageTaskOpen].ResourceID != taskID || got[stageport.StageTaskComplete].ResourceID != taskID {
		t.Fatalf("Plan stage task ids do not match: open=%s complete=%s task=%s", got[stageport.StageTaskOpen].ResourceID, got[stageport.StageTaskComplete].ResourceID, taskID)
	}
	if got[stageport.StageAnswerSheetSubmit].ResourceID != strconv.FormatUint(answerSheetID, 10) {
		t.Fatalf("AnswerSheet stage resource=%s want=%d", got[stageport.StageAnswerSheetSubmit].ResourceID, answerSheetID)
	}
	for index := 1; index < len(want); index++ {
		if got[want[index]].BusinessAt.Before(got[want[index-1]].BusinessAt) {
			t.Fatalf("historical timeline out of order: %s before %s", want[index], want[index-1])
		}
	}
	return got
}

func sortedStageNames(records map[string]stageport.Record) []string {
	result := make([]string, 0, len(records))
	for stage := range records {
		result = append(result, stage)
	}
	sort.Strings(result)
	return result
}

func assertFailedThenCompletedReportAttempts(t *testing.T, db *gorm.DB, orgID uint64, historical historicalseed.Context) {
	t.Helper()
	attempts, err := historicalstageinfra.NewRepository(db).ListScenarioAttempts(t.Context(), orgID, historical.BatchID, historical.ScenarioID)
	if err != nil {
		t.Fatal(err)
	}
	var report []stageport.AttemptRecord
	for _, attempt := range attempts {
		if attempt.Stage == stageport.StageReportGenerated {
			report = append(report, attempt)
		}
	}
	if len(report) < 2 || report[0].Status != "failed" || report[len(report)-1].Status != "completed" {
		t.Fatalf("report attempts=%+v, want failed then completed", report)
	}
}

func assertHistoricalClosureUniqueFacts(t *testing.T, db *gorm.DB, mongoDB *mongo.Database, orgID uint64, answerSheetID uint64, stages map[string]stageport.Record) {
	t.Helper()
	assertSQLCount := func(table, where string, args []any, want int64) {
		var count int64
		if err := db.Table(table).Where(where, args...).Count(&count).Error; err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != want {
			t.Fatalf("%s count=%d want=%d", table, count, want)
		}
	}
	assertSQLCount("assessment", "org_id = ? AND answer_sheet_id = ?", []any{orgID, answerSheetID}, 1)
	assertSQLCount("evaluation_outcome", "org_id = ? AND assessment_id = ?", []any{orgID, stages[stageport.StageAssessmentCreated].ResourceID}, 1)
	assertSQLCount("assessment_task", "id = ? AND assessment_id = ?", []any{stages[stageport.StageTaskComplete].ResourceID, stages[stageport.StageAssessmentCreated].ResourceID}, 1)
	outcomeID, err := strconv.ParseUint(stages[stageport.StageOutcomeCommitted].ResourceID, 10, 64)
	if err != nil {
		t.Fatalf("parse Outcome stage resource: %v", err)
	}
	reportID, err := strconv.ParseUint(stages[stageport.StageReportGenerated].ResourceID, 10, 64)
	if err != nil {
		t.Fatalf("parse Report stage resource: %v", err)
	}
	assertMongoCount(t, mongoDB, "answersheets", bson.M{"domain_id": answerSheetID}, 1)
	assertMongoCount(t, mongoDB, "report_generations", bson.M{"outcome_id": outcomeID}, 1)
	assertMongoCount(t, mongoDB, "interpretation_runs", bson.M{}, 1)
	assertMongoCount(t, mongoDB, "interpret_report_artifacts", bson.M{"domain_id": reportID}, 1)
}

func assertMongoCount(t *testing.T, db *mongo.Database, collection string, filter bson.M, want int64) {
	t.Helper()
	count, err := db.Collection(collection).CountDocuments(t.Context(), filter)
	if err != nil {
		t.Fatalf("count Mongo %s: %v", collection, err)
	}
	if count != want {
		t.Fatalf("Mongo %s count=%d want=%d filter=%v", collection, count, want, filter)
	}
}

func assertHistoricalClosureOutboxes(t *testing.T, db *gorm.DB, mongoDB *mongo.Database) {
	t.Helper()
	var mysqlPublished int64
	if err := db.Table("domain_event_outbox").Where("event_type IN ? AND status = ?", []string{eventcatalog.EvaluationRequested, eventcatalog.EvaluationOutcomeCommitted}, "published").Count(&mysqlPublished).Error; err != nil {
		t.Fatal(err)
	}
	if mysqlPublished < 2 {
		t.Fatalf("published MySQL outbox rows=%d want>=2", mysqlPublished)
	}
	mongoPublished, err := mongoDB.Collection("domain_event_outbox").CountDocuments(t.Context(), bson.M{"event_type": bson.M{"$in": []string{eventcatalog.AnswerSheetSubmitted, eventcatalog.InterpretationReportGenerated}}, "status": "published"})
	if err != nil || mongoPublished < 2 {
		t.Fatalf("published Mongo outbox rows=%d want>=2 err=%v", mongoPublished, err)
	}
}

func runHistoricalClosureStatistics(t *testing.T, c *container.Container, orgID uint64) {
	t.Helper()
	historicalDay := time.Date(2026, 7, 27, 0, 0, 0, 0, statisticsDomain.Shanghai)
	latestCompleteDay := statisticsDomain.BusinessDate(time.Now()).AddDate(0, 0, -1)
	for _, request := range []statisticsapp.RunRequest{
		{OrgID: int64(orgID), FromDate: historicalDay, ToDate: historicalDay, Mode: statisticsDomain.RunModeRepair, TriggerType: "integration", Reason: "historical closure repair"},
		{OrgID: int64(orgID), FromDate: historicalDay, ToDate: historicalDay, Mode: statisticsDomain.RunModeValidate, TriggerType: "integration", Reason: "historical closure validate"},
		{OrgID: int64(orgID), FromDate: latestCompleteDay, ToDate: latestCompleteDay, Mode: statisticsDomain.RunModePublish, TriggerType: "integration", Reason: "historical closure publish"},
	} {
		run, err := c.StatisticsModule.Coordinator.Run(t.Context(), request)
		if err != nil || run == nil || run.Status != statisticsDomain.RunStatusSucceeded {
			t.Fatalf("Statistics %s: run=%+v err=%v", request.Mode, run, err)
		}
		if request.Mode == statisticsDomain.RunModePublish && !run.AsOfDate.Equal(latestCompleteDay) {
			t.Fatalf("Statistics publish waterline=%s want=%s", run.AsOfDate, latestCompleteDay)
		}
	}
}
