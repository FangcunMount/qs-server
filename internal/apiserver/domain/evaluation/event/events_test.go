package event

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestNewSubmittedEventIncludesModelIdentityFields(t *testing.T) {
	t.Parallel()

	evt := NewSubmittedEvent(SubmittedInput{
		OrgID:             1,
		AssessmentID:      42,
		TesteeID:          1001,
		QuestionnaireCode: "QNR-1",
		QuestionnaireVer:  "1.0.0",
		AnswerSheetID:     "2001",
		ModelKind:         "personality",
		ModelSubKind:      string(modelcatalog.SubKindTypology),
		ModelAlgorithm:    string(modelcatalog.AlgorithmMBTI),
		ModelCode:         "MBTI-16P",
		ModelVersion:      "2.0.1",
		SubmittedAt:       time.Now(),
	})
	data := evt.Payload()
	if data.ModelSubKind != string(modelcatalog.SubKindTypology) {
		t.Fatalf("ModelSubKind = %q", data.ModelSubKind)
	}
	if data.ModelAlgorithm != string(modelcatalog.AlgorithmMBTI) {
		t.Fatalf("ModelAlgorithm = %q", data.ModelAlgorithm)
	}
}
