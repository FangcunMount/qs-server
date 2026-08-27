//go:build integration

package migration

import (
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestAIExplanationMongoColdStartEnforcesRuntimeSchema(t *testing.T) {
	client, db := mongodbtest.ReplicaSetDatabase(t)
	version, changed, err := NewMongoMigrator(client, &Config{Enabled: true, Database: db.Name()}).Run()
	wantVersion := latestEmbeddedMongoMigrationVersion(t)
	if err != nil || !changed || version != wantVersion {
		t.Fatalf("migrate MongoDB 0 -> %d: version=%d changed=%v err=%v", wantVersion, version, changed, err)
	}

	requiredIndexes := map[string][]string{
		"ai_explanation_generations": {
			"uk_ai_explanation_generation_domain",
			"uk_ai_explanation_generation_semantic_key",
			"idx_ai_explanation_generation_assessment_created",
			"ttl_ai_explanation_expires_at",
		},
		"ai_explanation_runs": {
			"uk_ai_explanation_run_domain",
			"uk_ai_explanation_run_attempt",
			"idx_ai_explanation_run_latest",
			"idx_ai_explanation_run_expired_lease",
			"uk_ai_explanation_run_retry_request",
			"uk_ai_explanation_run_retry_event",
			"ttl_ai_explanation_expires_at",
		},
		"ai_explanation_artifacts": {
			"uk_ai_explanation_artifact_domain",
			"uk_ai_explanation_artifact_generation",
			"idx_ai_explanation_artifact_source_audience",
			"idx_ai_explanation_artifact_subject_export",
			"ttl_ai_explanation_expires_at",
		},
		"ai_explanation_profiles": {
			"uk_ai_explanation_profile_domain",
			"uk_ai_explanation_profile_release",
			"idx_ai_explanation_profile_selector",
			"uk_ai_explanation_profile_published_selector_slot",
		},
		"ai_explanation_prompt_evaluations": {
			"uk_ai_explanation_prompt_evaluation_domain",
			"idx_ai_explanation_prompt_evaluation_profile_status",
			"idx_ai_explanation_prompt_evaluation_release",
			"uk_ai_explanation_prompt_evaluation_active_release",
			"uk_ai_explanation_prompt_evaluation_active_org_execution",
			"idx_ai_explanation_prompt_evaluation_expired_lease",
			"ttl_ai_explanation_expires_at",
		},
		"ai_explanation_prompt_evaluation_daily_budgets": {
			"uk_ai_explanation_prompt_evaluation_budget_org_day",
			"uk_ai_explanation_prompt_evaluation_budget_run",
			"ttl_ai_explanation_expires_at",
		},
		"ai_explanation_participant_daily_budgets": {
			"uk_ai_explanation_participant_budget_org_day",
			"uk_ai_explanation_participant_budget_reservation",
			"ttl_ai_explanation_expires_at",
		},
		"ai_explanation_participant_active_capacity": {
			"uk_ai_explanation_participant_active_capacity_org",
			"uk_ai_explanation_participant_active_generation",
			"uk_ai_explanation_participant_active_run",
		},
	}
	for collection, indexes := range requiredIndexes {
		for _, index := range indexes {
			assertMongoIndex(t, db.Collection(collection), index, true)
		}
	}

	for _, collection := range []string{
		"ai_explanation_generations",
		"ai_explanation_runs",
		"ai_explanation_artifacts",
		"ai_explanation_prompt_evaluations",
		"ai_explanation_prompt_evaluation_daily_budgets",
		"ai_explanation_participant_daily_budgets",
	} {
		spec := findAIExplanationMongoIndex(t, db.Collection(collection), "ttl_ai_explanation_expires_at")
		if spec.ExpireAfterSeconds == nil || *spec.ExpireAfterSeconds != 0 {
			t.Fatalf("TTL index %s.ttl_ai_explanation_expires_at expireAfterSeconds=%v, want 0", collection, spec.ExpireAfterSeconds)
		}
		if !reflect.DeepEqual(spec.Key, bson.D{{Key: "expires_at", Value: int32(1)}}) {
			t.Fatalf("TTL index %s key=%v, want expires_at:1", collection, spec.Key)
		}
	}

	exportIndex := findAIExplanationMongoIndex(t, db.Collection("ai_explanation_artifacts"), "idx_ai_explanation_artifact_subject_export")
	wantExportKey := bson.D{
		{Key: "source.association.org_id", Value: int32(1)},
		{Key: "source.association.testee_id", Value: int32(1)},
		{Key: "audience", Value: int32(1)},
		{Key: "generated_at", Value: int32(-1)},
		{Key: "domain_id", Value: int32(-1)},
	}
	if !reflect.DeepEqual(exportIndex.Key, wantExportKey) {
		t.Fatalf("subject export index key=%v, want %v", exportIndex.Key, wantExportKey)
	}

	assertAIExplanationPlaintextPayloadSchema(t, db)
	assertAIExplanationPhysicalUniquenessGates(t, db)
}

func TestAIExplanationMongoTTLMonitorDeletesExpiredRecords(t *testing.T) {
	client, db := mongodbtest.ReplicaSetDatabase(t)
	version, changed, err := NewMongoMigrator(client, &Config{Enabled: true, Database: db.Name()}).Run()
	wantVersion := latestEmbeddedMongoMigrationVersion(t)
	if err != nil || !changed || version != wantVersion {
		t.Fatalf("migrate MongoDB 0 -> %d: version=%d changed=%v err=%v", wantVersion, version, changed, err)
	}

	const probe = "ai-explanation-expired-record"
	expiresAt := time.Now().UTC().Add(-time.Hour)
	generation := aiExplanationGenerationDocument("ttl")
	generation["input_json"] = primitive.Binary{Subtype: 0, Data: []byte(`{"schema":"ai-explanation-input/v1"}`)}
	generation["status"] = "generated"
	generation["expires_at"] = expiresAt
	generation["ttl_probe"] = probe
	artifact := aiExplanationArtifactDocument("ttl")
	artifact["content"] = bson.M{"summary": "expired synthetic artifact"}
	artifact["expires_at"] = expiresAt
	artifact["ttl_probe"] = probe

	records := []struct {
		collection string
		document   bson.M
	}{
		{collection: "ai_explanation_generations", document: generation},
		{collection: "ai_explanation_runs", document: bson.M{
			"domain_id": "run-ttl", "generation_id": "generation-ttl", "attempt": 1,
			"status": "succeeded", "expires_at": expiresAt, "ttl_probe": probe,
		}},
		{collection: "ai_explanation_artifacts", document: artifact},
		{collection: "ai_explanation_prompt_evaluations", document: bson.M{
			"domain_id": "evaluation-ttl", "status": "approved", "expires_at": expiresAt, "ttl_probe": probe,
		}},
		{collection: "ai_explanation_prompt_evaluation_daily_budgets", document: bson.M{
			"org_id": int64(71), "budget_day": "2026-08-26", "expires_at": expiresAt, "ttl_probe": probe,
		}},
		{collection: "ai_explanation_participant_daily_budgets", document: bson.M{
			"org_id": int64(72), "budget_day": "2026-08-26", "expires_at": expiresAt, "ttl_probe": probe,
		}},
	}
	for _, record := range records {
		if _, err := db.Collection(record.collection).InsertOne(t.Context(), record.document); err != nil {
			t.Fatalf("insert expired TTL probe into %s: %v", record.collection, err)
		}
	}

	timeout := 90 * time.Second
	if raw := os.Getenv("QS_SERVER_TEST_MONGO_TTL_TIMEOUT"); raw != "" {
		parsed, parseErr := time.ParseDuration(raw)
		if parseErr != nil || parsed <= 0 {
			t.Fatalf("QS_SERVER_TEST_MONGO_TTL_TIMEOUT=%q must be a positive duration", raw)
		}
		timeout = parsed
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()

	remaining := make([]string, 0, len(records))
	for {
		remaining = remaining[:0]
		for _, record := range records {
			count, countErr := db.Collection(record.collection).CountDocuments(t.Context(), bson.M{"ttl_probe": probe})
			if countErr != nil {
				t.Fatalf("count TTL probe in %s: %v", record.collection, countErr)
			}
			if count != 0 {
				remaining = append(remaining, record.collection)
			}
		}
		if len(remaining) == 0 {
			return
		}
		select {
		case <-t.Context().Done():
			t.Fatalf("wait for AI explanation TTL probes: %v", t.Context().Err())
		case <-deadline.C:
			t.Fatalf("AI explanation TTL monitor did not delete expired records within %s; remaining=%v", timeout, remaining)
		case <-ticker.C:
		}
	}
}

func assertAIExplanationPlaintextPayloadSchema(t *testing.T, db *mongo.Database) {
	t.Helper()
	generation := aiExplanationGenerationDocument("plaintext-schema")
	generation["input_json"] = primitive.Binary{Subtype: 0, Data: []byte(`{"schema":"ai-explanation-input/v1"}`)}
	if _, err := db.Collection("ai_explanation_generations").InsertOne(t.Context(), generation); err != nil {
		t.Fatalf("insert AI explanation Generation with canonical input_json: %v", err)
	}

	artifact := aiExplanationArtifactDocument("plaintext-schema")
	artifact["content"] = bson.M{"summary": "synthetic artifact"}
	if _, err := db.Collection("ai_explanation_artifacts").InsertOne(t.Context(), artifact); err != nil {
		t.Fatalf("insert AI explanation Artifact with structured content: %v", err)
	}
}

func assertAIExplanationPhysicalUniquenessGates(t *testing.T, db *mongo.Database) {
	t.Helper()
	profiles := db.Collection("ai_explanation_profiles")
	if _, err := profiles.InsertOne(t.Context(), bson.M{
		"domain_id":  "profile-published-a",
		"definition": bson.M{"profile_id": "profile-a", "version": "v1"},
		"status":     "published", "selector_slot_key": "participant:scale:scored",
	}); err != nil {
		t.Fatalf("insert first published Profile selector slot: %v", err)
	}
	_, err := profiles.InsertOne(t.Context(), bson.M{
		"domain_id":  "profile-published-b",
		"definition": bson.M{"profile_id": "profile-b", "version": "v1"},
		"status":     "published", "selector_slot_key": "participant:scale:scored",
	})
	if err == nil || !mongo.IsDuplicateKeyError(err) {
		t.Fatalf("duplicate published Profile selector slot error=%v, want duplicate key", err)
	}

	evaluations := db.Collection("ai_explanation_prompt_evaluations")
	if _, err := evaluations.InsertOne(t.Context(), bson.M{
		"domain_id": "evaluation-a", "active_release_key": "release-a", "active_execution_org_key": "org:17",
	}); err != nil {
		t.Fatalf("insert first active Prompt evaluation: %v", err)
	}
	_, err = evaluations.InsertOne(t.Context(), bson.M{
		"domain_id": "evaluation-b", "active_release_key": "release-a", "active_execution_org_key": "org:18",
	})
	if err == nil || !mongo.IsDuplicateKeyError(err) {
		t.Fatalf("duplicate active Prompt release error=%v, want duplicate key", err)
	}
	_, err = evaluations.InsertOne(t.Context(), bson.M{
		"domain_id": "evaluation-c", "active_release_key": "release-c", "active_execution_org_key": "org:17",
	})
	if err == nil || !mongo.IsDuplicateKeyError(err) {
		t.Fatalf("duplicate active Prompt org execution error=%v, want duplicate key", err)
	}
}

func aiExplanationGenerationDocument(id string) bson.M {
	return bson.M{
		"domain_id":                  "generation-" + id,
		"source_report_id":           "report-" + id,
		"audience":                   "participant",
		"profile":                    bson.M{"id": "profile-v1", "version": "v1", "fingerprint": "profile-fingerprint"},
		"input_fingerprint":          "input-" + id,
		"execution_spec_fingerprint": "execution-" + id,
	}
}

func aiExplanationArtifactDocument(id string) bson.M {
	return bson.M{
		"domain_id":     "artifact-" + id,
		"generation_id": "generation-" + id,
		"source":        bson.M{"report_id": "report-" + id},
		"audience":      "participant",
	}
}

type aiExplanationMongoIndex struct {
	Name               string `bson:"name"`
	Key                bson.D `bson:"key"`
	Unique             bool   `bson:"unique,omitempty"`
	Sparse             bool   `bson:"sparse,omitempty"`
	ExpireAfterSeconds *int64 `bson:"expireAfterSeconds,omitempty"`
}

func findAIExplanationMongoIndex(t *testing.T, collection *mongo.Collection, name string) aiExplanationMongoIndex {
	t.Helper()
	cursor, err := collection.Indexes().List(t.Context())
	if err != nil {
		t.Fatalf("list indexes for %s: %v", collection.Name(), err)
	}
	defer cursor.Close(t.Context())
	for cursor.Next(t.Context()) {
		var spec aiExplanationMongoIndex
		if err := cursor.Decode(&spec); err != nil {
			t.Fatalf("decode index for %s: %v", collection.Name(), err)
		}
		if spec.Name == name {
			return spec
		}
	}
	if err := cursor.Err(); err != nil {
		t.Fatalf("iterate indexes for %s: %v", collection.Name(), err)
	}
	t.Fatalf("index %s.%s is missing", collection.Name(), name)
	return aiExplanationMongoIndex{}
}
