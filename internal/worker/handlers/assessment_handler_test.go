package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestHandleAssessmentSubmittedRejectsNegativeAssessmentID(t *testing.T) {
	client := &fakeWorkerInternalClient{}
	deps := newAnswerSheetHandlerTestDeps(client, nil)
	handler := handleAssessmentSubmitted(deps)

	err := handler(context.Background(), "assessment.submitted", mustBuildAssessmentSubmittedPayload(t, -1))
	if err == nil {
		t.Fatal("expected negative assessment id to be rejected")
	}
}

func TestHandleAssessmentFailedRejectsNegativeAssessmentID(t *testing.T) {
	client := &fakeWorkerInternalClient{}
	deps := newAnswerSheetHandlerTestDeps(client, nil)
	handler := handleAssessmentFailed(deps)

	err := handler(context.Background(), "assessment.failed", mustBuildAssessmentFailedPayload(t, -2))
	if err == nil {
		t.Fatal("expected negative assessment id to be rejected")
	}
}

func mustBuildAssessmentSubmittedPayload(t *testing.T, assessmentID int64) []byte {
	t.Helper()

	now := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	payload, err := json.Marshal(map[string]any{
		"id":            "evt-assessment-submitted",
		"eventType":     "assessment.submitted",
		"occurredAt":    now,
		"aggregateType": "Assessment",
		"aggregateID":   "agg-1",
		"data": map[string]any{
			"org_id":                18,
			"assessment_id":         assessmentID,
			"testee_id":             99,
			"questionnaire_code":    "QNR-001",
			"questionnaire_version": "1.0.0",
			"answersheet_id":        "456",
			"scale_code":            "scale-1",
			"submitted_at":          now,
		},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return payload
}

func mustBuildAssessmentFailedPayload(t *testing.T, assessmentID int64) []byte {
	t.Helper()

	now := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	payload, err := json.Marshal(map[string]any{
		"id":            "evt-assessment-failed",
		"eventType":     "assessment.failed",
		"occurredAt":    now,
		"aggregateType": "Assessment",
		"aggregateID":   "agg-2",
		"data": map[string]any{
			"org_id":        18,
			"assessment_id": assessmentID,
			"testee_id":     99,
			"reason":        "boom",
			"failed_at":     now,
		},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return payload
}
