//go:build integration

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	modelport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/event"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func monitoredProjectionClient(t *testing.T, monitor *event.CommandMonitor) *mongo.Client {
	t.Helper()
	uri := os.Getenv("QS_SERVER_TEST_MONGO_URI")
	if uri == "" {
		t.Fatal("QS_SERVER_TEST_MONGO_URI is required; SKIP is not allowed")
	}
	client, err := mongo.Connect(t.Context(), options.Client().ApplyURI(uri).SetMonitor(monitor))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := client.Disconnect(ctx); err != nil {
			t.Error(err)
		}
	})
	return client
}

func TestProjectionAuditRealMongoPaginationAllRolesAndNonzeroFindings(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	collection := db.Collection("assessment_models")
	docs := make([]interface{}, 0, 8)
	for i, family := range []string{"scale", "typology", "behavioral_rating", "cognitive"} {
		doc := projectionTestDoc(t, family)
		if i%2 == 0 {
			doc["record_role"], doc["revision"] = "head", int64(i+1)
		}
		if family == "typology" {
			doc["deleted_at"] = time.Now()
		}
		docs = append(docs, doc)
	}
	mismatch := projectionTestDoc(t, "scale")
	assets := mismatch["definition_v2"].(bson.M)["interpretation_assets"].(bson.M)
	assets["outcomes"].(bson.A)[0].(bson.M)["summary"] = "NEVER_PRINT_CHANGED_REPORT_TEXT"
	docs = append(docs, mismatch)
	missing := projectionTestDoc(t, "scale")
	delete(missing, "definition_v2")
	docs = append(docs, missing)
	unknown := projectionTestDoc(t, "scale")
	unknown["definition_schema_version"] = "future"
	docs = append(docs, unknown)
	invalid := projectionTestDoc(t, "scale")
	invalid["decision_kind"] = "ability_level"
	docs = append(docs, invalid)
	if _, err := collection.InsertMany(t.Context(), docs); err != nil {
		t.Fatal(err)
	}
	var batches atomic.Int64
	client := monitoredProjectionClient(t, &event.CommandMonitor{Started: func(_ context.Context, event *event.CommandStartedEvent) {
		if event.CommandName == "getMore" {
			batches.Add(1)
		}
		switch event.CommandName {
		case "insert", "update", "delete", "findAndModify", "createIndexes", "create", "drop":
			t.Errorf("audit issued write command %s", event.CommandName)
		}
	}})
	reader := client.Database(db.Name()).Collection("assessment_models")
	result := auditDefinitionProjectionCollection(t.Context(), reader, projectionTestConfig())
	if !result.Complete || result.ZeroMismatch || result.exitCode() != 2 || result.Expected != 8 || result.Scanned != 8 || result.Compared != 6 || result.Matched != 4 || result.Mismatched != 1 || result.MissingDefinition != 1 || result.UnknownSchema != 1 || result.Invalid != 1 || result.RecordErrors != 0 || result.ScanErrors != 0 || result.Historical != 1 {
		t.Fatalf("unexpected counts complete=%t zero=%t expected=%d scanned=%d compared=%d match=%d mismatch=%d missing=%d unknown=%d invalid=%d errors=%d/%d history=%d", result.Complete, result.ZeroMismatch, result.Expected, result.Scanned, result.Compared, result.Matched, result.Mismatched, result.MissingDefinition, result.UnknownSchema, result.Invalid, result.RecordErrors, result.ScanErrors, result.Historical)
	}
	if batches.Load() < 7 {
		t.Fatalf("real cursor pagination getMore=%d want>=7", batches.Load())
	}
	encoded, _ := json.Marshal(result)
	if bytes.Contains(encoded, []byte("NEVER_PRINT")) {
		t.Fatal("audit output disclosed report copy")
	}
	for _, record := range result.Records {
		if len(record.Fingerprint) != 64 || record.ModelID == "" || record.Revision == "" {
			t.Fatal("record was not bound to identity and revision")
		}
	}
	hashMismatch := projectionTestDoc(t, "scale")
	hashMismatch["source"].(bson.M)[modelport.SourceDefinitionContentHash] = strings.Repeat("0", 64)
	if _, err := collection.InsertOne(t.Context(), hashMismatch); err != nil {
		t.Fatal(err)
	}
	withHashFinding := auditDefinitionProjectionCollection(t.Context(), reader, projectionTestConfig())
	if !withHashFinding.Complete || withHashFinding.Scanned != 9 || withHashFinding.Matched != 5 || withHashFinding.Mismatched != 1 || withHashFinding.Invalid != 1 || withHashFinding.HashMetadataInvalid != 1 || withHashFinding.HashMetadataUnverifiable != 2 || withHashFinding.ZeroMismatch || withHashFinding.exitCode() != 2 {
		t.Fatalf("hash finding was merged into or hidden by asset counts: %+v", withHashFinding)
	}
	bounded := projectionTestConfig()
	bounded.maxRecords = 3
	partial := auditDefinitionProjectionCollection(t.Context(), reader, bounded)
	if partial.Complete || partial.ZeroMismatch || !partial.Truncated || partial.Scanned != 3 || partial.exitCode() != 3 {
		t.Fatalf("bounded scan accepted complete=%t zero=%t scanned=%d code=%d", partial.Complete, partial.ZeroMismatch, partial.Scanned, partial.exitCode())
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	failure := auditDefinitionProjectionCollection(cancelled, reader, projectionTestConfig())
	if failure.ScanErrors != 1 || failure.Complete || failure.ZeroMismatch || failure.exitCode() != 1 {
		t.Fatal("cancelled real scan reported zero")
	}
}

func TestProjectionAuditRealMongoSnapshotBindsConcurrentChanges(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	collection := db.Collection("assessment_models")
	original := projectionTestDoc(t, "scale")
	if _, err := collection.InsertOne(t.Context(), original); err != nil {
		t.Fatal(err)
	}
	var before bson.Raw
	if err := collection.FindOne(t.Context(), bson.M{"_id": original["_id"]}).Decode(&before); err != nil {
		t.Fatal(err)
	}
	expectedFingerprint := sha256.Sum256(before)
	var changed atomic.Bool
	client := monitoredProjectionClient(t, &event.CommandMonitor{Started: func(_ context.Context, event *event.CommandStartedEvent) {
		if event.CommandName != "find" || !changed.CompareAndSwap(false, true) {
			return
		}
		// The count has already established the snapshot. Mutate through the
		// fixture's separate client before the first audit find reaches Mongo.
		writerCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		if _, err := collection.UpdateOne(writerCtx, bson.M{"_id": original["_id"]}, bson.M{"$set": bson.M{"definition_v2.interpretation_assets.outcomes.0.summary": "NEVER_PRINT_CONCURRENT_COPY"}}); err != nil {
			t.Error(err)
		}
		if _, err := collection.InsertOne(writerCtx, projectionTestDoc(t, "typology")); err != nil {
			t.Error(err)
		}
	}})
	reader := client.Database(db.Name()).Collection("assessment_models")
	result := auditDefinitionProjectionCollection(t.Context(), reader, projectionTestConfig())
	if !changed.Load() || !result.Complete || !result.ZeroMismatch || result.Expected != 1 || result.Scanned != 1 || result.exitCode() != 0 {
		t.Fatalf("snapshot did not bind one cluster time: changed=%t complete=%t zero=%t expected=%d scanned=%d", changed.Load(), result.Complete, result.ZeroMismatch, result.Expected, result.Scanned)
	}
	if result.Records[0].Fingerprint != hex.EncodeToString(expectedFingerprint[:]) {
		t.Fatal("fingerprint bound to concurrent replacement instead of read snapshot")
	}
	if count, err := collection.CountDocuments(t.Context(), bson.D{}); err != nil || count != 2 {
		t.Fatalf("concurrent writes were not real count=%d err=%v", count, err)
	}
	if encoded, _ := json.Marshal(result); strings.Contains(string(encoded), "NEVER_PRINT") {
		t.Fatal("snapshot audit disclosed copy")
	}
}

func TestProjectionAuditRealMongoEmptyAndCLIReadOnlyResult(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	if err := db.CreateCollection(t.Context(), "assessment_models"); err != nil {
		t.Fatal(err)
	}
	cfg := projectionTestConfig()
	cfg.mongoURI = os.Getenv("QS_SERVER_TEST_MONGO_URI")
	cfg.mongoDB = db.Name()
	var output bytes.Buffer
	if code := runDefinitionProjectionAudit(cfg, &output); code != 2 {
		t.Fatalf("empty CLI audit code=%d", code)
	}
	var empty projectionReport
	if err := json.Unmarshal(output.Bytes(), &empty); err != nil {
		t.Fatal(err)
	}
	if !empty.Complete || empty.ZeroMismatch || empty.Reason != "empty_collection" {
		t.Fatal("empty collection reported zero mismatch")
	}
	if _, err := db.Collection("assessment_models").InsertOne(t.Context(), projectionTestDoc(t, "scale")); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if code := runDefinitionProjectionAudit(cfg, &output); code != 0 {
		t.Fatalf("complete matching CLI audit code=%d", code)
	}
	if bytes.Contains(output.Bytes(), []byte("NEVER_PRINT")) {
		t.Fatal("CLI disclosed report copy")
	}
}
