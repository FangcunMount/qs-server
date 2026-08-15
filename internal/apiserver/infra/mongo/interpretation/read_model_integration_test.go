package interpretation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/generation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	evaluationreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
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
		InterpretReportPO{
			BaseDocument: base.BaseDocument{
				ID:        ids[0],
				DomainID:  meta.FromUint64(baseID + 101),
				CreatedAt: now.Add(-3 * time.Minute),
				UpdatedAt: now.Add(-3 * time.Minute),
			},
			GeneratedAt: now.Add(-3 * time.Minute), OrgID: 1, AssessmentID: baseID + 1,
			ScaleName:  "抑郁自评",
			ScaleCode:  scaleCode,
			TesteeID:   testeeID,
			TotalScore: 90,
			RiskLevel:  "high",
			Conclusion: "高风险",
		},
		InterpretReportPO{
			BaseDocument: base.BaseDocument{
				ID:        ids[1],
				DomainID:  meta.FromUint64(baseID + 102),
				CreatedAt: now.Add(-1 * time.Minute),
				UpdatedAt: now.Add(-1 * time.Minute),
			},
			GeneratedAt: now.Add(-1 * time.Minute), OrgID: 1, AssessmentID: baseID + 2,
			ScaleName:  "抑郁自评",
			ScaleCode:  scaleCode,
			TesteeID:   testeeID,
			TotalScore: 55,
			RiskLevel:  "medium",
			Conclusion: "中风险",
		},
		InterpretReportPO{
			BaseDocument: base.BaseDocument{
				ID:        ids[2],
				DomainID:  meta.FromUint64(baseID + 103),
				CreatedAt: now.Add(-2 * time.Minute),
				UpdatedAt: now.Add(-2 * time.Minute),
			},
			GeneratedAt: now.Add(-2 * time.Minute), OrgID: 1, AssessmentID: baseID + 3,
			ScaleName:  "焦虑自评",
			ScaleCode:  otherScaleCode,
			TesteeID:   testeeID,
			TotalScore: 95,
			RiskLevel:  "severe",
			Conclusion: "严重风险",
		},
		InterpretReportPO{
			BaseDocument: base.BaseDocument{
				ID:        ids[3],
				DomainID:  meta.FromUint64(baseID + 104),
				CreatedAt: now,
				UpdatedAt: now,
			},
			GeneratedAt: now, OrgID: 1, AssessmentID: baseID + 4,
			ScaleName:  "抑郁自评",
			ScaleCode:  scaleCode,
			TesteeID:   testeeID + 1,
			TotalScore: 91,
			RiskLevel:  "high",
			Conclusion: "其他受试者",
		},
	}

	collection := db.Collection((InterpretReportPO{}).CollectionName())
	catalog := db.Collection((ReportCatalogPO{}).CollectionName())
	if _, err := collection.InsertMany(ctx, docs); err != nil {
		t.Fatalf("insert reports: %v", err)
	}
	catalogDocs := make([]interface{}, 0, len(docs))
	for _, raw := range docs {
		po := raw.(InterpretReportPO)
		catalogDocs = append(catalogDocs, ReportCatalogPO{AssessmentID: po.AssessmentID, OrgID: po.OrgID, TesteeID: po.TesteeID, SourceKind: ReportCatalogSourceArtifact, SourceID: po.DomainID.Uint64(), ModelCode: po.ScaleCode, RiskLevel: po.RiskLevel, SortAt: po.GeneratedAt, SortReportID: po.DomainID.Uint64()})
	}
	if _, err := catalog.InsertMany(ctx, catalogDocs); err != nil {
		t.Fatalf("insert report catalog: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = collection.DeleteMany(cleanupCtx, bson.M{"_id": bson.M{"$in": ids}})
		_, _ = catalog.DeleteMany(cleanupCtx, bson.M{"assessment_id": bson.M{"$gte": baseID + 1, "$lte": baseID + 4}})
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

func TestReportReadModelFailsClosedOnArtifactAssociationMismatchAgainstMongo(t *testing.T) {
	db := openEvaluationMongoContractDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	baseID := uint64(time.Now().UnixNano() / int64(time.Millisecond))
	catalogAssessment := baseID + 1
	foreignAssessment := baseID + 2
	testeeID := baseID + 1000
	foreignTestee := baseID + 1001
	now := time.Now().UTC().Truncate(time.Second)
	reportID := primitive.NewObjectID()
	reportDomainID := baseID + 101

	reportCollection := db.Collection((InterpretReportPO{}).CollectionName())
	catalogCollection := db.Collection((ReportCatalogPO{}).CollectionName())

	// Catalog authorizes assessment A / testee T, but source body belongs to assessment B / testee U.
	reportPO := InterpretReportPO{
		BaseDocument:        base.BaseDocument{ID: reportID, DomainID: meta.FromUint64(reportDomainID), CreatedAt: now, UpdatedAt: now},
		GenerationID:        baseID + 201,
		OutcomeID:           baseID + 301,
		InterpretationRunID: baseID + 401,
		ReportType:          "standard",
		TemplateVersion:     "v1",
		GeneratedAt:         now,
		OrgID:               99,
		AssessmentID:        foreignAssessment,
		TesteeID:            foreignTestee,
		ScaleCode:           "SDS",
		Conclusion:          "SECRET_FOREIGN_BODY",
		TotalScore:          77,
	}
	if _, err := reportCollection.InsertOne(ctx, reportPO); err != nil {
		t.Fatalf("insert mismatched artifact: %v", err)
	}
	if _, err := catalogCollection.InsertOne(ctx, ReportCatalogPO{
		AssessmentID: catalogAssessment,
		OrgID:        1,
		TesteeID:     testeeID,
		SourceKind:   ReportCatalogSourceArtifact,
		SourceID:     reportDomainID,
		ModelCode:    "SDS",
		SortAt:       now,
		SortReportID: reportDomainID,
	}); err != nil {
		t.Fatalf("insert catalog: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, _ = reportCollection.DeleteOne(cleanupCtx, bson.M{"_id": reportID})
		_, _ = catalogCollection.DeleteMany(cleanupCtx, bson.M{"assessment_id": catalogAssessment})
	})

	reader := NewReportReadModel(db)
	assertAssociationMismatchFailClosed(t, reader, ctx, catalogAssessment, testeeID, ReportCatalogSourceArtifact, reportDomainID, "SECRET_FOREIGN_BODY")
}

func assertAssociationMismatchFailClosed(
	t *testing.T,
	reader evaluationreadmodel.ReportReader,
	ctx context.Context,
	assessmentID, testeeID uint64,
	sourceKind string,
	sourceID uint64,
	secretBody string,
) {
	t.Helper()

	row, err := reader.GetReportByAssessmentID(ctx, assessmentID)
	if row != nil {
		t.Fatalf("detail must not return body on mismatch: %#v", row)
	}
	var mismatch *evaluationreadmodel.CatalogSourceAssociationMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("detail err = %v, want CatalogSourceAssociationMismatchError", err)
	}
	if mismatch.AssessmentID != assessmentID || mismatch.SourceKind != sourceKind || mismatch.SourceID != sourceID {
		t.Fatalf("detail mismatch identity = %#v", mismatch)
	}
	if err != nil && strings.Contains(err.Error(), secretBody) {
		t.Fatalf("detail error leaked report body: %v", err)
	}

	rows, _, err := reader.ListReports(ctx, evaluationreadmodel.ReportFilter{TesteeID: &testeeID}, evaluationreadmodel.PageRequest{Page: 1, PageSize: 10})
	if len(rows) != 0 {
		t.Fatalf("list must not return body on mismatch: %#v", rows)
	}
	if !errors.As(err, &mismatch) {
		t.Fatalf("list err = %v, want CatalogSourceAssociationMismatchError", err)
	}
	if mismatch.AssessmentID != assessmentID || mismatch.SourceKind != sourceKind || mismatch.SourceID != sourceID {
		t.Fatalf("list mismatch identity = %#v", mismatch)
	}
	if err != nil && strings.Contains(err.Error(), secretBody) {
		t.Fatalf("list error leaked report body: %v", err)
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
