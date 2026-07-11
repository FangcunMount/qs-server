package event

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestNewRequestedEventIncludesModelIdentityFields(t *testing.T) {
	t.Parallel()

	evt := NewRequestedEvent(RequestedInput{
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
		RequestedAt:       time.Now(),
	})
	data := evt.Payload()
	if data.ModelSubKind != string(modelcatalog.SubKindTypology) {
		t.Fatalf("ModelSubKind = %q", data.ModelSubKind)
	}
	if data.ModelAlgorithm != string(modelcatalog.AlgorithmMBTI) {
		t.Fatalf("ModelAlgorithm = %q", data.ModelAlgorithm)
	}
}

func TestNewOutcomeCommittedEventIncludesDurableOutcomeAndRunReferences(t *testing.T) {
	t.Parallel()

	evt := NewOutcomeCommittedEvent(1, 42, 1001, "9001", "42:1", time.Unix(100, 0))
	data := evt.Payload()
	if data.OutcomeID != "9001" || data.EvaluationRunID != "42:1" {
		t.Fatalf("evaluated references = outcome:%q run:%q", data.OutcomeID, data.EvaluationRunID)
	}
}
