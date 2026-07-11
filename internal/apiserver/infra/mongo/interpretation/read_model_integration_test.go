package interpretation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func openEvaluationMongoContractDB(t *testing.T) *mongo.Database {
	t.Helper()

	uri := os.Getenv("QS_SERVER_TEST_MONGO_URI")
	if uri == "" {
		skipEvaluationMongoContract(t)
	}
	dbName := os.Getenv("QS_SERVER_TEST_MONGO_DB")
	if dbName == "" {
		dbName = "qs_server_contract_test"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("connect mongo test db: %v", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		t.Fatalf("ping mongo test db: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Disconnect(context.Background())
	})

	return client.Database(dbName)
}

func skipEvaluationMongoContract(t *testing.T) {
	t.Helper()
	message := "QS_SERVER_TEST_MONGO_URI is not set; skipping Mongo evaluation report read model contract tests. " +
		"Coverage: testee/testeeIDs, high-risk/risk/scale filters, pagination/sort, not-found and legacy nil field mapping. " +
		"Run: QS_SERVER_TEST_MONGO_URI='mongodb://127.0.0.1:27017' QS_SERVER_TEST_MONGO_DB='qs_server_contract_test' " +
		"go test ./internal/apiserver/infra/mongo/interpretation -run 'Integration|AgainstMongo' -v"
	fmt.Fprintln(os.Stderr, message)
	t.Skip(message)
}

func TestReportReadModelListReportsFiltersAgainstMongo(t *testing.T) {
	db := openEvaluationMongoContractDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	baseID := uint64(time.Now().UnixNano() / int64(time.Millisecond))
	testeeID := baseID + 1000
	scaleCode := fmt.Sprintf("scale-report-%d", baseID)
	otherScaleCode := fmt.Sprintf("scale-other-%d", baseID)
	now := time.Now().UTC().Truncate(time.Second)

	ids := []primitive.ObjectID{
		primitive.NewObjectID(),
		primitive.NewObjectID(),
		primitive.NewObjectID(),
		primitive.NewObjectID(),
	}
	docs := []interface{}{
		ArchivedReportPO{
			BaseDocument: base.BaseDocument{
				ID:        ids[0],
				DomainID:  meta.FromUint64(baseID + 1),
				CreatedAt: now.Add(-3 * time.Minute),
				UpdatedAt: now.Add(-3 * time.Minute),
			},
			ScaleName:  "抑郁自评",
			ScaleCode:  scaleCode,
			TesteeID:   testeeID,
			TotalScore: 90,
			RiskLevel:  "high",
			Conclusion: "高风险",
		},
		ArchivedReportPO{
			BaseDocument: base.BaseDocument{
				ID:        ids[1],
				DomainID:  meta.FromUint64(baseID + 2),
				CreatedAt: now.Add(-1 * time.Minute),
				UpdatedAt: now.Add(-1 * time.Minute),
			},
			ScaleName:  "抑郁自评",
			ScaleCode:  scaleCode,
			TesteeID:   testeeID,
			TotalScore: 55,
			RiskLevel:  "medium",
			Conclusion: "中风险",
		},
		ArchivedReportPO{
			BaseDocument: base.BaseDocument{
				ID:        ids[2],
				DomainID:  meta.FromUint64(baseID + 3),
				CreatedAt: now.Add(-2 * time.Minute),
				UpdatedAt: now.Add(-2 * time.Minute),
			},
			ScaleName:  "焦虑自评",
			ScaleCode:  otherScaleCode,
			TesteeID:   testeeID,
			TotalScore: 95,
			RiskLevel:  "severe",
			Conclusion: "严重风险",
		},
		ArchivedReportPO{
			BaseDocument: base.BaseDocument{
				ID:        ids[3],
				DomainID:  meta.FromUint64(baseID + 4),
				CreatedAt: now,
				UpdatedAt: now,
			},
			ScaleName:  "抑郁自评",
			ScaleCode:  scaleCode,
			TesteeID:   testeeID + 1,
			TotalScore: 91,
			RiskLevel:  "high",
			Conclusion: "其他受试者",
		},
	}

	collection := db.Collection((&ArchivedReportPO{}).CollectionName())
	if _, err := collection.InsertMany(ctx, docs); err != nil {
		t.Fatalf("insert reports: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = collection.DeleteMany(cleanupCtx, bson.M{"_id": bson.M{"$in": ids}})
	})

	reader := NewReportReadModel(db)
	highRows, total, err := reader.ListReports(ctx, evaluationreadmodel.ReportFilter{
		TesteeID:     &testeeID,
		HighRiskOnly: true,
		ModelCode:    scaleCode,
	}, evaluationreadmodel.PageRequest{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("list high risk reports: %v", err)
	}
	if total != 1 || len(highRows) != 1 || highRows[0].AssessmentID != baseID+1 {
		t.Fatalf("high risk filtered rows = %#v total=%d, want report %d", highRows, total, baseID+1)
	}

	rows, total, err := reader.ListReports(ctx, evaluationreadmodel.ReportFilter{
		TesteeID: &testeeID,
	}, evaluationreadmodel.PageRequest{Page: 1, PageSize: 2})
	if err != nil {
		t.Fatalf("list reports by testee: %v", err)
	}
	if total != 3 || len(rows) != 2 {
		t.Fatalf("paged rows = %#v total=%d, want 2 of 3", rows, total)
	}
	if rows[0].AssessmentID != baseID+2 || rows[1].AssessmentID != baseID+3 {
		t.Fatalf("rows order = %#v, want created_at desc", rows)
	}

	reportRow, err := reader.GetReportByAssessmentID(ctx, baseID+1)
	if err != nil {
		t.Fatalf("get report by assessment id: %v", err)
	}
	if reportRow.ModelCode != scaleCode || reportRow.RiskLevel != "high" || reportRow.AssessmentID != baseID+1 {
		t.Fatalf("unexpected report row: %#v", reportRow)
	}
}

func TestReportReadModelPrefersCurrentReportsAndFallsBackToArchivesAgainstMongo(t *testing.T) {
	db := openEvaluationMongoContractDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	baseID := uint64(time.Now().UnixNano() / int64(time.Millisecond))
	testeeID := baseID + 1000
	now := time.Now().UTC().Truncate(time.Second)
	archiveCollection := db.Collection((&ArchivedReportPO{}).CollectionName())
	reportCollection := db.Collection((InterpretReportPO{}).CollectionName())
	archiveIDs := []primitive.ObjectID{primitive.NewObjectID(), primitive.NewObjectID()}
	reportID := primitive.NewObjectID()

	archiveDocs := []interface{}{
		ArchivedReportPO{
			BaseDocument: base.BaseDocument{ID: archiveIDs[0], DomainID: meta.FromUint64(baseID + 1), CreatedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute)},
			TesteeID:     testeeID, ScaleCode: "SDS", Conclusion: "archived duplicate", TotalScore: 1,
		},
		ArchivedReportPO{
			BaseDocument: base.BaseDocument{ID: archiveIDs[1], DomainID: meta.FromUint64(baseID + 2), CreatedAt: now.Add(-time.Minute), UpdatedAt: now.Add(-time.Minute)},
			TesteeID:     testeeID, ScaleCode: "SDS", Conclusion: "archived only", TotalScore: 2,
		},
	}
	if _, err := archiveCollection.InsertMany(ctx, archiveDocs); err != nil {
		t.Fatalf("insert archived reports: %v", err)
	}
	reportPO := InterpretReportPO{
		BaseDocument:        base.BaseDocument{ID: reportID, DomainID: meta.FromUint64(baseID + 101), CreatedAt: now, UpdatedAt: now},
		GenerationID:        baseID + 201,
		OutcomeID:           baseID + 301,
		InterpretationRunID: baseID + 401,
		ReportType:          "standard",
		TemplateVersion:     "v1",
		GeneratedAt:         now,
		AssessmentID:        baseID + 1,
		TesteeID:            testeeID,
		ScaleCode:           "SDS",
		Conclusion:          "current report wins",
		TotalScore:          42,
	}
	if _, err := reportCollection.InsertOne(ctx, reportPO); err != nil {
		t.Fatalf("insert current report: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = archiveCollection.DeleteMany(cleanupCtx, bson.M{"_id": bson.M{"$in": archiveIDs}})
		_, _ = reportCollection.DeleteOne(cleanupCtx, bson.M{"_id": reportID})
	})

	reader := NewReportReadModel(db)
	newRow, err := reader.GetReportByAssessmentID(ctx, baseID+1)
	if err != nil || newRow.Conclusion != "current report wins" || newRow.TotalScore != 42 {
		t.Fatalf("new-first report = %#v err=%v", newRow, err)
	}
	archivedRow, err := reader.GetReportByID(ctx, baseID+2)
	if err != nil || archivedRow.Conclusion != "archived only" {
		t.Fatalf("archive fallback report = %#v err=%v", archivedRow, err)
	}
	rows, total, err := reader.ListReports(ctx, evaluationreadmodel.ReportFilter{TesteeID: &testeeID}, evaluationreadmodel.PageRequest{Page: 1, PageSize: 10})
	if err != nil || total != 2 || len(rows) != 2 || rows[0].AssessmentID != baseID+1 || rows[0].Conclusion != "current report wins" {
		t.Fatalf("new-first report list = %#v total=%d err=%v", rows, total, err)
	}
}

func TestGenerationRepositoryRejectsStaleCASAgainstMongo(t *testing.T) {
	db := openEvaluationMongoContractDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo, err := NewGenerationRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	id := meta.FromUint64(uint64(time.Now().UnixNano() / int64(time.Millisecond)))
	generationRecord, err := generation.New(id, generation.Key{
		OutcomeID:       meta.FromUint64(id.Uint64() + 1),
		ReportType:      policy.ReportTypeStandard,
		TemplateVersion: policy.TemplateVersion("cas-v1"),
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, generationRecord); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Collection((ReportGenerationPO{}).CollectionName()).DeleteOne(context.Background(), bson.M{"domain_id": id.Uint64()})
	})
	if err := generationRecord.Begin(meta.FromUint64(id.Uint64()+2), now.Add(time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, generationRecord, 1); err != nil {
		t.Fatalf("first CAS save: %v", err)
	}
	if err := repo.Save(ctx, generationRecord, 1); !errors.Is(err, generation.ErrVersionConflict) {
		t.Fatalf("stale CAS save = %v, want version conflict", err)
	}
}
