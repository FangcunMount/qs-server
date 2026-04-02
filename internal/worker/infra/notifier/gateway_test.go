package notifier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/worker/port"
)

func TestGatewayNotifierPostsTaskNotifications(t *testing.T) {
	type requestBody struct {
		SchemaVersion    string                `json:"schema_version"`
		NotificationType string                `json:"notification_type"`
		TemplateCode     string                `json:"template_code"`
		Event            port.NotificationMeta `json:"event"`
		Recipient        struct {
			TesteeID string `json:"testee_id"`
		} `json:"recipient"`
		Data json.RawMessage `json:"data"`
	}

	var body requestBody
	var header http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		header = r.Header.Clone()
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := NewGatewayNotifier(server.URL, "gateway-token", time.Second)
	now := time.Date(2026, 4, 2, 14, 0, 0, 0, time.UTC)
	err := notifier.NotifyTaskCompleted(context.Background(), port.NotificationMeta{
		EventID:       "evt-gateway",
		EventType:     "task.completed",
		AggregateType: "AssessmentTask",
		AggregateID:   "task-1",
		OccurredAt:    now,
	}, port.TaskCompletedNotification{
		TaskID:       "task-1",
		PlanID:       "plan-1",
		TesteeID:     "testee-1",
		AssessmentID: "assessment-1",
		CompletedAt:  now,
	})
	if err != nil {
		t.Fatalf("NotifyTaskCompleted returned error: %v", err)
	}

	if body.SchemaVersion != gatewaySchemaVersion {
		t.Fatalf("unexpected schema version: %s", body.SchemaVersion)
	}
	if body.NotificationType != "task.completed" {
		t.Fatalf("unexpected notification type: %s", body.NotificationType)
	}
	if body.TemplateCode != "plan_task_completed" {
		t.Fatalf("unexpected template code: %s", body.TemplateCode)
	}
	if body.Event.EventID != "evt-gateway" || body.Event.AggregateID != "task-1" {
		t.Fatalf("unexpected event meta: %#v", body.Event)
	}
	if body.Recipient.TesteeID != "testee-1" {
		t.Fatalf("unexpected recipient: %#v", body.Recipient)
	}
	if len(body.Data) == 0 {
		t.Fatalf("expected data payload")
	}
	if got := header.Get("Authorization"); got != "Bearer gateway-token" {
		t.Fatalf("unexpected authorization header: %s", got)
	}
	if got := header.Get("X-QS-Event-Type"); got != "task.completed" {
		t.Fatalf("unexpected event type header: %s", got)
	}
}
