//go:build integration

package runtimeclosure

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	cbdatabase "github.com/FangcunMount/component-base/pkg/database"
	"github.com/FangcunMount/component-base/pkg/messaging"
	actorpb "github.com/FangcunMount/qs-server/api/grpc/gen/actor"
	answersheetpb "github.com/FangcunMount/qs-server/api/grpc/gen/answersheet"
	evaluationpb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	interpretationpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	assessmententryapp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/assessmententry"
	clinicianapp "github.com/FangcunMount/qs-server/internal/apiserver/application/actor/clinician"
	evaluationtesteeapp "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/testee"
	interpretationparticipantapp "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/participant"
	assessmentintakejourney "github.com/FangcunMount/qs-server/internal/apiserver/application/journey/assessmentintake"
	planapp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
	statisticsapp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	cachebootstrap "github.com/FangcunMount/qs-server/internal/apiserver/cache/subsystem"
	"github.com/FangcunMount/qs-server/internal/apiserver/container"
	modelDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelFactor "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	statisticsDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	questionnaireDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	mongoModelCatalog "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/modelcatalog"
	mongoQuestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/questionnaire"
	rulesetInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/ruleset"
	modelport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	grpctransport "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc"
	grpcservice "github.com/FangcunMount/qs-server/internal/apiserver/transport/grpc/service"
	collectionevaluation "github.com/FangcunMount/qs-server/internal/collection-server/application/evaluation"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/reportwait"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	migrationpkg "github.com/FangcunMount/qs-server/internal/pkg/migration"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
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
	runtimeModelCode    = "SCALE_RUNTIME_E2E"
	runtimeQuestionCode = "Q_RUNTIME_E2E"
	runtimeVersion      = "1.0.0"
)

// TestCurrentRuntimeClosure exercises the ordinary, current-time path through
// Actor, AssessmentEntry, Plan, AnswerSheet, Assessment, Evaluation,
// Interpretation and report-wait using real application services,
// repositories, durable outboxes and worker handlers.
func TestCurrentRuntimeClosure(t *testing.T) {
	mysqlDSN := os.Getenv("MYSQL_DSN")
	if mysqlDSN == "" {
		t.Fatal("MYSQL_DSN is required; the runtime closure test must not skip")
	}
	redisURL := os.Getenv("QS_SERVER_TEST_REDIS_URL")
	if redisURL == "" {
		t.Fatal("QS_SERVER_TEST_REDIS_URL is required; the runtime closure test must not skip")
	}
	startedAt := time.Now().UTC().Add(-5 * time.Second)

	sqlDB, databaseName := openRuntimeDatabase(t, mysqlDSN)
	mysqlMigrator := migrationpkg.NewMigrator(sqlDB, &migrationpkg.Config{Enabled: true, Database: databaseName})
	mysqlVersion, changed, err := mysqlMigrator.Run()
	if err != nil || !changed || mysqlVersion == 0 {
		t.Fatalf("migrate empty MySQL: version=%d changed=%v err=%v", mysqlVersion, changed, err)
	}
	if version, changed, err := mysqlMigrator.Run(); err != nil || changed || version != mysqlVersion {
		t.Fatalf("repeat MySQL migration: version=%d want_version=%d changed=%v err=%v", version, mysqlVersion, changed, err)
	}
	gormDB, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	mongoClient, mongoDB := mongodbtest.ReplicaSetDatabase(t)
	mongoMigrator := migrationpkg.NewMongoMigrator(mongoClient, &migrationpkg.Config{Enabled: true, Database: mongoDB.Name()})
	mongoVersion, changed, err := mongoMigrator.Run()
	if err != nil || !changed || mongoVersion == 0 {
		t.Fatalf("migrate empty MongoDB: version=%d changed=%v err=%v", mongoVersion, changed, err)
	}
	if version, changed, err := mongoMigrator.Run(); err != nil || changed || version != mongoVersion {
		t.Fatalf("repeat MongoDB migration: version=%d want_version=%d changed=%v err=%v", version, mongoVersion, changed, err)
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
		Consumers:  map[string]eventsubsystem.ConsumerOptions{"modelcatalog.hot_rank_projection": {Enabled: false}},
	})
	if err != nil {
		t.Fatalf("build event subsystem: %v", err)
	}
	locks := newTestLocks(redisClient, "apiserver", "test:runtime-closure:apiserver")
	workerLocks := newTestLocks(redisClient, "worker", "test:runtime-closure:worker")
	cacheSubsystem := newRuntimeCacheSubsystem(redisClient)
	c := container.NewContainerWithOptions(gormDB, mongoDB, redisClient, container.ContainerOptions{
		EventSubsystem: eventing, CacheSubsystem: cacheSubsystem, LockSubsystem: locks, Silent: true,
	})
	if err := c.Initialize(); err != nil {
		t.Fatalf("initialize application container: %v", err)
	}
	if err := c.StartEventSubsystem(t.Context()); err != nil {
		t.Fatalf("start durable outbox relays: %v", err)
	}
	t.Cleanup(func() {
		if err := c.Cleanup(); err != nil {
			t.Errorf("cleanup container: %v", err)
		}
	})

	seedRuntimeCatalog(t, mongoDB)
	orgID := uint64(820000 + time.Now().UnixNano()%10000)
	grpcDeps := c.BuildGRPCDeps(nil)
	testeeID, entryID := createRuntimeActorAndEntry(t, c, grpcDeps, orgID)

	planResult, err := c.PlanModule.CommandService.CreatePlan(t.Context(), planapp.CreatePlanDTO{
		OrgID: int64(orgID), ScaleCode: runtimeModelCode, ScheduleType: "by_day", TriggerTime: "00:00:00", Interval: 1, TotalTimes: 1,
	})
	if err != nil {
		t.Fatalf("create Plan: %v", err)
	}
	today := time.Now().In(statisticsDomain.Shanghai).Format("2006-01-02")
	enrollment, err := c.PlanModule.CommandService.EnrollTestee(t.Context(), planapp.EnrollTesteeDTO{
		OrgID: int64(orgID), PlanID: planResult.ID, TesteeID: strconv.FormatUint(testeeID, 10), StartDate: today,
	})
	if err != nil || len(enrollment.Tasks) != 1 {
		t.Fatalf("enroll Testee: result=%+v err=%v", enrollment, err)
	}
	taskID := enrollment.Tasks[0].ID
	opened, err := c.PlanModule.CommandService.OpenTask(t.Context(), int64(orgID), taskID)
	if err != nil || opened == nil || opened.Status != "opened" {
		t.Fatalf("open Plan task: result=%+v err=%v", opened, err)
	}

	answerService := grpcservice.NewAnswerSheetService(grpcDeps.Survey.AnswerSheetSubmissionService)
	journey := assessmentintakejourney.NewService(
		grpcDeps.Survey.AnswerSheetScoringService,
		rulesetInfra.NewAssessmentBindingResolver(grpcDeps.PublishedModelCatalog),
		grpcDeps.Plan.TaskAssessmentResolver,
		grpcDeps.Plan.CommandService,
		grpcDeps.Evaluation.IntakeService,
		grpcDeps.Interpretation.ReportStatusReporter,
	)
	assessmentService := grpcservice.NewAssessmentIntakeService(journey, grpcDeps.Evaluation.IntakeService, grpcDeps.Survey.AnswerSheetManagementService)
	evaluationService := grpcservice.NewEvaluationWorkerService(grpcDeps.Evaluation.WorkerService)
	interpretationService := grpcservice.NewInterpretationAutomationService(grpcDeps.Interpretation.AutomationService)
	workerDeps := &handlers.Dependencies{
		Logger:                         slog.New(slog.NewTextHandler(io.Discard, nil)),
		AssessmentIntakeClient:         assessmentService,
		EvaluationWorkerClient:         evaluationWorkerAdapter{service: evaluationService},
		InterpretationAutomationClient: interpretationWorkerAdapter{service: interpretationService},
		ReportStatusReporter:           grpcDeps.Interpretation.ReportStatusReporter,
		LockManager:                    workerLocks, LockRunner: workerLocks, LockKeyBuilder: workerLocks.Builder(),
	}
	registry := handlers.NewRegistry()
	answerHandler := mustWorkerHandler(t, registry, "answersheet_submitted_handler", workerDeps)
	evaluationHandler := mustWorkerHandler(t, registry, "evaluation_requested_handler", workerDeps)
	outcomeHandler := mustWorkerHandler(t, registry, "evaluation_outcome_committed_handler", workerDeps)
	reportHandler := mustWorkerHandler(t, registry, "interpretation_report_generated_handler", workerDeps)

	request := &answersheetpb.SaveAnswerSheetRequest{
		QuestionnaireCode: runtimeQuestionCode, QuestionnaireVersion: runtimeVersion,
		IdempotencyKey: "runtime-e2e-answer-0001", WriterId: testeeID, TesteeId: testeeID, OrgId: orgID,
		TaskId: taskID, OriginRef: &answersheetpb.OriginRef{Type: "plan_task", Id: taskID},
		Answers: []*answersheetpb.Answer{{QuestionCode: "Q1", QuestionType: "Radio", Value: "A"}},
	}
	answerResponse, err := answerService.SaveAnswerSheet(t.Context(), request)
	if err != nil || answerResponse.GetId() == 0 {
		t.Fatalf("submit AnswerSheet: response=%+v err=%v", answerResponse, err)
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
	if err := outcomeHandler(t.Context(), eventcatalog.EvaluationOutcomeCommitted, outcomeMessage.Payload); err != nil {
		t.Fatalf("consume evaluation.outcome.committed: %v", err)
	}
	reportMessage := capture.Wait(t, eventcatalog.InterpretationReportGenerated)
	if err := reportHandler(t.Context(), eventcatalog.InterpretationReportGenerated, reportMessage.Payload); err != nil {
		t.Fatalf("consume interpretation.report.generated: %v", err)
	}

	replayed, err := answerService.SaveAnswerSheet(t.Context(), request)
	if err != nil || replayed.GetId() != answerResponse.GetId() {
		t.Fatalf("AnswerSheet idempotency: first=%d replay=%+v err=%v", answerResponse.GetId(), replayed, err)
	}
	readiness, err := assessmentService.ResolveAssessmentByAnswerSheetID(t.Context(), &evaluationpb.ResolveAssessmentByAnswerSheetIDRequest{AnswerSheetId: answerResponse.GetId()})
	if err != nil || readiness.GetReadinessPhase() != "ready" || readiness.GetAssessmentStatus() != "evaluated" {
		t.Fatalf("assessment readiness: response=%+v err=%v", readiness, err)
	}
	assertReportWaitClosure(t, grpcDeps, testeeID, readiness.GetAssessmentId())
	assertCurrentRuntimeFacts(t, gormDB, mongoDB, orgID, testeeID, entryID, taskID, answerResponse.GetId(), readiness.GetAssessmentId(), startedAt)
	assertLegacyBusinessDateStatistics(t, c, gormDB, orgID, testeeID)
}

func createRuntimeActorAndEntry(t *testing.T, c *container.Container, grpcDeps grpctransport.Deps, orgID uint64) (uint64, uint64) {
	t.Helper()
	clinician, err := c.ActorModule.ClinicianLifecycleService.Register(t.Context(), clinicianapp.RegisterClinicianDTO{
		OrgID: int64(orgID), Name: "runtime-closure-clinician", Department: "integration", Title: "counselor",
		ClinicianType: "counselor", EmployeeCode: fmt.Sprintf("runtime-%d", orgID), IsActive: true,
	})
	if err != nil {
		t.Fatalf("create Clinician: %v", err)
	}
	profileID := orgID + 1000000
	actorService := grpcservice.NewActorService(
		grpcDeps.Actor.TesteeRegistrationService,
		grpcDeps.Actor.TesteeManagementService,
		grpcDeps.Actor.TesteeQueryService,
		grpcDeps.Actor.ClinicianRelationshipService,
	)
	testee, err := actorService.CreateTestee(t.Context(), &actorpb.CreateTesteeRequest{
		OrgId: orgID, IamProfileId: profileID, Name: "runtime-closure-testee", Source: "online_form",
	})
	if err != nil || testee.GetId() == 0 {
		t.Fatalf("create Testee through Actor service: response=%+v err=%v", testee, err)
	}
	entry, err := c.ActorModule.AssessmentEntryService.Create(t.Context(), assessmententryapp.CreateAssessmentEntryDTO{
		OrgID: int64(orgID), ClinicianID: clinician.ID, TargetType: "scale", TargetCode: runtimeModelCode, TargetVersion: runtimeVersion,
	})
	if err != nil {
		t.Fatalf("create AssessmentEntry: %v", err)
	}
	resolved, err := c.ActorModule.AssessmentEntryService.Resolve(t.Context(), entry.Token)
	if err != nil || resolved == nil || resolved.Entry == nil || resolved.Entry.ID != entry.ID {
		t.Fatalf("resolve AssessmentEntry: result=%+v err=%v", resolved, err)
	}
	intake, err := c.ActorModule.AssessmentEntryService.Intake(t.Context(), entry.Token, assessmententryapp.IntakeByAssessmentEntryDTO{
		ProfileID: &profileID, Name: testee.GetName(), Gender: int8(testee.GetGender()),
	})
	if err != nil {
		t.Fatalf("intake AssessmentEntry: %v", err)
	}
	if intake == nil || intake.Testee == nil || intake.Testee.ID != testee.GetId() {
		t.Fatalf("AssessmentEntry intake did not reuse Actor Testee: result=%+v actor_testee_id=%d", intake, testee.GetId())
	}
	if intake.Relation == nil || intake.Assignment == nil {
		t.Fatalf("AssessmentEntry intake relations are incomplete: result=%+v", intake)
	}
	return testee.GetId(), entry.ID
}

type runtimeReportQuery struct {
	assessments evaluationtesteeapp.Service
	reports     interpretationparticipantapp.Service
}

func (q runtimeReportQuery) GetMyAssessment(ctx context.Context, testeeID, assessmentID uint64) (*collectionevaluation.AssessmentDetailResponse, error) {
	result, err := q.assessments.GetAssessment(ctx, evaluationtesteeapp.Actor{TesteeID: testeeID}, assessmentID)
	if err != nil {
		return nil, err
	}
	response := &collectionevaluation.AssessmentDetailResponse{
		ID: strconv.FormatUint(result.ID, 10), OrgID: strconv.FormatUint(result.OrgID, 10),
		TesteeID: strconv.FormatUint(result.TesteeID, 10), Status: result.Status,
	}
	if result.PrimaryScore != nil {
		response.PrimaryScore = &collectionevaluation.ScoreValueResponse{
			Kind: result.PrimaryScore.Kind, Value: result.PrimaryScore.Value, Label: result.PrimaryScore.Label, Max: result.PrimaryScore.Max,
		}
	}
	if result.Level != nil {
		response.Level = &collectionevaluation.ResultLevelResponse{
			Code: result.Level.Code, Label: result.Level.Label, Severity: result.Level.Severity,
		}
	}
	if result.FailureReason != nil {
		response.FailureReason = *result.FailureReason
	}
	return response, nil
}

func (q runtimeReportQuery) GetAssessmentReport(ctx context.Context, testeeID, assessmentID uint64) (*collectionevaluation.AssessmentReportResponse, error) {
	result, err := q.reports.GetMyReport(ctx, interpretationparticipantapp.Actor{TesteeID: testeeID}, interpretationparticipantapp.GetQuery{AssessmentID: assessmentID})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, nil
	}
	return &collectionevaluation.AssessmentReportResponse{AssessmentID: strconv.FormatUint(assessmentID, 10)}, nil
}

func assertReportWaitClosure(t *testing.T, grpcDeps grpctransport.Deps, testeeID, assessmentID uint64) {
	t.Helper()
	query := runtimeReportQuery{assessments: grpcDeps.Evaluation.TesteeService, reports: grpcDeps.Interpretation.ParticipantService}
	cache := grpcDeps.Interpretation.ReportStatusReporter.Cache()
	waiter := reportwait.NewService(query, cache, nil, nil, reportwait.DefaultConfig())
	status, err := waiter.Wait(t.Context(), testeeID, assessmentID, time.Second)
	if err != nil || status == nil || status.Status != "completed" || status.Stage != "completed" {
		t.Fatalf("wait report: status=%+v err=%v", status, err)
	}
	snapshot, err := cache.Get(t.Context(), strconv.FormatUint(assessmentID, 10))
	if err != nil || snapshot == nil || snapshot.ReportID == "" {
		t.Fatalf("report status snapshot: snapshot=%+v err=%v", snapshot, err)
	}
	report, err := query.GetAssessmentReport(t.Context(), testeeID, assessmentID)
	if err != nil || report == nil || report.AssessmentID != strconv.FormatUint(assessmentID, 10) {
		t.Fatalf("participant report: report=%+v err=%v", report, err)
	}
}

func assertLegacyBusinessDateStatistics(t *testing.T, c *container.Container, db *gorm.DB, orgID, testeeID uint64) {
	t.Helper()
	plan, err := c.PlanModule.CommandService.CreatePlan(t.Context(), planapp.CreatePlanDTO{
		OrgID: int64(orgID), ScaleCode: runtimeModelCode, ScheduleType: "by_day", TriggerTime: "00:00:00", Interval: 1, TotalTimes: 1,
	})
	if err != nil {
		t.Fatalf("create legacy compatibility Plan: %v", err)
	}
	today := statisticsDomain.BusinessDate(time.Now())
	enrollment, err := c.PlanModule.CommandService.EnrollTestee(t.Context(), planapp.EnrollTesteeDTO{
		OrgID: int64(orgID), PlanID: plan.ID, TesteeID: strconv.FormatUint(testeeID, 10), StartDate: today.Format("2006-01-02"),
	})
	if err != nil || len(enrollment.Tasks) != 1 {
		t.Fatalf("enroll legacy compatibility task: result=%+v err=%v", enrollment, err)
	}
	legacyTaskID := enrollment.Tasks[0].ID
	legacyDate := today.AddDate(0, 0, -1)
	legacyAt := legacyDate.Add(12 * time.Hour)
	update := db.Table("assessment_task").Where("id = ? AND org_id = ?", legacyTaskID, orgID).UpdateColumn("business_created_at", legacyAt)
	if update.Error != nil || update.RowsAffected != 1 {
		t.Fatalf("simulate legacy task business_created_at: rows=%d err=%v", update.RowsAffected, update.Error)
	}
	run, err := c.StatisticsModule.Coordinator.Run(t.Context(), statisticsapp.RunRequest{
		OrgID: int64(orgID), FromDate: legacyDate, ToDate: legacyDate,
		Reason: "runtime closure business_created_at compatibility", TriggerType: "integration", Mode: statisticsDomain.RunModeRepair,
	})
	if err != nil || run == nil || run.Status != statisticsDomain.RunStatusSucceeded {
		t.Fatalf("repair legacy task statistics: run=%+v err=%v", run, err)
	}
	var fact struct {
		OccurredAt time.Time
		StatDate   time.Time
	}
	if err := db.Table("statistics_plan_fact").Select("occurred_at, stat_date").Where("org_id = ? AND task_id = ? AND fact_type = 'task_created'", orgID, legacyTaskID).Take(&fact).Error; err != nil {
		t.Fatalf("load legacy task_created fact: %v", err)
	}
	if !fact.OccurredAt.Equal(legacyAt) || !statisticsDomain.BusinessDate(fact.StatDate).Equal(legacyDate) {
		t.Fatalf("legacy task fact dates: occurred_at=%s stat_date=%s want=%s", fact.OccurredAt, fact.StatDate, legacyDate)
	}
	var daily struct {
		StatDate         time.Time
		TaskCreatedCount int64
	}
	if err := db.Table("statistics_plan_activity_daily").Select("stat_date, task_created_count").Where("org_id = ? AND plan_id = ? AND stat_date = ?", orgID, plan.ID, legacyDate).Take(&daily).Error; err != nil {
		t.Fatalf("load legacy plan activity projection: %v", err)
	}
	if !statisticsDomain.BusinessDate(daily.StatDate).Equal(legacyDate) || daily.TaskCreatedCount != 1 {
		t.Fatalf("legacy plan projection: %+v want stat_date=%s task_created_count=1", daily, legacyDate)
	}
}

func openRuntimeDatabase(t *testing.T, dsn string) (*sql.DB, string) {
	t.Helper()
	cfg, err := drivermysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Loc == nil || cfg.Loc.String() != statisticsDomain.Shanghai.String() {
		t.Fatalf("MYSQL_DSN loc=%v, want %s", cfg.Loc, statisticsDomain.Shanghai)
	}
	databaseName := fmt.Sprintf("qs_runtime_closure_%d", time.Now().UnixNano())
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

func newTestLocks(client redis.UniversalClient, component, namespace string) *locksubsystem.Subsystem {
	handle := &redisruntime.Handle{
		Family: redisruntime.FamilyLock, Client: client,
		Builder: keyspace.NewBuilderWithNamespace(namespace), Configured: true, Available: true,
	}
	return locksubsystem.New(locksubsystem.Options{Component: component, Handle: handle})
}

type runtimeRedisResolver struct{ client redis.UniversalClient }

func (r runtimeRedisResolver) GetRedisClient() (redis.UniversalClient, error) {
	return r.client, nil
}

func (r runtimeRedisResolver) GetRedisClientByProfile(string) (redis.UniversalClient, error) {
	return r.client, nil
}

func (runtimeRedisResolver) GetRedisProfileStatus(string) cbdatabase.RedisProfileStatus {
	return cbdatabase.RedisProfileStatus{State: cbdatabase.RedisProfileStateAvailable}
}

func newRuntimeCacheSubsystem(client redis.UniversalClient) *cachebootstrap.Subsystem {
	return cachebootstrap.NewSubsystem("apiserver", runtimeRedisResolver{client: client}, &genericoptions.RedisRuntimeOptions{
		Namespace: "test:runtime-closure",
		Families: map[string]*genericoptions.RedisRuntimeFamilyRoute{
			string(redisruntime.FamilyOps): {
				RedisProfile: "runtime", NamespaceSuffix: "ops",
			},
		},
	}, container.ContainerCacheOptions{})
}

func seedRuntimeCatalog(t *testing.T, db *mongo.Database) {
	t.Helper()
	question, err := questionnaireDomain.NewQuestion(
		questionnaireDomain.WithCode(meta.NewCode("Q1")), questionnaireDomain.WithStem("Runtime closure question"),
		questionnaireDomain.WithQuestionType(questionnaireDomain.TypeRadio), questionnaireDomain.WithOption("A", "A", 1), questionnaireDomain.WithOption("B", "B", 0), questionnaireDomain.WithRequired(),
	)
	if err != nil {
		t.Fatal(err)
	}
	publishedAt := time.Now().UTC()
	questionnaire, err := questionnaireDomain.NewQuestionnaire(meta.NewCode(runtimeQuestionCode), "Runtime closure questionnaire",
		questionnaireDomain.WithVersion(questionnaireDomain.Version(runtimeVersion)), questionnaireDomain.WithStatus(questionnaireDomain.STATUS_PUBLISHED),
		questionnaireDomain.WithRecordRole(questionnaireDomain.RecordRolePublishedSnapshot), questionnaireDomain.WithActivePublished(true), questionnaireDomain.WithReleaseStatus(questionnaireDomain.ReleaseStatusActive),
		questionnaireDomain.WithPublishedAt(&publishedAt), questionnaireDomain.WithQuestions([]questionnaireDomain.Question{question}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := mongoQuestionnaire.NewRepository(db).CreatePublishedSnapshot(t.Context(), questionnaire, true); err != nil {
		t.Fatalf("seed published questionnaire: %v", err)
	}
	model := &modelport.PublishedModel{
		SchemaVersion: modelDomain.SchemaVersionV2, ProductChannel: modelDomain.ProductChannelMedicalScale,
		Kind: modelDomain.KindScale, Algorithm: modelDomain.AlgorithmScaleDefault, AlgorithmFamily: modelDomain.AlgorithmFamilyFactorScoring,
		Code: runtimeModelCode, Version: runtimeVersion, Title: "Runtime closure scale", Status: "published", ReleaseStatus: modelDomain.ReleaseStatusActive,
		DecisionKind: modelDomain.DecisionKindScoreRange, QuestionnaireCode: runtimeQuestionCode, QuestionnaireVersion: runtimeVersion,
		DefinitionV2: &modelDomain.Definition{
			Measure:     modelDomain.MeasureSpec{Factors: []modelDomain.Factor{{Code: "TOTAL", Title: "Total", Role: modelFactor.FactorRoleTotal}}, Scoring: []modelDomain.Scoring{{FactorCode: "TOTAL", Strategy: modelFactor.ScoringStrategySum, Sources: []modelDomain.ScoringSource{{Kind: modelDomain.ScoringSourceQuestion, Code: "Q1"}}}}},
			Outcomes:    []modelDomain.Outcome{{Code: "low", Title: "Low"}},
			Conclusions: []modelDomain.Conclusion{modelDomain.RiskConclusion{FactorCode: "TOTAL", Rules: []modelDomain.ScoreRangeOutcome{{MinScore: 0, MaxScore: 10, MaxInclusive: true, OutcomeCode: "low", Level: "low"}}}},
			ReportMap: modelDomain.ReportMap{Sections: []modelDomain.ReportSection{{
				Code: modelDomain.ReportSectionKindFactorScores, Kind: modelDomain.ReportSectionKindFactorScores, SourceRefs: []string{"TOTAL"},
			}}},
		},
	}
	if err := mongoModelCatalog.NewRepository(db).UpsertPublishedModel(t.Context(), model); err != nil {
		t.Fatalf("seed published model: %v", err)
	}
}

type evaluationWorkerAdapter struct {
	service *grpcservice.EvaluationWorkerService
}

func (a evaluationWorkerAdapter) ExecuteEvaluation(ctx context.Context, assessmentID uint64) (*evaluationpb.ExecuteEvaluationResponse, error) {
	return a.service.ExecuteEvaluation(ctx, &evaluationpb.ExecuteEvaluationRequest{AssessmentId: assessmentID})
}

type interpretationWorkerAdapter struct {
	service *grpcservice.InterpretationAutomationService
}

func (a interpretationWorkerAdapter) GenerateReportFromOutcome(ctx context.Context, outcomeID string) (*interpretationpb.GenerateReportFromAssessmentResponse, error) {
	return a.service.GenerateReportFromOutcome(ctx, &interpretationpb.GenerateReportFromOutcomeRequest{OutcomeId: outcomeID})
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

func newCapturedMQPublisher() *capturedMQPublisher                           { return &capturedMQPublisher{} }
func (p *capturedMQPublisher) Publish(context.Context, string, []byte) error { return nil }
func (p *capturedMQPublisher) Close() error                                  { return nil }
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

func assertCurrentRuntimeFacts(
	t *testing.T,
	db *gorm.DB,
	mongoDB *mongo.Database,
	orgID, testeeID, entryID uint64,
	taskID string,
	answerSheetID, assessmentID uint64,
	startedAt time.Time,
) {
	t.Helper()
	var testee struct{ CreatedAt time.Time }
	if err := db.Table("testee").Select("created_at").Where("id = ? AND org_id = ?", testeeID, orgID).Take(&testee).Error; err != nil {
		t.Fatalf("load Testee: %v", err)
	}
	assertCurrentTime(t, "testee.created_at", testee.CreatedAt, startedAt)

	var entry struct{ CreatedAt time.Time }
	if err := db.Table("assessment_entry").Select("created_at").Where("id = ? AND org_id = ?", entryID, orgID).Take(&entry).Error; err != nil {
		t.Fatalf("load AssessmentEntry: %v", err)
	}
	assertCurrentTime(t, "assessment_entry.created_at", entry.CreatedAt, startedAt)
	assertRowCount(t, db, "assessment_entry_resolve_log", "org_id = ? AND entry_id = ?", 1, orgID, entryID)
	assertRowCount(t, db, "assessment_entry_intake_log", "org_id = ? AND entry_id = ? AND testee_id = ?", 1, orgID, entryID, testeeID)
	assertRowCount(t, db, "clinician_relation", "org_id = ? AND source_type = 'assessment_entry' AND source_id = ? AND testee_id = ?", 2, orgID, entryID, testeeID)

	var task struct {
		BusinessCreatedAt sql.NullTime `gorm:"column:business_created_at"`
		CreatedAt         time.Time    `gorm:"column:created_at"`
		PlanID            uint64       `gorm:"column:plan_id"`
	}
	if err := db.Table("assessment_task").Select("business_created_at, created_at, plan_id").Where("id = ? AND org_id = ?", taskID, orgID).Take(&task).Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.BusinessCreatedAt.Valid {
		t.Fatalf("new task business_created_at=%s, want NULL", task.BusinessCreatedAt.Time)
	}
	assertCurrentTime(t, "task.created_at", task.CreatedAt, startedAt)
	var plan struct{ CreatedAt time.Time }
	if err := db.Table("assessment_plan").Select("created_at").Where("id = ? AND org_id = ?", task.PlanID, orgID).Take(&plan).Error; err != nil {
		t.Fatalf("load Plan: %v", err)
	}
	assertCurrentTime(t, "plan.created_at", plan.CreatedAt, startedAt)

	var assessment struct{ CreatedAt, SubmittedAt time.Time }
	if err := db.Table("assessment").Select("created_at, submitted_at").Where("id = ? AND org_id = ? AND answer_sheet_id = ?", assessmentID, orgID, answerSheetID).Take(&assessment).Error; err != nil {
		t.Fatalf("load assessment: %v", err)
	}
	assertCurrentTime(t, "assessment.created_at", assessment.CreatedAt, startedAt)
	assertCurrentTime(t, "assessment.submitted_at", assessment.SubmittedAt, startedAt)

	var outcome struct{ EvaluatedAt time.Time }
	if err := db.Table("evaluation_outcome").Select("evaluated_at").Where("assessment_id = ? AND org_id = ?", assessmentID, orgID).Take(&outcome).Error; err != nil {
		t.Fatalf("load outcome: %v", err)
	}
	assertCurrentTime(t, "outcome.evaluated_at", outcome.EvaluatedAt, startedAt)

	var answer struct {
		FilledAt    time.Time `bson:"filled_at"`
		Attribution struct {
			CapturedAt time.Time `bson:"captured_at"`
		} `bson:"attribution"`
	}
	if err := mongoDB.Collection("answersheets").FindOne(t.Context(), bson.M{"domain_id": answerSheetID}).Decode(&answer); err != nil {
		t.Fatalf("load AnswerSheet: %v", err)
	}
	assertCurrentTime(t, "answersheet.filled_at", answer.FilledAt, startedAt)
	assertCurrentTime(t, "answersheet.attribution.captured_at", answer.Attribution.CapturedAt, startedAt)
	answerCount, err := mongoDB.Collection("answersheets").CountDocuments(t.Context(), bson.M{"domain_id": answerSheetID})
	if err != nil || answerCount != 1 {
		t.Fatalf("AnswerSheet idempotency count=%d err=%v", answerCount, err)
	}
	assertRowCount(t, db, "assessment", "org_id = ? AND answer_sheet_id = ?", 1, orgID, answerSheetID)
	assertRowCount(t, db, "evaluation_outcome", "org_id = ? AND assessment_id = ?", 1, orgID, assessmentID)

	var artifact struct {
		GeneratedAt time.Time `bson:"generated_at"`
	}
	if err := mongoDB.Collection("interpret_report_artifacts").FindOne(t.Context(), bson.M{"assessment_id": assessmentID}).Decode(&artifact); err != nil {
		t.Fatalf("load report artifact: %v", err)
	}
	assertCurrentTime(t, "report.generated_at", artifact.GeneratedAt, startedAt)
	reportCount, err := mongoDB.Collection("interpret_report_artifacts").CountDocuments(t.Context(), bson.M{"assessment_id": assessmentID})
	if err != nil || reportCount != 1 {
		t.Fatalf("report artifact count=%d err=%v", reportCount, err)
	}
}

func assertRowCount(t *testing.T, db *gorm.DB, table, where string, want int64, args ...any) {
	t.Helper()
	var got int64
	if err := db.Table(table).Where(where, args...).Count(&got).Error; err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s count=%d, want %d", table, got, want)
	}
}

func assertCurrentTime(t *testing.T, name string, value, floor time.Time) {
	t.Helper()
	value = value.UTC()
	if value.Before(floor) || value.After(time.Now().UTC().Add(10*time.Second)) {
		t.Fatalf("%s=%s is outside current runtime window [%s, now]", name, value, floor)
	}
}
