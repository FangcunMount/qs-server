package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestQuestionnaireChangedHandler_PublishedGeneratesQRCode(t *testing.T) {
	client := &fakeWorkerInternalClient{}
	deps := &Dependencies{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		InternalClient: client,
	}

	payload := mustBuildLifecycleChangedPayload(t, "questionnaire.changed", "Questionnaire", "Q-1", map[string]any{
		"code":       "Q-1",
		"version":    "1.0",
		"title":      "PHQ-9",
		"action":     "published",
		"changed_at": time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC),
	})

	if err := handleQuestionnaireChanged(deps)(context.Background(), "questionnaire.changed", payload); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if client.questionnaireQRCodeCalls != 1 {
		t.Fatalf("expected 1 questionnaire QR call, got %d", client.questionnaireQRCodeCalls)
	}
}

func TestAssessmentModelChangedHandler_NonPublishedSkipsQRCode(t *testing.T) {
	client := &fakeWorkerInternalClient{}
	deps := &Dependencies{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		InternalClient: client,
	}

	payload := mustBuildLifecycleChangedPayload(t, "assessment_model.changed", "AssessmentModel", "SDS", map[string]any{
		"kind":       "scale",
		"code":       "SDS",
		"version":    "",
		"title":      "SDS",
		"action":     "updated",
		"changed_at": time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC),
	})

	if err := handleAssessmentModelChanged(deps)(context.Background(), "assessment_model.changed", payload); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if client.scaleQRCodeCalls != 0 {
		t.Fatalf("expected 0 scale QR calls, got %d", client.scaleQRCodeCalls)
	}
}

func TestAssessmentModelChangedHandler_PublishedScaleInvokesPostActions(t *testing.T) {
	client := &fakeWorkerInternalClient{}
	deps := &Dependencies{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		InternalClient: client,
	}

	payload := mustBuildLifecycleChangedPayload(t, "assessment_model.changed", "AssessmentModel", "SDS", map[string]any{
		"kind":       "scale",
		"code":       "SDS",
		"version":    "1.0.0",
		"title":      "SDS",
		"action":     "published",
		"changed_at": time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC),
	})

	if err := handleAssessmentModelChanged(deps)(context.Background(), "assessment_model.changed", payload); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if client.scaleQRCodeCalls != 1 {
		t.Fatalf("expected 1 scale post-action call, got %d", client.scaleQRCodeCalls)
	}
}

func TestAssessmentModelChangedHandler_RejectsMalformedPayload(t *testing.T) {
	deps := &Dependencies{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	err := handleAssessmentModelChanged(deps)(context.Background(), "assessment_model.changed", []byte("{"))
	if err == nil {
		t.Fatal("expected malformed payload to be rejected")
	}
}

func mustBuildLifecycleChangedPayload(t *testing.T, eventType, aggregateType, aggregateID string, data map[string]any) []byte {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"id":            "evt-lifecycle",
		"eventType":     eventType,
		"occurredAt":    time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC),
		"aggregateType": aggregateType,
		"aggregateID":   aggregateID,
		"data":          data,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return payload
}
