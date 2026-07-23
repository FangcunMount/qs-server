package assessment

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	evaldomainevent "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/event"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestNewEvaluationRequestedEventIncludesModelIdentityFields(t *testing.T) {
	t.Parallel()

	modelRef := NewEvaluationModelRefWithIdentity(
		EvaluationModelKindTypology,
		modelcatalog.SubKindTypology,
		modelcatalog.AlgorithmPersonalityTypology,
		meta.ID(0),
		meta.NewCode("MBTI-16P"),
		"2.0.1",
		"MBTI",
	)
	evt := NewEvaluationRequestedEvent(
		1,
		NewID(42),
		testee.NewID(1001),
		NewQuestionnaireRefByCode(meta.NewCode("QNR-1"), "1.0.0"),
		NewAnswerSheetRef(meta.FromUint64(2001)),
		&modelRef,
		time.Now(),
	)
	data := evt.Payload()
	if data.ModelAlgorithm != string(modelcatalog.AlgorithmPersonalityTypology) {
		t.Fatalf("ModelAlgorithm = %q", data.ModelAlgorithm)
	}
	if evt.EventType() != evaldomainevent.TypeRequested {
		t.Fatalf("event type = %q", evt.EventType())
	}
}
