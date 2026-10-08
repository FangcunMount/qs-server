package retirement

import (
	"encoding/json"
	"errors"
	sqlevaluation "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/evaluation"
	"testing"
)

func TestSQLResponsibilitySnapshotCannotBecomeSourceApproval(t *testing.T) {
	for _, value := range []any{&SQLResponsibilitySnapshot{}, SQLSourceResponsibilityView{SourceAuthenticationRequired: true}} {
		if _, err := json.Marshal(value); !errors.Is(err, ErrSourceSerialization) {
			t.Fatal("private responsibility view serialized")
		}
	}
	var absent *SQLResponsibilitySnapshot
	if _, err := absent.ForUntrustedSQLSource(t.Context(), nil); err == nil {
		t.Fatal("absent authenticated snapshot accepted")
	}
	if absent.Report().DropReady {
		t.Fatal("zero report granted DROP")
	}
}

func TestSQLResponsibilityOriginalIDCollisionCannotBeHiddenByOtherScope(t *testing.T) {
	source := &DecodedSourceEvent{EventID: "old-id", EventType: "evaluation.requested", AggregateType: "Evaluation", AggregateID: "42"}
	for _, row := range []sqlevaluation.SQLResponsibilityObservation{{EventID: "old-id", EventType: "task.opened.reminder.requested", OwnerKind: "AssessmentTask", OwnerID: "task-1", ScopeClass: "scope_outside_retirement"}, {EventID: "old-id", EventType: "evaluation.requested", OwnerKind: "Evaluation", OwnerID: "43"}} {
		if !sqlResponsibilitySourceCollision(row, source) {
			t.Fatal("original ID collision hidden by unrelated or wrong owner scope")
		}
	}
	if sqlResponsibilitySourceCollision(sqlevaluation.SQLResponsibilityObservation{EventID: "different-id", EventType: "evaluation.failed", OwnerKind: "Evaluation", OwnerID: "42"}, source) || sqlResponsibilitySourceCollision(sqlevaluation.SQLResponsibilityObservation{EventID: "old-id"}, source) {
		t.Fatal("retained metadata or another original ID misclassified")
	}
}
