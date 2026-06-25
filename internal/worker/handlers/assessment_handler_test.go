package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/internal/apiserver/interface/grpc/proto/internalapi"
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

func TestHandleAssessmentSubmittedAcksWhenEvaluationAlreadyProcessed(t *testing.T) {
	client := &assessmentEvaluateClient{
		resp: &pb.EvaluateAssessmentResponse{
			Success: false,
			Status:  "already_interpreted",
			Message: "assessment already processed",
		},
	}
	deps := newAnswerSheetHandlerTestDeps(client, nil)
	handler := handleAssessmentSubmitted(deps)

	err := handler(context.Background(), "assessment.submitted", mustBuildAssessmentSubmittedPayload(t, 42))
	if err != nil {
		t.Fatalf("expected duplicate evaluation to ack without error, got %v", err)
	}
	if client.evaluateCalls != 1 {
		t.Fatalf("evaluate calls = %d, want 1", client.evaluateCalls)
	}
}

type assessmentEvaluateClient struct {
	fakeWorkerInternalClient
	resp          *pb.EvaluateAssessmentResponse
	err           error
	evaluateCalls int
}

func (c *assessmentEvaluateClient) EvaluateAssessment(
	_ context.Context,
	_ uint64,
) (*pb.EvaluateAssessmentResponse, error) {
	c.evaluateCalls++
	return c.resp, c.err
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
