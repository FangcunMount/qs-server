package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventoutcome"
)

func TestHandleAssessmentSubmittedFailsWhenInternalClientMissing(t *testing.T) {
	deps := newAnswerSheetHandlerTestDeps(nil, nil)
	handler := handleAssessmentSubmitted(deps)

	err := handler(context.Background(), "assessment.submitted", mustBuildAssessmentSubmittedPayload(t, 42))
	if err == nil {
		t.Fatal("expected error when internal client is missing")
	}
}

func TestHandleAssessmentEvaluatedFailsWhenInternalClientMissing(t *testing.T) {
	deps := newAnswerSheetHandlerTestDeps(nil, nil)
	handler := handleAssessmentEvaluated(deps)

	err := handler(context.Background(), "assessment.evaluated", mustBuildAssessmentEvaluatedPayload(t, 42))
	if err == nil {
		t.Fatal("expected error when internal client is missing")
	}
}

func TestHandleAssessmentEvaluatedCallsGenerateReport(t *testing.T) {
	client := &assessmentGenerateReportClient{
		resp: &pb.GenerateReportFromAssessmentResponse{Success: true, Status: "interpreted"},
	}
	deps := newAnswerSheetHandlerTestDeps(client, nil)
	handler := handleAssessmentEvaluated(deps)

	if err := handler(context.Background(), "assessment.evaluated", mustBuildAssessmentEvaluatedPayload(t, 42)); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if client.generateReportCalls != 1 {
		t.Fatalf("generate report calls = %d, want 1", client.generateReportCalls)
	}
}

func TestHandleAssessmentEvaluatedRejectsNegativeAssessmentID(t *testing.T) {
	client := &fakeWorkerInternalClient{}
	deps := newAnswerSheetHandlerTestDeps(client, nil)
	handler := handleAssessmentEvaluated(deps)

	err := handler(context.Background(), "assessment.evaluated", mustBuildAssessmentEvaluatedPayload(t, -1))
	if err == nil {
		t.Fatal("expected negative assessment id to be rejected")
	}
}

type assessmentGenerateReportClient struct {
	fakeWorkerInternalClient
	resp                *pb.GenerateReportFromAssessmentResponse
	err                 error
	generateReportCalls int
}

func (c *assessmentGenerateReportClient) GenerateReportFromAssessment(
	_ context.Context,
	_ uint64,
) (*pb.GenerateReportFromAssessmentResponse, error) {
	c.generateReportCalls++
	return c.resp, c.err
}

func mustBuildAssessmentEvaluatedPayload(t *testing.T, assessmentID int64) []byte {
	t.Helper()

	now := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	payload, err := json.Marshal(map[string]any{
		"id":            "evt-assessment-evaluated",
		"eventType":     "assessment.evaluated",
		"occurredAt":    now,
		"aggregateType": "Assessment",
		"aggregateID":   "agg-evaluated",
		"data": map[string]any{
			"org_id":        18,
			"assessment_id": assessmentID,
			"testee_id":     99,
			"evaluated_at":  now,
		},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return payload
}

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

func TestAssessmentInterpretedOutcomeHelpers(t *testing.T) {
	level := &eventoutcome.ResultLevel{Code: "severe", Severity: "high"}
	score := &eventoutcome.ScoreValue{Value: 18.5}
	if got := assessmentLevelCode(level); got != "severe" {
		t.Fatalf("level code = %q, want severe", got)
	}
	if got := assessmentLevelSeverity(level); got != "high" {
		t.Fatalf("severity = %q, want high", got)
	}
	if got := assessmentPrimaryScoreValue(score); got != 18.5 {
		t.Fatalf("score = %v, want 18.5", got)
	}
	if got := assessmentPrimaryScoreValue(nil); got != 0 {
		t.Fatalf("nil score = %v, want 0", got)
	}
}

func TestHandleAssessmentInterpretedOutcomeAcksHighSeverityPayload(t *testing.T) {
	deps := newAnswerSheetHandlerTestDeps(&fakeWorkerInternalClient{}, nil)
	handler := handleAssessmentInterpreted(deps)
	if err := handler(context.Background(), eventcatalog.AssessmentInterpreted, mustBuildAssessmentInterpretedOutcomePayload(t)); err != nil {
		t.Fatalf("handler: %v", err)
	}
}

func mustBuildAssessmentInterpretedOutcomePayload(t *testing.T) []byte {
	t.Helper()

	now := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	payload, err := json.Marshal(map[string]any{
		"id":            "evt-assessment-interpreted-v2",
		"eventType":     eventcatalog.AssessmentInterpreted,
		"occurredAt":    now,
		"aggregateType": "Assessment",
		"aggregateID":   "agg-3",
		"data": map[string]any{
			"org_id":         18,
			"assessment_id":  42,
			"testee_id":      99,
			"level":          map[string]any{"code": "severe", "label": "severe", "severity": "high"},
			"primary_score":  map[string]any{"kind": "raw_total", "value": 18.5},
			"interpreted_at": now,
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
