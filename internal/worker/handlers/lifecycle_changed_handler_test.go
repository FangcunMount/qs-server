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

func TestScaleChangedHandler_NonPublishedSkipsQRCode(t *testing.T) {
	client := &fakeWorkerInternalClient{}
	deps := &Dependencies{
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		InternalClient: client,
	}

	payload := mustBuildLifecycleChangedPayload(t, "scale.changed", "MedicalScale", "12", map[string]any{
		"scale_id":   12,
		"code":       "SDS",
		"version":    "",
		"name":       "SDS",
		"action":     "updated",
		"changed_at": time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC),
	})

	if err := handleScaleChanged(deps)(context.Background(), "scale.changed", payload); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	if client.scaleQRCodeCalls != 0 {
		t.Fatalf("expected 0 scale QR calls, got %d", client.scaleQRCodeCalls)
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
