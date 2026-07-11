package outcome

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestExecutionIsEvaluatorResultAndRecordIsDurableFact(t *testing.T) {
	model := ModelRef{ModelKind: modelcatalog.KindScale, ModelCode: "S-1", ModelVersion: "1.0.0", ModelTitle: "Scale"}
	execution := NewExecution(
		model,
		Summary{PrimaryLabel: "low"},
		Detail{Kind: modelcatalog.KindScale},
	)
	if execution == nil || execution.ModelRef != model {
		t.Fatalf("execution = %#v, want evaluator result with its model reference", execution)
	}
	payload, err := json.Marshal(execution)
	if err != nil {
		t.Fatalf("marshal Execution: %v", err)
	}
	record, err := NewRecord(NewRecordInput{
		ID:           meta.FromUint64(1),
		AssessmentID: meta.FromUint64(2),
		TesteeID:     3,
		RunID:        "2:1",
		Model:        ModelIdentity{Kind: modelcatalog.KindScale, Code: "S-1", Version: "1.0.0", Title: "Scale"},
		Payload:      payload,
		EvaluatedAt:  time.Unix(100, 0),
	})
	if err != nil {
		t.Fatalf("NewRecord: %v", err)
	}
	var restored Execution
	if err := json.Unmarshal(record.Payload(), &restored); err != nil {
		t.Fatalf("unmarshal Record payload into Execution: %v", err)
	}
	if restored.ModelRef.Code() != model.Code() || restored.Summary.PrimaryLabel != execution.Summary.PrimaryLabel {
		t.Fatalf("restored execution = %#v, want %#v", restored, execution)
	}
}
