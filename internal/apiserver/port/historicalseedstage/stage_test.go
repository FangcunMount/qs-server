package historicalseedstage

import (
	"encoding/json"
	"testing"
)

func TestRecordPayloadJSONIsExposedAsObject(t *testing.T) {
	payload, err := json.Marshal(Record{PayloadJSON: json.RawMessage(`{"generation_id":"gen-1","run_id":"run-1"}`)})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	var report map[string]string
	if err := json.Unmarshal(decoded["payload_json"], &report); err != nil {
		t.Fatalf("payload_json is not an object: %s (%v)", decoded["payload_json"], err)
	}
	if report["generation_id"] != "gen-1" || report["run_id"] != "run-1" {
		t.Fatalf("payload_json = %#v", report)
	}
}
