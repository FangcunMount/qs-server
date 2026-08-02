package handlers

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	evalpb "github.com/FangcunMount/qs-server/api/grpc/gen/evaluation"
	interpretationpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
)

func TestEvaluationAndInterpretationHandlersDoNotUseRetiredPayloadTypes(t *testing.T) {
	t.Parallel()

	forbidden := []string{
		"AssessmentInterpretedPayload",
		"AssessmentSubmittedData",
		"AssessmentEvaluatedData",
		"AssessmentFailedData",
	}
	for _, path := range []string{"assessment_handler.go", "report_handler.go"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(data)
		for _, token := range forbidden {
			if strings.Contains(text, token) {
				t.Fatalf("%s contains %q; interpreted/report handlers must use eventoutcome payloads only", path, token)
			}
		}
	}
}

func TestWorkerProcessesStoredEventsWithRetiredHistoricalContextAsOrdinaryEvents(t *testing.T) {
	t.Run("answersheet submitted", func(t *testing.T) {
		client := &fakeWorkerInternalClient{}
		handler := handleAnswerSheetSubmitted(newAnswerSheetHandlerTestDeps(client, nil))
		payload := withRetiredHistoricalContext(t, mustBuildAnswerSheetSubmittedPayload(t, 701))

		if err := handler(context.Background(), eventcatalog.AnswerSheetSubmitted, payload); err != nil {
			t.Fatalf("handle stored answersheet event: %v", err)
		}
		if client.createCalls != 1 {
			t.Fatalf("EnsureAssessment calls = %d, want 1", client.createCalls)
		}
	})

	t.Run("evaluation requested", func(t *testing.T) {
		client := &assessmentEvaluateClient{resp: &evalpb.ExecuteEvaluationResponse{Status: "evaluated"}}
		handler := handleEvaluationRequested(newAnswerSheetHandlerTestDeps(client, nil))
		payload := withRetiredHistoricalContext(t, mustBuildEvaluationRequestedPayload(t, 702))

		if err := handler(context.Background(), eventcatalog.EvaluationRequested, payload); err != nil {
			t.Fatalf("handle stored evaluation event: %v", err)
		}
		if client.evaluateCalls != 1 {
			t.Fatalf("ExecuteEvaluation calls = %d, want 1", client.evaluateCalls)
		}
	})

	t.Run("evaluation outcome committed", func(t *testing.T) {
		client := &assessmentGenerateReportClient{resp: &interpretationpb.GenerateReportFromAssessmentResponse{
			Success: true,
			Status:  "generated",
		}}
		handler := handleEvaluationOutcomeCommitted(newAnswerSheetHandlerTestDeps(client, nil))
		payload := withRetiredHistoricalContext(t, mustBuildEvaluationOutcomeCommittedPayload(t, 703))

		if err := handler(context.Background(), eventcatalog.EvaluationOutcomeCommitted, payload); err != nil {
			t.Fatalf("handle stored outcome event: %v", err)
		}
		if client.generateReportCalls != 1 {
			t.Fatalf("GenerateReport calls = %d, want 1", client.generateReportCalls)
		}
	})
}

func withRetiredHistoricalContext(t *testing.T, payload []byte) []byte {
	t.Helper()

	var envelope map[string]any
	if err := json.Unmarshal(payload, &envelope); err != nil {
		t.Fatalf("decode event fixture: %v", err)
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatal("event fixture data is not an object")
	}
	data["historical_context"] = map[string]any{
		"batch_id":    "retired-batch",
		"scenario_id": "retired-scenario",
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("encode stored event fixture: %v", err)
	}
	return encoded
}
