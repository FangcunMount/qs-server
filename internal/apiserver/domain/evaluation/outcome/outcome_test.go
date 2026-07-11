package outcome

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestRecordCopiesCanonicalPayload(t *testing.T) {
	t.Parallel()

	payload := []byte(`{"primary":{"value":12}}`)
	record, err := NewRecord(NewRecordInput{
		ID:           meta.FromUint64(1),
		AssessmentID: meta.FromUint64(2),
		RunID:        "2:1",
		Model: ModelIdentity{
			Kind:    modelcatalog.KindScale,
			Code:    "S-1",
			Version: "1.0.0",
			Title:   "scale",
		},
		Runtime: RuntimeIdentity{
			AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
			DecisionKind:    modelcatalog.DecisionKindScoreRange,
		},
		Payload:     payload,
		EvaluatedAt: time.Unix(100, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	payload[0] = '['
	got := record.Payload()
	got[0] = '['
	if string(record.Payload()) != `{"primary":{"value":12}}` {
		t.Fatalf("record payload was mutated: %s", record.Payload())
	}
	if record.SchemaVersion() != CurrentSchemaVersion {
		t.Fatalf("schema version = %d", record.SchemaVersion())
	}
}
