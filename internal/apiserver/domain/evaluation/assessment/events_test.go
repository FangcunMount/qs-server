package assessment

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestNewAssessmentSubmittedEventIncludesModelIdentityFields(t *testing.T) {
	t.Parallel()

	modelRef := NewEvaluationModelRefWithIdentity(
		EvaluationModelKindPersonality,
		modelcatalog.SubKindTypology,
		modelcatalog.AlgorithmMBTI,
		meta.ID(0),
		meta.NewCode("MBTI-16P"),
		"2.0.1",
		"MBTI",
	)
	evt := NewAssessmentSubmittedEvent(
		1,
		NewID(42),
		testee.NewID(1001),
		NewQuestionnaireRefByCode(meta.NewCode("QNR-1"), "1.0.0"),
		NewAnswerSheetRef(meta.FromUint64(2001)),
		&modelRef,
		nil,
		time.Now(),
	)
	data := evt.Payload()
	if data.ModelSubKind != string(modelcatalog.SubKindTypology) {
		t.Fatalf("ModelSubKind = %q", data.ModelSubKind)
	}
	if data.ModelAlgorithm != string(modelcatalog.AlgorithmMBTI) {
		t.Fatalf("ModelAlgorithm = %q", data.ModelAlgorithm)
	}
}
