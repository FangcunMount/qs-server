package handlers

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/component-base/pkg/eventcodec"
	interpretationpb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventpayload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
)

type aiExplanationAutomationClientStub struct {
	response           *interpretationpb.ExecuteAIExplanationResponse
	err                error
	executionRequest   *interpretationpb.ExecuteAIExplanationRequest
	evaluationResponse *interpretationpb.ExecutePromptEvaluationStepResponse
	evaluationErr      error
	evaluationRequest  *interpretationpb.ExecutePromptEvaluationStepRequest
}

func (s *aiExplanationAutomationClientStub) ExecutePromptEvaluationStep(_ context.Context, request *interpretationpb.ExecutePromptEvaluationStepRequest) (*interpretationpb.ExecutePromptEvaluationStepResponse, error) {
	s.evaluationRequest = request
	return s.evaluationResponse, s.evaluationErr
}

func (s *aiExplanationAutomationClientStub) ExecuteAIExplanation(_ context.Context, request *interpretationpb.ExecuteAIExplanationRequest) (*interpretationpb.ExecuteAIExplanationResponse, error) {
	s.executionRequest = request
	return s.response, s.err
}

func TestAIExplanationRequestedHandlerCallsOneShotAutomation(t *testing.T) {
	client := &aiExplanationAutomationClientStub{response: &interpretationpb.ExecuteAIExplanationResponse{
		Success: true, Status: "generated", GenerationId: "701", RunId: "801", ArtifactId: "901",
	}}
	handler := handleAIExplanationRequested(&Dependencies{Logger: discardLogger(), AIExplanationAutomationClient: client})
	if err := handler(context.Background(), eventcatalog.AIExplanationRequested, aiExplanationRequestedPayload(t)); err != nil {
		t.Fatal(err)
	}
	if client.executionRequest == nil || client.executionRequest.GetGenerationId() != "701" ||
		client.executionRequest.GetTraceId() != "evt-ai-requested-1" || client.executionRequest.GetEventId() != "evt-ai-requested-1" {
		t.Fatalf("automation request = %#v", client.executionRequest)
	}
}

func TestAIExplanationRequestedHandlerAcksPersistedFailure(t *testing.T) {
	client := &aiExplanationAutomationClientStub{response: &interpretationpb.ExecuteAIExplanationResponse{
		Status: "failed", GenerationId: "701", RunId: "801", FailureKind: "provider_timeout",
		FailureCode: "provider_timeout", SafeMessage: "AI 解读暂时不可用", Retryable: true,
	}}
	handler := handleAIExplanationRequested(&Dependencies{Logger: discardLogger(), AIExplanationAutomationClient: client})
	if err := handler(context.Background(), eventcatalog.AIExplanationRequested, aiExplanationRequestedPayload(t)); err != nil {
		t.Fatalf("persisted failure must ACK requested event: %v", err)
	}
}

func TestAIExplanationRetryRequestedHandlerForwardsDurableAuthorizationProof(t *testing.T) {
	client := &aiExplanationAutomationClientStub{response: &interpretationpb.ExecuteAIExplanationResponse{
		Success: true, Status: "generated", GenerationId: "701", RunId: "802", ArtifactId: "902",
	}}
	handler := handleAIExplanationRequested(&Dependencies{Logger: discardLogger(), AIExplanationAutomationClient: client})
	if err := handler(context.Background(), eventcatalog.AIExplanationRetryRequested, aiExplanationRetryRequestedPayload(t)); err != nil {
		t.Fatal(err)
	}
	request := client.executionRequest
	if request == nil || request.GetGenerationId() != "701" || request.GetTraceId() != "evt-ai-retry-1" ||
		request.GetEventId() != "evt-ai-retry-1" || request.GetExpectedAttempt() != 1 ||
		request.GetAttemptOrigin() != "manual" || request.GetActionRequestId() != "retry-request-1" {
		t.Fatalf("retry automation request = %#v", request)
	}
}

func TestAIExplanationLeaseRecoveryHandlerForwardsExactLeaseProof(t *testing.T) {
	client := &aiExplanationAutomationClientStub{response: &interpretationpb.ExecuteAIExplanationResponse{
		Success: true, Status: "processing", GenerationId: "701", RunId: "801",
	}}
	handler := handleAIExplanationRequested(&Dependencies{Logger: discardLogger(), AIExplanationAutomationClient: client})
	if err := handler(context.Background(), eventcatalog.AIExplanationLeaseRecoveryRequested, aiExplanationLeaseRecoveryRequestedPayload(t)); err != nil {
		t.Fatal(err)
	}
	request := client.executionRequest
	if request == nil || request.GetGenerationId() != "701" || request.GetExpectedRunId() != "801" ||
		request.GetExpectedLeaseExpiresAt() != "2026-08-27T01:04:00Z" || request.GetExpectedInvocationPhase() != "prepared" ||
		request.GetEventId() != "evt-ai-lease-recovery-1" {
		t.Fatalf("lease recovery automation request = %#v", request)
	}
}

func TestAIExplanationRequestedHandlerNacksTransportAndInvalidResponse(t *testing.T) {
	for _, test := range []struct {
		name   string
		client *aiExplanationAutomationClientStub
	}{
		{name: "transport", client: &aiExplanationAutomationClientStub{err: errors.New("unavailable")}},
		{name: "mismatched generation", client: &aiExplanationAutomationClientStub{response: &interpretationpb.ExecuteAIExplanationResponse{Success: true, Status: "generated", GenerationId: "702"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := handleAIExplanationRequested(&Dependencies{Logger: discardLogger(), AIExplanationAutomationClient: test.client})
			if err := handler(context.Background(), eventcatalog.AIExplanationRequested, aiExplanationRequestedPayload(t)); err == nil {
				t.Fatal("expected requested event NACK error")
			}
		})
	}
}

func TestAIExplanationPromptEvaluationStepHandlerCallsAddressedAutomation(t *testing.T) {
	client := &aiExplanationAutomationClientStub{evaluationResponse: &interpretationpb.ExecutePromptEvaluationStepResponse{
		Success: true, RunId: "1701", CaseId: "PROMPT-EVAL-001", Attempt: 1,
		Status: "progressed", RunStatus: "collecting", NextCaseId: "PROMPT-EVAL-001", NextAttempt: 2,
	}}
	handler := handleAIExplanationPromptEvaluationStep(&Dependencies{Logger: discardLogger(), AIExplanationAutomationClient: client})
	if err := handler(context.Background(), eventcatalog.AIExplanationPromptEvaluationStepRequested, aiExplanationPromptEvaluationStepPayload(t)); err != nil {
		t.Fatal(err)
	}
	request := client.evaluationRequest
	if request == nil || request.GetOrgId() != 1 || request.GetRunId() != "1701" || request.GetCaseId() != "PROMPT-EVAL-001" ||
		request.GetAttempt() != 1 || request.GetRequestedBy() != "user:42" || request.GetEventId() != "evt-ai-prompt-eval-1" {
		t.Fatalf("evaluation automation request = %#v", request)
	}
}

func TestAIExplanationPromptEvaluationStepHandlerForwardsExactV2Address(t *testing.T) {
	client := &aiExplanationAutomationClientStub{evaluationResponse: &interpretationpb.ExecutePromptEvaluationStepResponse{
		Success: true, RunId: "1702", CaseId: "PROMPT-EVAL-001", Status: "progressed", RunStatus: "collecting",
		EvidenceVersion: eventpayload.AIExplanationPromptEvaluationEvidenceVersionV2, ExecutionKind: "generation",
		SlotOrdinal: 1, ExecutionOrdinal: 1,
	}}
	handler := handleAIExplanationPromptEvaluationStep(&Dependencies{Logger: discardLogger(), AIExplanationAutomationClient: client})
	if err := handler(context.Background(), eventcatalog.AIExplanationPromptEvaluationStepRequested, aiExplanationPromptEvaluationStepV2Payload(t)); err != nil {
		t.Fatal(err)
	}
	request := client.evaluationRequest
	if request == nil || request.GetRunId() != "1702" || request.GetAttempt() != 0 || request.GetRecheckId() != "" ||
		request.GetEvidenceVersion() != "v2" || request.GetExecutionKind() != "generation" || request.GetSlotOrdinal() != 1 ||
		request.GetCandidateId() != "" || request.GetExecutionOrdinal() != 1 || request.GetEventId() != "evt-ai-prompt-eval-v2-1" {
		t.Fatalf("v2 evaluation automation request = %#v", request)
	}

	client.evaluationResponse.ExecutionOrdinal = 2
	if err := handler(context.Background(), eventcatalog.AIExplanationPromptEvaluationStepRequested, aiExplanationPromptEvaluationStepV2Payload(t)); err == nil {
		t.Fatal("mismatched v2 execution response must NACK the event")
	}
}

func TestAIExplanationPromptEvaluationRecheckHandlerForwardsAndValidatesRecheckIdentity(t *testing.T) {
	client := &aiExplanationAutomationClientStub{evaluationResponse: &interpretationpb.ExecutePromptEvaluationStepResponse{
		Success: true, RunId: "1701", CaseId: "PROMPT-EVAL-001", Attempt: 1,
		Status: "recheck_completed", RunStatus: "completed", RecheckId: "1801",
	}}
	handler := handleAIExplanationPromptEvaluationStep(&Dependencies{Logger: discardLogger(), AIExplanationAutomationClient: client})
	if err := handler(context.Background(), eventcatalog.AIExplanationPromptEvaluationStepRequested, aiExplanationPromptEvaluationRecheckPayload(t)); err != nil {
		t.Fatal(err)
	}
	request := client.evaluationRequest
	if request == nil || request.GetRunId() != "1701" || request.GetCaseId() != "PROMPT-EVAL-001" ||
		request.GetAttempt() != 1 || request.GetRecheckId() != "1801" || request.GetEventId() != "evt-ai-prompt-recheck-1" {
		t.Fatalf("recheck automation request = %#v", request)
	}

	client.evaluationResponse.RecheckId = "1802"
	if err := handler(context.Background(), eventcatalog.AIExplanationPromptEvaluationStepRequested, aiExplanationPromptEvaluationRecheckPayload(t)); err == nil {
		t.Fatal("mismatched recheck response must NACK the event")
	}
}

func TestAIExplanationPromptEvaluationStepHandlerNacksTransportAndInvalidResponse(t *testing.T) {
	for _, test := range []struct {
		name   string
		client *aiExplanationAutomationClientStub
	}{
		{name: "transport", client: &aiExplanationAutomationClientStub{evaluationErr: errors.New("unavailable")}},
		{name: "mismatched target", client: &aiExplanationAutomationClientStub{evaluationResponse: &interpretationpb.ExecutePromptEvaluationStepResponse{Success: true, RunId: "1701", CaseId: "PROMPT-EVAL-002", Attempt: 1, Status: "progressed"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler := handleAIExplanationPromptEvaluationStep(&Dependencies{Logger: discardLogger(), AIExplanationAutomationClient: test.client})
			if err := handler(context.Background(), eventcatalog.AIExplanationPromptEvaluationStepRequested, aiExplanationPromptEvaluationStepPayload(t)); err == nil {
				t.Fatal("expected prompt evaluation event NACK error")
			}
		})
	}
}

func TestAIExplanationTerminalHandlerValidatesBothTerminalFacts(t *testing.T) {
	handler := handleAIExplanationTerminal(&Dependencies{Logger: discardLogger()})
	generated := event.New(eventcatalog.AIExplanationGenerated, "AIExplanationGeneration", "701", eventpayload.AIExplanationGeneratedData{
		OrgID: 1, GenerationID: "701", RunID: "801", ArtifactID: "901", AssessmentID: "501", TesteeID: 601,
		SourceReportID: "201", Audience: "participant", GeneratedAt: time.Date(2026, 8, 27, 1, 0, 0, 0, time.UTC),
	})
	if err := handler(context.Background(), eventcatalog.AIExplanationGenerated, encodeAIExplanationEvent(t, generated)); err != nil {
		t.Fatal(err)
	}
	failed := event.New(eventcatalog.AIExplanationFailed, "AIExplanationGeneration", "701", eventpayload.AIExplanationFailedData{
		OrgID: 1, GenerationID: "701", RunID: "801", AssessmentID: "501", TesteeID: 601, SourceReportID: "201",
		Audience: "participant", Attempt: 1, FailureKind: "provider_timeout", FailureCode: "provider_timeout",
		Retryable: true, SafeReason: "AI 解读暂时不可用", FailedAt: time.Date(2026, 8, 27, 1, 0, 0, 0, time.UTC),
	})
	if err := handler(context.Background(), eventcatalog.AIExplanationFailed, encodeAIExplanationEvent(t, failed)); err != nil {
		t.Fatal(err)
	}
	if err := handler(context.Background(), "interpretation.ai_explanation.unknown", encodeAIExplanationEvent(t, failed)); err == nil {
		t.Fatal("unknown terminal event must fail closed")
	}
}

func aiExplanationRequestedPayload(t *testing.T) []byte {
	t.Helper()
	requested := event.Event[eventpayload.AIExplanationRequestedData]{
		BaseEvent: event.BaseEvent{
			ID: "evt-ai-requested-1", EventTypeValue: eventcatalog.AIExplanationRequested,
			OccurredAtValue: time.Date(2026, 8, 27, 1, 0, 0, 0, time.UTC), AggregateTypeValue: "AIExplanationGeneration", AggregateIDValue: "701",
		},
		Data: eventpayload.AIExplanationRequestedData{
			OrgID: 1, GenerationID: "701", AssessmentID: "501", TesteeID: 601, SourceReportID: "201",
			Audience: "participant", RequestedAt: time.Date(2026, 8, 27, 1, 0, 0, 0, time.UTC),
		},
	}
	return encodeAIExplanationEvent(t, requested)
}

func aiExplanationRetryRequestedPayload(t *testing.T) []byte {
	t.Helper()
	requested := event.Event[eventpayload.AIExplanationRetryRequestedData]{
		BaseEvent: event.BaseEvent{
			ID: "evt-ai-retry-1", EventTypeValue: eventcatalog.AIExplanationRetryRequested,
			OccurredAtValue: time.Date(2026, 8, 27, 1, 5, 0, 0, time.UTC), AggregateTypeValue: "AIExplanationGeneration", AggregateIDValue: "701",
		},
		Data: eventpayload.AIExplanationRetryRequestedData{
			OrgID: 1, GenerationID: "701", FailedRunID: "801", AssessmentID: "501", TesteeID: 601,
			SourceReportID: "201", Audience: "participant", ExpectedAttempt: 1, NextAttempt: 2,
			AttemptOrigin: "manual", ActionRequestID: "retry-request-1", RequestedAt: time.Date(2026, 8, 27, 1, 5, 0, 0, time.UTC),
		},
	}
	return encodeAIExplanationEvent(t, requested)
}

func aiExplanationLeaseRecoveryRequestedPayload(t *testing.T) []byte {
	t.Helper()
	requested := event.Event[eventpayload.AIExplanationLeaseRecoveryRequestedData]{
		BaseEvent: event.BaseEvent{
			ID: "evt-ai-lease-recovery-1", EventTypeValue: eventcatalog.AIExplanationLeaseRecoveryRequested,
			OccurredAtValue: time.Date(2026, 8, 27, 1, 5, 0, 0, time.UTC), AggregateTypeValue: "AIExplanationGeneration", AggregateIDValue: "701",
		},
		Data: eventpayload.AIExplanationLeaseRecoveryRequestedData{
			OrgID: 1, GenerationID: "701", RunID: "801", Attempt: 1,
			ExpectedLeaseExpiresAt: time.Date(2026, 8, 27, 1, 4, 0, 0, time.UTC),
			InvocationPhase:        "prepared", RequestedAt: time.Date(2026, 8, 27, 1, 5, 0, 0, time.UTC),
		},
	}
	return encodeAIExplanationEvent(t, requested)
}

func aiExplanationPromptEvaluationStepPayload(t *testing.T) []byte {
	t.Helper()
	requested := event.Event[eventpayload.AIExplanationPromptEvaluationStepRequestedData]{
		BaseEvent: event.BaseEvent{
			ID: "evt-ai-prompt-eval-1", EventTypeValue: eventcatalog.AIExplanationPromptEvaluationStepRequested,
			OccurredAtValue: time.Date(2026, 8, 27, 2, 0, 0, 0, time.UTC), AggregateTypeValue: "AIExplanationPromptEvaluation", AggregateIDValue: "1701",
		},
		Data: eventpayload.AIExplanationPromptEvaluationStepRequestedData{
			OrgID: 1, RunID: "1701", CaseID: "PROMPT-EVAL-001", Attempt: 1,
			RequestedBy: "user:42", RequestedAt: time.Date(2026, 8, 27, 2, 0, 0, 0, time.UTC),
		},
	}
	return encodeAIExplanationEvent(t, requested)
}

func aiExplanationPromptEvaluationStepV2Payload(t *testing.T) []byte {
	t.Helper()
	requested := event.Event[eventpayload.AIExplanationPromptEvaluationStepRequestedData]{
		BaseEvent: event.BaseEvent{
			ID: "evt-ai-prompt-eval-v2-1", EventTypeValue: eventcatalog.AIExplanationPromptEvaluationStepRequested,
			OccurredAtValue: time.Date(2026, 8, 31, 2, 0, 0, 0, time.UTC), AggregateTypeValue: "AIExplanationPromptEvaluation", AggregateIDValue: "1702",
		},
		Data: eventpayload.AIExplanationPromptEvaluationStepRequestedData{
			OrgID: 1, RunID: "1702", CaseID: "PROMPT-EVAL-001",
			EvidenceVersion: "v2", ExecutionKind: "generation", SlotOrdinal: 1, ExecutionOrdinal: 1,
			RequestedBy: "user:42", RequestedAt: time.Date(2026, 8, 31, 2, 0, 0, 0, time.UTC),
		},
	}
	return encodeAIExplanationEvent(t, requested)
}

func aiExplanationPromptEvaluationRecheckPayload(t *testing.T) []byte {
	t.Helper()
	requested := event.Event[eventpayload.AIExplanationPromptEvaluationStepRequestedData]{
		BaseEvent: event.BaseEvent{
			ID: "evt-ai-prompt-recheck-1", EventTypeValue: eventcatalog.AIExplanationPromptEvaluationStepRequested,
			OccurredAtValue: time.Date(2026, 8, 29, 2, 0, 0, 0, time.UTC), AggregateTypeValue: "AIExplanationPromptEvaluationRecheck", AggregateIDValue: "1801",
		},
		Data: eventpayload.AIExplanationPromptEvaluationStepRequestedData{
			OrgID: 1, RunID: "1701", CaseID: "PROMPT-EVAL-001", Attempt: 1, RecheckID: "1801",
			RequestedBy: "user:42", RequestedAt: time.Date(2026, 8, 29, 2, 0, 0, 0, time.UTC),
		},
	}
	return encodeAIExplanationEvent(t, requested)
}

func encodeAIExplanationEvent(t *testing.T, value event.DomainEvent) []byte {
	t.Helper()
	payload, err := eventcodec.EncodeDomainEvent(value)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
