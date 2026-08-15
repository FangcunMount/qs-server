//go:build integration

package mongoconsistency

import (
	"context"
	"testing"
	"time"

	appaudit "github.com/FangcunMount/qs-server/internal/apiserver/application/mongoconsistency"
	answersheet "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/answersheet"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestScannerDetectsEverySupportedDriftAgainstReplicaSet(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	insertAuditDriftFixtures(t, ctx, db)
	stats := scanAllPhases(t, ctx, db)
	for kind := range appaudit.DriftSeverities {
		if stats.Findings[kind] == 0 {
			t.Errorf("drift %s was not detected; findings=%v", kind, stats.Findings)
		}
	}
}

func TestScannerAcceptsMarkedHealthyTransactionsAgainstReplicaSet(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	mustInsertMany(t, ctx, db.Collection("answersheets"), []any{
		bson.M{"domain_id": uint64(101), "durable_acceptance": bson.M{"schema_version": 1, "event_id": "evt-sheet-101"}},
	})
	mustInsertMany(t, ctx, db.Collection("domain_event_outbox"), []any{
		bson.M{"event_id": "evt-sheet-101", "event_type": answersheet.EventTypeSubmitted, "aggregate_type": answersheet.AggregateType, "aggregate_id": "101"},
	})
	mustInsertMany(t, ctx, db.Collection("report_generations"), []any{
		bson.M{"domain_id": uint64(201), "transaction_schema_version": 1, "status": "pending"},
	})

	stats := scanAllPhases(t, ctx, db)
	if stats.Total() != 0 {
		t.Fatalf("healthy marked transaction fixtures produced drift: %#v", stats)
	}
}

func insertAuditDriftFixtures(t *testing.T, ctx context.Context, db *mongo.Database) {
	t.Helper()
	mustInsertMany(t, ctx, db.Collection("answersheets"), []any{
		bson.M{"domain_id": uint64(101), "durable_acceptance": bson.M{"schema_version": 1, "event_id": "evt-sheet-missing"}},
	})
	mustInsertMany(t, ctx, db.Collection("domain_event_outbox"), []any{
		bson.M{"event_id": "evt-orphan", "event_type": answersheet.EventTypeSubmitted, "aggregate_type": answersheet.AggregateType, "aggregate_id": "102"},
	})
	mustInsertMany(t, ctx, db.Collection("report_generations"), []any{
		bson.M{"domain_id": uint64(201), "transaction_schema_version": 1, "status": "generating", "latest_run_id": uint64(0)},
		bson.M{"domain_id": uint64(202), "transaction_schema_version": 1, "status": "generating", "latest_run_id": uint64(302)},
		bson.M{"domain_id": uint64(203), "transaction_schema_version": 1, "status": "generated", "latest_run_id": uint64(303), "report_id": uint64(403)},
	})
	mustInsertMany(t, ctx, db.Collection("interpretation_runs"), []any{
		bson.M{"domain_id": uint64(302), "generation_id": uint64(999), "status": "failed"},
		bson.M{"domain_id": uint64(303), "generation_id": uint64(203), "status": "succeeded"},
		bson.M{"domain_id": uint64(304), "generation_id": uint64(204), "status": "failed", "retry_event_id": "evt-retry-missing"},
	})
	mustInsertMany(t, ctx, db.Collection("assessment_models"), []any{
		bson.M{"domain_id": uint64(801), "record_role": "head", "status": "published", "code": "missing-active", "kind": "scale", "algorithm": "scale_default", "questionnaire_code": "q1", "questionnaire_version": "v1"},
		bson.M{"domain_id": uint64(802), "record_role": "head", "status": "published", "code": "bad-active", "kind": "scale", "algorithm": "scale_default", "questionnaire_code": "q2", "questionnaire_version": "v1"},
		bson.M{
			"record_role": "published_snapshot", "release_status": "active", "status": "published",
			"code": "bad-active", "release_version": "v1", "schema_version": "v2",
			"kind": "typology", "algorithm": "personality_typology", "decision_kind": "score_range",
			"questionnaire_code": "missing-q", "questionnaire_version": "v9",
			"definition_v2": bson.M{"calibration": bson.M{"norm_refs": []any{bson.M{"factor_code": "total", "norm_table_version": "missing-norm"}}}},
			"source":        bson.M{"definition_hash": "wrong"},
		},
	})
}

func scanAllPhases(t *testing.T, ctx context.Context, db *mongo.Database) appaudit.Statistics {
	t.Helper()
	stats := appaudit.NewStatistics()
	scanner := NewScanner(db, nil)
	for _, phase := range appaudit.AuditPhases {
		upper, err := scanner.UpperBound(ctx, phase, 3*time.Second)
		if err != nil {
			t.Fatalf("upper bound %s: %v", phase, err)
		}
		result, err := scanner.ScanBatch(ctx, appaudit.BatchRequest{
			Phase: phase, UpperBound: upper, Limit: 200, MaxTime: 3 * time.Second, MaxSamples: 10,
		})
		if err != nil {
			t.Fatalf("scan %s: %v", phase, err)
		}
		if !result.Exhausted {
			t.Fatalf("single fixture batch did not exhaust phase %s", phase)
		}
		stats.Add(result, 10)
	}
	return stats
}

func mustInsertMany(t *testing.T, ctx context.Context, collection *mongo.Collection, docs []any) {
	t.Helper()
	if _, err := collection.InsertMany(ctx, docs); err != nil {
		t.Fatalf("insert %s fixtures: %v", collection.Name(), err)
	}
}
