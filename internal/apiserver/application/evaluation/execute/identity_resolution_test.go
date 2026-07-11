package execute

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestResolveExecutionIdentityPrefersInputAlgorithmWhenAssessmentMissingIt(t *testing.T) {
	modelRef := assessment.NewEvaluationModelRefByCode(
		assessment.EvaluationModelKindPersonality,
		meta.NewCode("BIG5_IPIP_50"),
		"1.0.0",
		"大五人格",
	)
	a, err := assessment.NewAssessment(
		1,
		meta.FromUint64(2001),
		assessment.NewQuestionnaireRefByCode(meta.NewCode("BIG5_IPIP_50"), "1.0.0"),
		assessment.NewAnswerSheetRef(meta.FromUint64(5001)),
		assessment.NewAdhocOrigin(),
		assessment.WithEvaluationModel(modelRef),
	)
	if err != nil {
		t.Fatalf("NewAssessment: %v", err)
	}
	input := &evaluationinput.InputSnapshot{
		Model: evaluationinput.NewTypologyModelSnapshot(&modeltypology.Payload{
			Code: "BIG5_IPIP_50", Version: "1.0.0", Algorithm: modelcatalog.AlgorithmBigFive, Status: "published",
		}),
	}
	if got := resolveExecutionIdentity(a, input); got != evaluation.PersonalityTypologyIdentity(modelcatalog.AlgorithmBigFive) {
		t.Fatalf("resolveExecutionIdentity() = %s, want %s", got, evaluation.PersonalityTypologyIdentity(modelcatalog.AlgorithmBigFive))
	}
}
