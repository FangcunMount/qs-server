//go:build integration

package main

import (
	"testing"
	"time"

	isoaiexplanation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/interpretation/aiexplanation"
	"github.com/FangcunMount/qs-server/internal/pkg/mongodbtest"
	"go.mongodb.org/mongo-driver/bson"
)

func TestAIExplanationPromptEvaluationSizeAuditReadsRawBSONWithoutReencoding(t *testing.T) {
	_, db := mongodbtest.ReplicaSetDatabase(t)
	collection := db.Collection((isoaiexplanation.PromptEvaluationRunPO{}).CollectionName())
	now := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	generationRaw := []byte(`{}`)
	generationNormalized := []byte(`{"ok":true}`)
	semanticRaw := []byte(`{}`)
	semanticNormalized := []byte(`{"score":5}`)

	_, err := collection.InsertOne(t.Context(), bson.M{
		"domain_id": int64(101), "created_at": now, "updated_at": now,
		"status": "awaiting_review", "version": int64(1), "reviews": bson.A{},
		"attempts": bson.A{bson.M{
			"case_id": "case-1", "attempt": 1, "stage": "generation",
			"started_at": now, "finished_at": now, "provider_call_count": 1,
			"raw_output": generationRaw, "normalized_output": generationNormalized,
			"assertions": bson.A{},
			"semantic_execution": bson.M{
				"invocation_id": "semantic-case-1", "evaluator_version": "v1",
				"started_at": now, "finished_at": now, "provider_call_count": 1,
				"raw_output": semanticRaw, "normalized_output": semanticNormalized,
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := collection.FindOne(t.Context(), bson.M{"domain_id": int64(101)}).Raw()
	if err != nil {
		t.Fatal(err)
	}

	matched, values, err := loadMeasurements(t.Context(), collection, 0)
	if err != nil {
		t.Fatal(err)
	}
	wantPayloadBytes := len(generationRaw) + len(generationNormalized) + len(semanticRaw) + len(semanticNormalized)
	wantWithoutPayloads := int64(len(raw) - wantPayloadBytes)
	if matched != 1 || len(values.runBSON) != 1 || values.runBSON[0] != int64(len(raw)) ||
		len(values.runBSONWithoutOutputPayloads) != 1 || values.runBSONWithoutOutputPayloads[0] != wantWithoutPayloads ||
		values.runBSONWithoutOutputPayloads[0] >= values.runBSON[0] ||
		values.generationExecutions != 1 || values.semanticExecutions != 1 {
		t.Fatalf("raw BSON measurements = matched:%d values:%#v want_without_payloads:%d", matched, values, wantWithoutPayloads)
	}
	report := buildReport(now, collection.Name(), matched, values)
	if !report.V2ObservedOutputProjection.Available || report.RunBSONWithoutOutputPayloads.Max != wantWithoutPayloads {
		t.Fatalf("BSON report = %#v", report)
	}
}
