package answersheet

import "testing"

func TestSubmitAnswerSheetRequestUnmarshalJSONAcceptsNumericTesteeID(t *testing.T) {
	var req SubmitAnswerSheetRequest
	payload := []byte(`{"questionnaire_code":"Q","questionnaire_version":"1","testee_id":615969735435104814,"answers":[{"question_code":"q1","question_type":"single_choice","value":"1"}]}`)

	if err := req.UnmarshalJSON(payload); err != nil {
		t.Fatalf("UnmarshalJSON returned error: %v", err)
	}
	if req.TesteeID != 615969735435104814 {
		t.Fatalf("unexpected testee id: %d", req.TesteeID)
	}
}

func TestSubmitAnswerSheetRequestUnmarshalJSONRejectsFractionalTesteeID(t *testing.T) {
	var req SubmitAnswerSheetRequest
	payload := []byte(`{"questionnaire_code":"Q","questionnaire_version":"1","testee_id":1.5,"answers":[{"question_code":"q1","question_type":"single_choice","value":"1"}]}`)

	if err := req.UnmarshalJSON(payload); err == nil {
		t.Fatal("expected fractional testee_id to be rejected")
	}
}
