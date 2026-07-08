package typologysession

import "testing"

func TestStartSessionRequestUnmarshalJSONAcceptsStringTesteeID(t *testing.T) {
	var req StartSessionRequest
	payload := []byte(`{"model_code":"MBTI_OEJTS","testee_id":"618855887087350318"}`)

	if err := req.UnmarshalJSON(payload); err != nil {
		t.Fatalf("UnmarshalJSON returned error: %v", err)
	}
	if req.ModelCode != "MBTI_OEJTS" {
		t.Fatalf("unexpected model_code: %q", req.ModelCode)
	}
	if req.TesteeID != 618855887087350318 {
		t.Fatalf("unexpected testee_id: %d", req.TesteeID)
	}
}

func TestStartSessionRequestUnmarshalJSONAcceptsNumericTesteeID(t *testing.T) {
	var req StartSessionRequest
	payload := []byte(`{"model_code":"MBTI_OEJTS","testee_id":618855887087350318}`)

	if err := req.UnmarshalJSON(payload); err != nil {
		t.Fatalf("UnmarshalJSON returned error: %v", err)
	}
	if req.TesteeID != 618855887087350318 {
		t.Fatalf("unexpected testee_id: %d", req.TesteeID)
	}
}
