//go:build integration

package migration

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	componenterrors "github.com/FangcunMount/component-base/pkg/errors"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	statisticsModule "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/statistics"
	actorDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor"
	assessmententryDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/assessmententry"
	clinicianDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/clinician"
	testeeDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	assessmentDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	outcomeDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	interpretationPolicy "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	reportDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	modelcatalogDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	planDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	statisticsDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	answersheetDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	questionnaireDomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	mongoAnswersheet "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/answersheet"
	mongoInterpretation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation"
	mysqlActor "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor"
	mysqlEvaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	mysqlPlan "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/backpressure"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
	redis "github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/mongo"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type coldStartLockRunner struct{}

func (coldStartLockRunner) Run(ctx context.Context, _ locklease.WorkloadID, _ string, _ time.Duration, body func(context.Context) error) (locklease.RunResult, error) {
	return locklease.RunResult{Acquired: true}, body(ctx)
}

type overloadedReadLimiter struct{}

func (overloadedReadLimiter) Acquire(ctx context.Context) (context.Context, func(), error) {
	return ctx, func() {}, context.DeadlineExceeded
}

var _ backpressure.Acquirer = overloadedReadLimiter{}

func TestStatisticsColdStartPublishIdempotencyAndRedisFailure(t *testing.T) {
	mysqlDSN := os.Getenv("MYSQL_DSN")
	if mysqlDSN == "" {
		t.Skip("MYSQL_DSN is required for the Statistics cold-start integration test")
	}
	redisURL := os.Getenv("QS_SERVER_TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("QS_SERVER_TEST_REDIS_URL is required for the Statistics cold-start integration test")
	}

	sqlDB, databaseName := openStatisticsMigrationDatabase(t, mysqlDSN)
	version, _, err := NewMigrator(sqlDB, &Config{Enabled: true, Database: databaseName}).Run()
	if err != nil || version != 57 {
		t.Fatalf("migrate empty MySQL: version=%d err=%v", version, err)
	}
	gormDB, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	mongoClient, mongoDB := mongodbtest.ReplicaSetDatabase(t)
	mongoVersion, _, err := NewMongoMigrator(mongoClient, &Config{Enabled: true, Database: mongoDB.Name()}).Run()
	if err != nil || mongoVersion != 18 {
		t.Fatalf("migrate empty MongoDB: version=%d err=%v", mongoVersion, err)
	}

	redisOptions, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Fatalf("parse QS_SERVER_TEST_REDIS_URL: %v", err)
	}
	controlRedis := redis.NewClient(redisOptions)
	if err := controlRedis.Ping(t.Context()).Err(); err != nil {
		t.Fatalf("ping test Redis: %v", err)
	}
	t.Cleanup(func() { _ = controlRedis.Close() })
	runtimeRedis := redis.NewClient(redisOptions)
	t.Cleanup(func() { _ = runtimeRedis.Close() })

	orgID := int64(700000 + time.Now().UnixNano()%100000)
	t.Cleanup(func() { deleteStatisticsColdStartRedisKeys(t, controlRedis, orgID) })
	latestCompleteDay := statisticsDomain.BusinessDate(time.Now()).AddDate(0, 0, -1)
	eventAt := latestCompleteDay.Add(10 * time.Hour)

	fixture := seedStatisticsColdStartBusinessFacts(t, gormDB, mongoDB, orgID, eventAt)
	module, err := statisticsModule.New(statisticsModule.Deps{
		MySQLDB: gormDB, MongoDB: mongoDB, RedisClient: runtimeRedis,
		LockRunner: coldStartLockRunner{}, QueryTTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}

	request := statisticsApp.RunRequest{
		OrgID: orgID, FromDate: latestCompleteDay, ToDate: latestCompleteDay,
		Mode: statisticsDomain.RunModePublish, TriggerType: "integration", Reason: "cold-start contract",
	}
	first, err := module.Coordinator.Run(t.Context(), request)
	if err != nil || first == nil || first.Status != statisticsDomain.RunStatusSucceeded || first.CacheGeneration <= 0 {
		t.Fatalf("first publish: run=%+v err=%v", first, err)
	}
	assertStatisticsColdStartRows(t, gormDB, orgID)
	factCounts := statisticsColdStartTableCounts(t, gormDB, orgID, []string{
		"statistics_access_fact", "statistics_assessment_fact", "statistics_plan_fact",
	})
	resultCounts := statisticsColdStartTableCounts(t, gormDB, orgID, []string{
		"statistics_access_daily", "statistics_assessment_daily", "statistics_plan_activity_daily",
		"statistics_plan_fulfillment_daily", "statistics_org_snapshot",
	})

	overview, err := module.ReadService.Overview(t.Context(), orgID, statisticsApp.QueryFilter{Preset: "latest_complete_day"})
	if err != nil {
		t.Fatal(err)
	}
	if overview.Freshness.AsOfDate != latestCompleteDay.Format("2006-01-02") || overview.Freshness.IsStale {
		t.Fatalf("freshness=%+v", overview.Freshness)
	}
	if overview.Metrics.AnswerSheetSubmissionCount != 1 || overview.Metrics.AssessmentCount != 1 || overview.Metrics.ReportCount != 1 {
		t.Fatalf("overview metrics=%+v", overview.Metrics)
	}

	second, err := module.Coordinator.Run(t.Context(), request)
	if err != nil || second == nil || second.Status != statisticsDomain.RunStatusSucceeded || second.CacheGeneration <= first.CacheGeneration {
		t.Fatalf("second publish: first=%+v second=%+v err=%v", first, second, err)
	}
	assertStatisticsCountsEqual(t, factCounts, statisticsColdStartTableCounts(t, gormDB, orgID, mapKeys(factCounts)))
	assertStatisticsCountsEqual(t, resultCounts, statisticsColdStartTableCounts(t, gormDB, orgID, mapKeys(resultCounts)))

	if err := runtimeRedis.Close(); err != nil {
		t.Fatal(err)
	}
	stale, err := module.ReadService.Overview(t.Context(), orgID, statisticsApp.QueryFilter{Preset: "latest_complete_day"})
	if err != nil || !stale.Freshness.IsStale {
		t.Fatalf("Redis-down stale read: value=%+v err=%v", stale, err)
	}

	failed, err := module.Coordinator.Run(t.Context(), request)
	if err == nil || failed == nil || failed.Status != statisticsDomain.RunStatusDataCommitted {
		t.Fatalf("Redis-down publish: run=%+v err=%v", failed, err)
	}
	overloadedModule, err := statisticsModule.New(statisticsModule.Deps{
		MySQLDB: gormDB, MongoDB: mongoDB, RedisClient: runtimeRedis,
		LockRunner: coldStartLockRunner{}, MySQLLimiter: overloadedReadLimiter{}, QueryTTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := overloadedModule.ReadService.Overview(t.Context(), orgID, statisticsApp.QueryFilter{}); !componenterrors.IsCode(err, code.ErrStatisticsOverloaded) {
		t.Fatalf("overloaded read error=%v", err)
	}

	if stored, err := fixture.answerSheets.FindByID(t.Context(), fixture.answerSheetID); err != nil || stored == nil {
		t.Fatalf("AnswerSheet changed by Statistics failure: stored=%v err=%v", stored != nil, err)
	}
	if stored, err := fixture.outcomes.FindByID(t.Context(), fixture.outcomeID); err != nil || stored == nil {
		t.Fatalf("Outcome changed by Statistics failure: stored=%v err=%v", stored != nil, err)
	}
}

type statisticsColdStartFixture struct {
	answerSheets  *mongoAnswersheet.Repository
	outcomes      outcomeDomain.Repository
	answerSheetID meta.ID
	outcomeID     meta.ID
}

func seedStatisticsColdStartBusinessFacts(t *testing.T, db *gorm.DB, mongoDB *mongo.Database, orgID int64, eventAt time.Time) statisticsColdStartFixture {
	t.Helper()
	ctx := t.Context()

	testee := testeeDomain.NewTestee(orgID, "cold-start-testee", testeeDomain.GenderUnknown, nil)
	if err := mysqlActor.NewTesteeRepository(db).Save(ctx, testee); err != nil {
		t.Fatal(err)
	}
	clinician := clinicianDomain.NewClinician(orgID, nil, "cold-start-clinician", "test", "doctor", clinicianDomain.TypeDoctor, "cold-start", true)
	if err := mysqlActor.NewClinicianRepository(db).Save(ctx, clinician); err != nil {
		t.Fatal(err)
	}
	entry := assessmententryDomain.NewAssessmentEntry(orgID, clinician.ID(), "statistics-cold-start", assessmententryDomain.TargetTypeScale, "S-COLD", "v1", true, nil)
	if err := mysqlActor.NewAssessmentEntryRepository(db).Save(ctx, entry); err != nil {
		t.Fatal(err)
	}
	activityRepo := mysqlActor.NewAssessmentEntryActivityLogRepository(db)
	if err := mysqlActor.NewAssessmentEntryResolveLogger(activityRepo).LogResolve(ctx, orgID, clinician.ID().Uint64(), entry.ID().Uint64(), eventAt); err != nil {
		t.Fatal(err)
	}
	if err := mysqlActor.NewAssessmentEntryIntakeLogger(activityRepo).LogIntake(ctx, orgID, clinician.ID().Uint64(), entry.ID().Uint64(), testee.ID().Uint64(), eventAt.Add(time.Minute), true, true); err != nil {
		t.Fatal(err)
	}

	plan, err := planDomain.NewAssessmentPlan(orgID, "S-COLD", planDomain.PlanScheduleByDay, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := mysqlPlan.NewPlanRepository(db).Save(ctx, plan); err != nil {
		t.Fatal(err)
	}
	enrollment := planDomain.NewEnrollment(orgID, plan.GetID(), testee.ID(), 1, eventAt, eventAt.Add(2*time.Minute))
	if err := mysqlPlan.NewEnrollmentRepository(db).Save(ctx, enrollment); err != nil {
		t.Fatal(err)
	}
	task := planDomain.NewAssessmentTask(plan.GetID(), 1, orgID, testee.ID(), "S-COLD", eventAt.Add(3*time.Minute))
	task.AssignEnrollment(enrollment.ID())
	if err := mysqlPlan.NewTaskRepository(db).Save(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := db.Table("assessment_task").Where("id=?", task.GetID().Uint64()).Update("created_at", eventAt.Add(3*time.Minute)).Error; err != nil {
		t.Fatal(err)
	}

	questionnaireRef, err := answersheetDomain.NewQuestionnaireRef("Q-COLD", "v1", "cold-start questionnaire")
	if err != nil {
		t.Fatal(err)
	}
	answer, err := answersheetDomain.NewAnswer(meta.NewCode("Q1"), questionnaireDomain.TypeText, answersheetDomain.NewStringValue("ok"), 0)
	if err != nil {
		t.Fatal(err)
	}
	admission, err := answersheetDomain.NewAssessmentAdmission("Q-COLD", "v1", "scale", "", "", "S-COLD", "v1", "cold-start scale")
	if err != nil {
		t.Fatal(err)
	}
	attribution, err := answersheetDomain.NewAttributionSnapshot(
		answersheetDomain.OriginRef{Type: answersheetDomain.OriginTypePlanTask, ID: task.GetID().String()},
		clinician.ID().String(), entry.ID().String(), plan.GetID().String(), enrollment.ID().String(), task.GetID().String(), eventAt,
	)
	if err != nil {
		t.Fatal(err)
	}
	submission, err := answersheetDomain.NewSubmissionContextWithAttribution(
		actorDomain.NewFillerRef(900001, actorDomain.FillerTypeSelf), actorDomain.NewTesteeRef(testee.ID()),
		meta.FromUint64(uint64(orgID)), task.GetID().String(), attribution, admission,
	)
	if err != nil {
		t.Fatal(err)
	}
	answerSheetID := meta.New()
	answerSheet, err := answersheetDomain.Submit(answerSheetID, questionnaireRef, submission, []answersheetDomain.Answer{answer}, eventAt.Add(4*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	answerSheets, err := mongoAnswersheet.NewRepository(mongoDB)
	if err != nil {
		t.Fatal(err)
	}
	if err := answerSheets.Create(ctx, answerSheet); err != nil {
		t.Fatal(err)
	}

	assessment, err := assessmentDomain.NewAssessment(
		orgID, testee.ID(), assessmentDomain.NewQuestionnaireRefByCode(meta.NewCode("Q-COLD"), "v1"),
		assessmentDomain.NewAnswerSheetRef(answerSheetID), assessmentDomain.NewPlanOrigin(plan.GetID().String()),
		assessmentDomain.WithEvaluationModel(assessmentDomain.NewScaleEvaluationModelRef(meta.New(), meta.NewCode("S-COLD"), "v1", "cold-start scale")),
	)
	if err != nil {
		t.Fatal(err)
	}
	assessments := mysqlEvaluation.NewAssessmentRepository(db)
	if err := assessments.Save(ctx, assessment); err != nil {
		t.Fatal(err)
	}
	if err := db.Table("assessment").Where("id=?", assessment.ID().Uint64()).Update("created_at", eventAt.Add(5*time.Minute)).Error; err != nil {
		t.Fatal(err)
	}

	outcomeID := meta.New()
	outcome, err := outcomeDomain.NewRecord(outcomeDomain.NewRecordInput{
		ID: outcomeID, OrgID: orgID, AssessmentID: assessment.ID(), TesteeID: testee.ID().Uint64(), RunID: "cold-start-evaluation-run",
		Model:   outcomeDomain.ModelIdentity{Kind: modelcatalogDomain.KindScale, Code: "S-COLD", Version: "v1", Title: "cold-start scale"},
		Runtime: outcomeDomain.RuntimeIdentity{AlgorithmFamily: modelcatalogDomain.AlgorithmFamilyFactorScoring, DecisionKind: modelcatalogDomain.DecisionKindScoreRange},
		Payload: []byte(`{"status":"ok"}`), SchemaVersion: outcomeDomain.CurrentSchemaVersion, EvaluatedAt: eventAt.Add(6 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	outcomes := mysqlEvaluation.NewOutcomeRepository(db)
	if err := outcomes.Save(ctx, outcome); err != nil {
		t.Fatal(err)
	}

	report, err := reportDomain.RestoreInterpretReport(reportDomain.InterpretReportInput{
		ID: meta.New(), GenerationID: meta.New(), OutcomeID: outcomeID, InterpretationRunID: meta.New(),
		Association: reportDomain.Association{OrgID: orgID, AssessmentID: assessment.ID(), TesteeID: testee.ID().Uint64()},
		ReportType:  interpretationPolicy.ReportTypeStandard, TemplateVersion: interpretationPolicy.TemplateVersionV1,
		Content:     reportDomain.Content{Model: reportDomain.ModelIdentity{Kind: "scale", Code: "S-COLD", Version: "v1", Title: "cold-start scale"}},
		GeneratedAt: eventAt.Add(7 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	reports, err := mongoInterpretation.NewReportRepository(mongoDB)
	if err != nil {
		t.Fatal(err)
	}
	if err := reports.Insert(ctx, report); err != nil {
		t.Fatal(err)
	}

	return statisticsColdStartFixture{answerSheets: answerSheets, outcomes: outcomes, answerSheetID: answerSheetID, outcomeID: outcomeID}
}

func deleteStatisticsColdStartRedisKeys(t *testing.T, client *redis.Client, orgID int64) {
	t.Helper()
	pattern := "query:*:statistics:org:" + strconv.FormatInt(orgID, 10) + "*"
	var cursor uint64
	for {
		keys, next, err := client.Scan(context.Background(), cursor, pattern, 100).Result()
		if err != nil {
			t.Errorf("scan Statistics cold-start Redis keys: %v", err)
			return
		}
		if len(keys) > 0 {
			if err := client.Del(context.Background(), keys...).Err(); err != nil {
				t.Errorf("delete Statistics cold-start Redis keys: %v", err)
				return
			}
		}
		cursor = next
		if cursor == 0 {
			return
		}
	}
}

func statisticsColdStartTableCounts(t *testing.T, db *gorm.DB, orgID int64, tables []string) map[string]int64 {
	t.Helper()
	counts := make(map[string]int64, len(tables))
	for _, table := range tables {
		var count int64
		if err := db.Table(table).Where("org_id=?", orgID).Count(&count).Error; err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		counts[table] = count
	}
	return counts
}

func assertStatisticsCountsEqual(t *testing.T, want, got map[string]int64) {
	t.Helper()
	for table, count := range want {
		if got[table] != count {
			t.Fatalf("%s count=%d want %d", table, got[table], count)
		}
	}
}

func mapKeys(values map[string]int64) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func assertStatisticsColdStartRows(t *testing.T, db *gorm.DB, orgID int64) {
	t.Helper()
	for table, minimum := range map[string]int64{
		"statistics_access_fact": 2, "statistics_assessment_fact": 4, "statistics_plan_fact": 2,
		"statistics_access_daily": 1, "statistics_assessment_daily": 1, "statistics_plan_activity_daily": 1,
		"statistics_plan_fulfillment_daily": 1, "statistics_org_snapshot": 1,
	} {
		var count int64
		if err := db.Table(table).Where("org_id=?", orgID).Count(&count).Error; err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count < minimum {
			t.Fatalf("%s rows=%d want at least %d", table, count, minimum)
		}
	}
}
