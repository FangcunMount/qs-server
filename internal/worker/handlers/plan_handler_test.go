package handlers

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParsePlanCreatedEventData_UsesScaleCode(t *testing.T) {
	now := time.Date(2026, 3, 24, 10, 0, 0, 0, time.UTC)

	payload, err := json.Marshal(map[string]any{
		"id":            "evt-1",
		"eventType":     "plan.created",
		"occurredAt":    now,
		"aggregateType": "AssessmentPlan",
		"aggregateID":   "plan-1",
		"data": map[string]any{
			"plan_id":    "plan-1",
			"scale_code": "SDS",
			"created_at": now,
		},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	var data PlanCreatedPayload
	_, err = ParseEventData(payload, &data)
	if err != nil {
		t.Fatalf("parse event data: %v", err)
	}

	if data.PlanID != "plan-1" {
		t.Fatalf("unexpected plan id: %q", data.PlanID)
	}
	if data.ScaleCode != "SDS" {
		t.Fatalf("unexpected scale code: %q", data.ScaleCode)
	}
	if !data.CreatedAt.Equal(now) {
		t.Fatalf("unexpected created_at: %v", data.CreatedAt)
	}
}
