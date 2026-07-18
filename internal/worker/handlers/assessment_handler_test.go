package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	evalpb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	interpretationpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
)

func TestHandleEvaluationRequestedFailsWhenInternalClientMissing(t *testing.T) {
	handler := handleEvaluationRequested(newAnswerSheetHandlerTestDeps(nil, nil))
	if err := handler(context.Background(), eventcatalog.EvaluationRequested, mustBuildEvaluationRequestedPayload(t, 42)); err == nil {
		t.Fatal("expected error when internal client is missing")
	}
}

func TestHandleEvaluationRequestedCallsEvaluate(t *testing.T) {
	client := &assessmentEvaluateClient{resp: &evalpb.ExecuteEvaluationResponse{
		Status:    "evaluated",
		RunId:     "42:1",
		OutcomeId: "9001",
	}}
	handler := handleEvaluationRequested(newAnswerSheetHandlerTestDeps(client, nil))
	if err := handler(context.Background(), eventcatalog.EvaluationRequested, mustBuildEvaluationRequestedPayload(t, 42)); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if client.evaluateCalls != 1 {
		t.Fatalf("evaluate calls = %d, want 1", client.evaluateCalls)
	}
	if client.resp.GetStatus() != "evaluated" || client.resp.GetRunId() != "42:1" || client.resp.GetOutcomeId() != "9001" {
		t.Fatalf("evaluate response = %#v, want status/run/outcome fields", client.resp)
	}
}

func TestAutomaticEvaluationRetryEmergencySwitchDoesNotBlockManualRetry(t *testing.T) {
	client := &assessmentEvaluateClient{resp: &evalpb.ExecuteEvaluationResponse{Status: "evaluated", RunId: "42:4", OutcomeId: "9001"}}
	deps := newAnswerSheetHandlerTestDeps(client, nil)
	deps.DisableAutomaticRetry = true
	handler := handleEvaluationRequested(deps)
	if err := handler(t.Context(), eventcatalog.EvaluationRetryRequested, mustBuildEvaluationRetryPayload(t, "automatic")); !errors.Is(err, ErrAutomaticRetryPaused) {
		t.Fatalf("automatic retry error = %v, want emergency pause", err)
	}
	if client.evaluateCalls != 0 {
		t.Fatalf("automatic retry calls = %d, want 0", client.evaluateCalls)
	}
	if err := handler(t.Context(), eventcatalog.EvaluationRetryRequested, mustBuildEvaluationRetryPayload(t, "manual")); err != nil {
		t.Fatal(err)
	}
	if client.evaluateCalls != 1 {
		t.Fatalf("manual retry calls = %d, want 1", client.evaluateCalls)
	}
}

func TestHandleEvaluationOutcomeCommittedCallsGenerateReport(t *testing.T) {
	client := &assessmentGenerateReportClient{resp: &interpretationpb.GenerateReportFromAssessmentResponse{
		Success:      true,
		Status:       "generated",
		GenerationId: "gen-1",
		RunId:        "run-1",
		ReportId:     "report-1",
	}}
	handler := handleEvaluationOutcomeCommitted(newAnswerSheetHandlerTestDeps(client, nil))
	if err := handler(context.Background(), eventcatalog.EvaluationOutcomeCommitted, mustBuildEvaluationOutcomeCommittedPayload(t, 42)); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if client.generateReportCalls != 1 {
		t.Fatalf("generate report calls = %d, want 1", client.generateReportCalls)
	}
	if !client.resp.GetSuccess() || client.resp.GetStatus() != "generated" || client.resp.GetReportId() != "report-1" {
		t.Fatalf("generate report response = %#v, want success generated report", client.resp)
	}
}

func TestHandleEvaluationFailedRejectsNegativeAssessmentID(t *testing.T) {
	handler := handleEvaluationFailed(newAnswerSheetHandlerTestDeps(&fakeWorkerInternalClient{}, nil))
	if err := handler(context.Background(), eventcatalog.EvaluationFailed, mustBuildEvaluationFailedPayload(t, -2)); err == nil {
		t.Fatal("expected negative assessment id to be rejected")
	}
}

type assessmentEvaluateClient struct {
	fakeWorkerInternalClient
	resp          *evalpb.ExecuteEvaluationResponse
	err           error
	evaluateCalls int
}

func (c *assessmentEvaluateClient) ExecuteEvaluation(context.Context, uint64) (*evalpb.ExecuteEvaluationResponse, error) {
	c.evaluateCalls++
	return c.resp, c.err
}

type assessmentGenerateReportClient struct {
	fakeWorkerInternalClient
	resp                *interpretationpb.GenerateReportFromAssessmentResponse
	err                 error
	generateReportCalls int
}

func (c *assessmentGenerateReportClient) GenerateReportFromOutcome(context.Context, string) (*interpretationpb.GenerateReportFromAssessmentResponse, error) {
	c.generateReportCalls++
	return c.resp, c.err
}

func mustBuildEvaluationRequestedPayload(t *testing.T, assessmentID int64) []byte {
	t.Helper()
	now := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	payload, err := json.Marshal(map[string]any{
		"id": "evt-evaluation-requested", "eventType": eventcatalog.EvaluationRequested, "occurredAt": now, "aggregateType": "Evaluation", "aggregateID": "42",
		"data": map[string]any{
			"org_id": 18, "assessment_id": assessmentID, "testee_id": 99, "questionnaire_code": "QNR-001", "questionnaire_version": "1.0.0", "answersheet_id": "456", "model_code": "model-1", "requested_at": now,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func mustBuildEvaluationRetryPayload(t *testing.T, origin string) []byte {
	t.Helper()
	now := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	payload, err := json.Marshal(map[string]any{
		"id": "eval-retry:42:3:" + origin, "eventType": eventcatalog.EvaluationRetryRequested,
		"occurredAt": now, "aggregateType": "Evaluation", "aggregateID": "42",
		"data": map[string]any{
			"org_id": 18, "assessment_id": 42, "testee_id": 99, "questionnaire_code": "QNR-001",
			"questionnaire_version": "1.0.0", "answersheet_id": "456", "model_code": "model-1",
			"requested_at": now, "expected_attempt": 3, "attempt_origin": origin, "mode": "next_attempt",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func mustBuildEvaluationOutcomeCommittedPayload(t *testing.T, assessmentID int64) []byte {
	t.Helper()
	now := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	payload, err := json.Marshal(map[string]any{
		"id": "evt-evaluation-outcome-committed", "eventType": eventcatalog.EvaluationOutcomeCommitted, "occurredAt": now, "aggregateType": "Evaluation", "aggregateID": "42",
		"data": map[string]any{
			"org_id": 18, "assessment_id": assessmentID, "testee_id": 99, "outcome_id": "9001", "evaluation_run_id": "42:1", "committed_at": now,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func mustBuildEvaluationFailedPayload(t *testing.T, assessmentID int64) []byte {
	t.Helper()
	now := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	payload, err := json.Marshal(map[string]any{
		"id": "evt-evaluation-failed", "eventType": eventcatalog.EvaluationFailed, "occurredAt": now, "aggregateType": "Evaluation", "aggregateID": "42",
		"data": map[string]any{"org_id": 18, "assessment_id": assessmentID, "testee_id": 99, "reason": "boom", "failed_at": now},
	})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}
