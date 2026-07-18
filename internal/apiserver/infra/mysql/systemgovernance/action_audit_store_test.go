package systemgovernance

import (
	"encoding/json"
	"testing"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
)

func TestDecodeActionAuditReplaySupportsV2ErrorAndLegacyResult(t *testing.T) {
	v2, err := json.Marshal(actionAuditEnvelope{
		SchemaVersion: 2,
		Error:         &app.ActionAuditError{Code: 409, Message: "conflict"},
	})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := decodeActionAuditReplay(string(v2))
	if err != nil || replay.Error == nil || replay.Error.Code != 409 || replay.Result != nil {
		t.Fatalf("v2 replay = %+v, %v", replay, err)
	}

	legacy, err := json.Marshal(app.ActionRunResult{ActionID: "resilience.tune_rate_limit", Status: "failed"})
	if err != nil {
		t.Fatal(err)
	}
	replay, err = decodeActionAuditReplay(string(legacy))
	if err != nil || replay.Result == nil || replay.Result.ActionID != "resilience.tune_rate_limit" || replay.Error != nil {
		t.Fatalf("legacy replay = %+v, %v", replay, err)
	}
}
