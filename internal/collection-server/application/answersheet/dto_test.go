package answersheet

import (
	"encoding/json"
	"testing"
)

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

func TestSubmitAnswerSheetRequestUnmarshalJSONAcceptsStringTesteeID(t *testing.T) {
	var req SubmitAnswerSheetRequest
	payload := []byte(`{"questionnaire_code":"Q","questionnaire_version":"1","testee_id":"618855887087350318","answers":[{"question_code":"q1","question_type":"single_choice","value":"1"}]}`)

	if err := req.UnmarshalJSON(payload); err != nil {
		t.Fatalf("UnmarshalJSON returned error: %v", err)
	}
	if req.TesteeID != 618855887087350318 {
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

func TestSubmitStartReferenceCompatibility(t *testing.T) {
	for _, value := range []string{"null", `""`, `"99"`, "99"} {
		var req SubmitAnswerSheetRequest
		if err := json.Unmarshal([]byte(`{"testee_id":"7","answering_start_id":`+value+`}`), &req); err != nil {
			t.Fatalf("%s: %v", value, err)
		}
		if (value == `"99"` || value == "99") && req.AnsweringStartID != 99 {
			t.Fatal("start reference dropped")
		}
	}
	for _, value := range []string{`"0"`, "0", "-1", `"missing"`, "{}", "[]", "true", `"9223372036854775808"`} {
		var req SubmitAnswerSheetRequest
		if err := json.Unmarshal([]byte(`{"testee_id":"7","answering_start_id":`+value+`}`), &req); err == nil {
			t.Fatalf("invalid reference %s silently accepted", value)
		}
	}
	var req SubmitAnswerSheetRequest
	if err := json.Unmarshal([]byte(`{"testee_id":"7"}`), &req); err != nil || req.AnsweringStartID != 0 {
		t.Fatal("legacy missing start rejected")
	}
}
