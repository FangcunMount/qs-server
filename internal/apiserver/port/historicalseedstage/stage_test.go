package historicalseedstage

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
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

type lifecycleRecorderStub struct {
	handle          AttemptHandle
	completedHandle AttemptHandle
	failedHandle    AttemptHandle
}

func (s *lifecycleRecorderStub) Begin(_ context.Context, attempt Attempt) (AttemptHandle, error) {
	s.handle = AttemptHandle{ID: 17, Stage: attempt.Stage, ContextHash: "fingerprint"}
	return s.handle, nil
}

func (s *lifecycleRecorderStub) CompleteAttempt(_ context.Context, handle AttemptHandle, completion Completion) (*Record, error) {
	s.completedHandle = handle
	return &Record{Stage: completion.Stage, Status: "completed"}, nil
}

func (s *lifecycleRecorderStub) Fail(_ context.Context, handle AttemptHandle, _ Failure) error {
	s.failedHandle = handle
	return nil
}

func (s *lifecycleRecorderStub) Complete(_ context.Context, _ Completion) (*Record, error) {
	return nil, errors.New("legacy Complete must not be used when an exact handle is present")
}

func TestAttemptHelpersCarryAndCloseTheExactHandle(t *testing.T) {
	recorder := &lifecycleRecorderStub{}
	businessAt := time.Date(2025, 1, 1, 9, 0, 0, 0, time.UTC)
	ctx, handle, err := BeginStageAttempt(context.Background(), recorder, Attempt{Stage: StageEntryResolve, BusinessAt: businessAt})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CompleteStage(ctx, recorder, Completion{Stage: StageEntryResolve, BusinessAt: businessAt, ResourceType: "assessment_entry", ResourceID: "1"}); err != nil {
		t.Fatal(err)
	}
	if recorder.completedHandle != handle {
		t.Fatalf("completed handle=%+v want=%+v", recorder.completedHandle, handle)
	}
	failure := errors.New("boom")
	if err := FailStageAttempt(ctx, recorder, handle, Failure{Stage: StageEntryResolve, BusinessAt: businessAt, Err: failure}); err != nil {
		t.Fatal(err)
	}
	if recorder.failedHandle != handle {
		t.Fatalf("failed handle=%+v want=%+v", recorder.failedHandle, handle)
	}
}
