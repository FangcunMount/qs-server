package eventpayload

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEvaluationRequestedWireContract(t *testing.T) {
	t.Parallel()

	payload, err := json.Marshal(EvaluationRequestedData{
		OrgID: 7, AssessmentID: 42, TesteeID: 9,
		QuestionnaireCode: "q-1", QuestionnaireVer: "v2", AnswerSheetID: "answer-1",
		ModelKind: "scale", ModelCode: "model-1", ModelVersion: "v3",
		RequestedAt: time.Date(2026, time.July, 13, 10, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	want := `{"org_id":7,"assessment_id":42,"testee_id":9,"questionnaire_code":"q-1","questionnaire_version":"v2","answersheet_id":"answer-1","model_kind":"scale","model_code":"model-1","model_version":"v3","requested_at":"2026-07-13T10:30:00Z"}`
	if got := string(payload); got != want {
		t.Fatalf("wire JSON = %s, want %s", got, want)
	}
}

func TestLifecycleActionWireValues(t *testing.T) {
	t.Parallel()

	if QuestionnaireChangeActionPublished != "published" || QuestionnaireChangeActionUnpublished != "unpublished" || QuestionnaireChangeActionArchived != "archived" {
		t.Fatal("questionnaire lifecycle action wire values changed")
	}
	if AssessmentModelChangeActionPublished != "published" || AssessmentModelChangeActionUnpublished != "unpublished" || AssessmentModelChangeActionArchived != "archived" {
		t.Fatal("assessment model lifecycle action wire values changed")
	}
}
