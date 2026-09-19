package assessment

import (
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	evaldomainevent "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/event"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/retrygovernance"
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

func TestRetryEventIDFitsPersistentIdentityAndRemainsIdempotent(t *testing.T) {
	a := &Assessment{id: NewID(638017038744302126)}
	at := time.Now()
	requestID := "9fa91baf-8a9b-4ebf-9957-94f516b9d96e"
	for _, origin := range []retrygovernance.AttemptOrigin{retrygovernance.AttemptOriginManual, retrygovernance.AttemptOriginForce} {
		for _, request := range []string{requestID, strings.Repeat("a", 64)} {
			first := NewEvaluationRetryRequestedEvent(a, 3, origin, request, at)
			if len(first.EventID()) > 64 {
				t.Fatalf("retry event ID exceeds checkpoint/outbox limit: %d", len(first.EventID()))
			}
			repeated := NewEvaluationRetryRequestedEvent(a, 3, origin, request, at.Add(time.Minute))
			if first.EventID() != repeated.EventID() {
				t.Fatal("same retry changed identity")
			}
			changed := NewEvaluationRetryRequestedEvent(a, 3, origin, request[:len(request)-1]+"b", at)
			if first.EventID() == changed.EventID() {
				t.Fatal("different request IDs collapsed")
			}
			next := NewEvaluationRetryRequestedEvent(a, 4, origin, request, at)
			if first.EventID() == next.EventID() {
				t.Fatal("different attempts collapsed")
			}
			if first.Data.ActionRequestID != request {
				t.Fatal("audit request ID changed")
			}
		}
	}
	automatic := NewEvaluationRetryRequestedEvent(a, 3, retrygovernance.AttemptOriginAutomatic, "", at)
	if automatic.EventID() != "eval-retry:638017038744302126:3:automatic" {
		t.Fatal("existing automatic identity changed")
	}
}
