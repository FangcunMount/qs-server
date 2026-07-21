package codec

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestDecodeExecutionRejectsNonCurrentSchemas(t *testing.T) {
	for _, schema := range []uint{0, 1, 99} {
		record := evaluationfact.NewRecord(evaluationfact.NewRecordInput{ID: meta.FromUint64(1), SchemaVersion: schema, Payload: []byte(`{}`)})
		if _, err := DecodeExecution(record); err == nil {
			t.Fatalf("schema %d: expected unsupported schema error", schema)
		}
	}
}

func TestDecodeExecutionReadsSchemaV2ClassificationFacts(t *testing.T) {
	record := evaluationfact.NewRecord(evaluationfact.NewRecordInput{
		ID: meta.FromUint64(12), SchemaVersion: 2,
		Model:   evaluationfact.ModelIdentity{Kind: modelcatalog.KindTypology, Algorithm: modelcatalog.AlgorithmPersonalityTypology, Code: "MBTI"},
		Runtime: evaluationfact.RuntimeIdentity{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification, DecisionKind: modelcatalog.DecisionKindPoleComposition},
		Payload: []byte(`{"Detail":{"Payload":{"type_code":"INTJ","match_percent":80,"special_trigger":"stable"}},"Dimensions":[{"Code":"EI","Score":{"Value":8},"Preference":"I"}]}`),
	})
	execution, err := DecodeExecution(record)
	if err != nil {
		t.Fatal(err)
	}
	fact, ok := ClassificationFactFromPayload(execution.Detail.Payload)
	if !ok || fact.TypeCode != "INTJ" || fact.MatchPercent != 80 || fact.SpecialTrigger != "stable" {
		t.Fatalf("classification fact = %#v", execution.Detail.Payload)
	}
	if len(execution.Dimensions) != 1 || execution.Dimensions[0].Preference != "I" {
		t.Fatalf("dimensions = %#v", execution.Dimensions)
	}
}
