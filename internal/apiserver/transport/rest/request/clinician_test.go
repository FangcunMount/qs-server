package request

import (
	"encoding/json"
	"testing"
)

func TestClinicianRequestsRejectRetiredBinding(t *testing.T) {
	for _, data := range []string{`{"operator_id":null}`, `{"operator_id":"123"}`, `{"staff_id":123}`, `{"operator_id":{}}`} {
		for _, request := range []interface{}{&CreateClinicianRequest{}, &UpdateClinicianRequest{}} {
			if err := json.Unmarshal([]byte(data), request); err == nil {
				t.Fatalf("accepted binding: %s", data)
			}
		}
	}
	for _, request := range []interface{}{&CreateClinicianRequest{}, &UpdateClinicianRequest{}} {
		if err := json.Unmarshal([]byte(`{"name":"医生","clinician_type":"doctor"}`), request); err != nil {
			t.Fatal(err)
		}
	}
}
