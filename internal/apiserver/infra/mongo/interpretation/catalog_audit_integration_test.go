package interpretation

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	base "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestReportCatalogAuditIndexesAndBatchBoundAgainstMongo(t *testing.T) {
	db := openCatalogAuditMongoContractDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	artifactCollection := db.Collection((InterpretReportPO{}).CollectionName())
	catalogCollection := db.Collection((ReportCatalogPO{}).CollectionName())
	if _, err := artifactCollection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "deleted_at", Value: 1}, {Key: "assessment_id", Value: 1}, {Key: "generated_at", Value: -1}, {Key: "domain_id", Value: -1}},
		Options: options.Index().SetName(IndexCatalogAuditArtifact),
	}); err != nil {
		t.Fatalf("create artifact audit index: %v", err)
	}
	if _, err := catalogCollection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "assessment_id", Value: 1}},
		Options: options.Index().SetName("uk_report_catalog_assessment").SetUnique(true),
	}); err != nil {
		t.Fatalf("create catalog assessment index: %v", err)
	}

	baseAssessmentID := meta.New().Uint64()
	now := time.Now().UTC().Truncate(time.Millisecond)
	documents := make([]interface{}, 0, 250)
	for offset := uint64(1); offset <= 250; offset++ {
		documents = append(documents, InterpretReportPO{
			BaseDocument: base.BaseDocument{
				DomainID:  meta.FromUint64(baseAssessmentID + 1000 + offset),
				CreatedAt: now, UpdatedAt: now,
			},
			GeneratedAt: now, OrgID: 7, AssessmentID: baseAssessmentID + offset, TesteeID: baseAssessmentID + 5000 + offset,
		})
	}
	if _, err := artifactCollection.InsertMany(ctx, documents); err != nil {
		t.Fatalf("insert audit candidates: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		rangeFilter := bson.M{"$gt": baseAssessmentID, "$lte": baseAssessmentID + 250}
		_, _ = artifactCollection.DeleteMany(cleanupCtx, bson.M{"assessment_id": rangeFilter})
		_, _ = catalogCollection.DeleteMany(cleanupCtx, bson.M{"assessment_id": rangeFilter})
	})

	store := &CatalogReconcileStore{db: db}
	if err := store.VerifyAuditIndexes(ctx); err != nil {
		t.Fatalf("verify audit indexes: %v", err)
	}
	assertArtifactAuditExplainUsesIndex(t, ctx, db, baseAssessmentID, baseAssessmentID+250)

	request := CatalogAuditBatchRequest{
		Phase: CatalogAuditPhaseMissing, AfterAssessmentID: baseAssessmentID,
		UpperAssessmentID: baseAssessmentID + 250, Limit: 200, MaxTime: 3 * time.Second,
	}
	first, err := store.ScanAuditBatch(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Scanned != 200 || first.Counts.Missing != 200 || first.Exhausted || first.NextAssessmentID != baseAssessmentID+200 {
		t.Fatalf("first bounded batch = %#v", first)
	}
	request.AfterAssessmentID = first.NextAssessmentID
	second, err := store.ScanAuditBatch(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if second.Scanned != 50 || second.Counts.Missing != 50 || !second.Exhausted || second.NextAssessmentID != baseAssessmentID+250 {
		t.Fatalf("second bounded batch = %#v", second)
	}

}

func openCatalogAuditMongoContractDB(t *testing.T) *mongo.Database {
	t.Helper()
	uri := os.Getenv("QS_SERVER_TEST_MONGO_URI")
	if uri == "" {
		message := "QS_SERVER_TEST_MONGO_URI is not set; skipping report catalog audit Mongo contract test. " +
			"Coverage: required index verification, execution plan, and 200-candidate batch bound. " +
			"Run: QS_SERVER_TEST_MONGO_URI='mongodb://127.0.0.1:27017' QS_SERVER_TEST_MONGO_DB='qs_server_contract_test' " +
			"go test ./internal/apiserver/infra/mongo/interpretation -run TestReportCatalogAuditIndexesAndBatchBoundAgainstMongo -v"
		fmt.Fprintln(os.Stderr, message)
		t.Skip(message)
	}
	dbName := os.Getenv("QS_SERVER_TEST_MONGO_DB")
	if dbName == "" {
		dbName = "qs_server_contract_test"
	}
	connectCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(connectCtx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("connect mongo test db: %v", err)
	}
	if err := client.Ping(connectCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		t.Fatalf("ping mongo test db: %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	return client.Database(dbName)
}

func assertArtifactAuditExplainUsesIndex(t *testing.T, ctx context.Context, db *mongo.Database, after, upper uint64) {
	t.Helper()
	command := bson.D{{Key: "explain", Value: bson.D{
		{Key: "find", Value: (InterpretReportPO{}).CollectionName()},
		{Key: "filter", Value: bson.M{"deleted_at": nil, "assessment_id": bson.M{"$gt": after, "$lte": upper}}},
		{Key: "sort", Value: bson.D{{Key: "assessment_id", Value: 1}, {Key: "generated_at", Value: -1}, {Key: "domain_id", Value: -1}}},
		{Key: "hint", Value: IndexCatalogAuditArtifact},
		{Key: "limit", Value: int64(200)},
	}}, {Key: "verbosity", Value: "executionStats"}}
	var result bson.M
	if err := db.RunCommand(ctx, command).Decode(&result); err != nil {
		t.Fatalf("explain artifact audit query: %v", err)
	}
	encoded, err := bson.MarshalExtJSON(result, false, false)
	if err != nil {
		t.Fatalf("marshal explain result: %v", err)
	}
	if !strings.Contains(string(encoded), IndexCatalogAuditArtifact) {
		t.Fatalf("explain did not use %s: %s", IndexCatalogAuditArtifact, encoded)
	}
	executionStats, ok := result["executionStats"].(bson.M)
	if !ok {
		t.Fatalf("explain executionStats missing: %s", encoded)
	}
	nReturned, ok := numericUint64(executionStats["nReturned"])
	if !ok || nReturned > 200 {
		t.Fatalf("explain nReturned = %v, want <= 200 (%s)", executionStats["nReturned"], fmt.Sprintf("index=%s", IndexCatalogAuditArtifact))
	}
}
