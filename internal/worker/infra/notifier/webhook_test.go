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

func TestWebhookNotifierPostsTaskNotifications(t *testing.T) {
	type requestBody struct {
		SchemaVersion string          `json:"schema_version"`
		EventID       string          `json:"event_id"`
		EventType     string          `json:"event_type"`
		AggregateType string          `json:"aggregate_type"`
		AggregateID   string          `json:"aggregate_id"`
		OccurredAt    time.Time       `json:"occurred_at"`
		Data          json.RawMessage `json:"data"`
	}

	var bodies []requestBody
	var headers []http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			_ = r.Body.Close()
		}()

		var body requestBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		bodies = append(bodies, body)
		headers = append(headers, r.Header.Clone())
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := NewWebhookNotifier(server.URL, time.Second, "secret-for-test")
	now := time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC)
	meta := port.NotificationMeta{
		EventID:       "evt-1",
		EventType:     "task.completed",
		AggregateType: "AssessmentTask",
		AggregateID:   "task-completed",
		OccurredAt:    now,
	}

	if err := notifier.NotifyTaskCompleted(context.Background(), meta, port.TaskCompletedNotification{
		TaskID:       "task-completed",
		PlanID:       "plan-2",
		TesteeID:     "testee-2",
		AssessmentID: "assessment-2",
		CompletedAt:  now,
	}); err != nil {
		t.Fatalf("NotifyTaskCompleted: %v", err)
	}
	meta.EventID = "evt-2"
	meta.EventType = "task.expired"
	meta.AggregateID = "task-expired"
	if err := notifier.NotifyTaskExpired(context.Background(), meta, port.TaskExpiredNotification{
		TaskID:    "task-expired",
		PlanID:    "plan-3",
		TesteeID:  "testee-3",
		ExpiredAt: now,
	}); err != nil {
		t.Fatalf("NotifyTaskExpired: %v", err)
	}
	meta.EventID = "evt-3"
	meta.EventType = "task.canceled"
	meta.AggregateID = "task-canceled"
	if err := notifier.NotifyTaskCanceled(context.Background(), meta, port.TaskCanceledNotification{
		TaskID:     "task-canceled",
		PlanID:     "plan-4",
		TesteeID:   "testee-4",
		CanceledAt: now,
	}); err != nil {
		t.Fatalf("NotifyTaskCanceled: %v", err)
	}

	if len(bodies) != 3 {
		t.Fatalf("expected 3 webhook requests, got %d", len(bodies))
	}

	eventTypes := []string{"task.completed", "task.expired", "task.canceled"}
	for i, eventType := range eventTypes {
		if bodies[i].SchemaVersion != webhookSchemaVersion {
			t.Fatalf("unexpected schema version at index %d: %s", i, bodies[i].SchemaVersion)
		}
		if bodies[i].EventType != eventType {
			t.Fatalf("unexpected event type at index %d: got %s want %s", i, bodies[i].EventType, eventType)
		}
		if bodies[i].AggregateType != "AssessmentTask" {
			t.Fatalf("unexpected aggregate type at index %d: %s", i, bodies[i].AggregateType)
		}
		if bodies[i].EventID == "" || bodies[i].OccurredAt.IsZero() {
			t.Fatalf("expected event metadata at index %d: %#v", i, bodies[i])
		}
		if len(bodies[i].Data) == 0 {
			t.Fatalf("expected data payload at index %d", i)
		}
		if headers[i].Get("X-QS-Notification-Version") != webhookSchemaVersion {
			t.Fatalf("unexpected notification version header at index %d: %s", i, headers[i].Get("X-QS-Notification-Version"))
		}
		if headers[i].Get("X-QS-Event-Type") != eventType {
			t.Fatalf("unexpected event type header at index %d: %s", i, headers[i].Get("X-QS-Event-Type"))
		}
		if headers[i].Get("X-QS-Event-ID") == "" {
			t.Fatalf("expected event id header at index %d", i)
		}
		if headers[i].Get("X-QS-Occurred-At") == "" {
			t.Fatalf("expected occurred_at header at index %d", i)
		}
		if headers[i].Get("X-QS-Signature-Alg") != webhookSignatureAlgorithm {
			t.Fatalf("unexpected signature alg header at index %d: %s", i, headers[i].Get("X-QS-Signature-Alg"))
		}
		expectedSignature, err := json.Marshal(webhookEnvelope{
			SchemaVersion: bodies[i].SchemaVersion,
			EventID:       bodies[i].EventID,
			EventType:     bodies[i].EventType,
			AggregateType: bodies[i].AggregateType,
			AggregateID:   bodies[i].AggregateID,
			OccurredAt:    bodies[i].OccurredAt,
			Data:          json.RawMessage(bodies[i].Data),
		})
		if err != nil {
			t.Fatalf("marshal expected body at index %d: %v", i, err)
		}
		if headers[i].Get("X-QS-Signature") != signWebhookPayload([]byte("secret-for-test"), expectedSignature) {
			t.Fatalf("unexpected signature header at index %d: %s", i, headers[i].Get("X-QS-Signature"))
		}
	}
}

func TestWebhookNotifierOmitsSignatureHeadersWhenSecretEmpty(t *testing.T) {
	var header http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	notifier := NewWebhookNotifier(server.URL, time.Second, "")
	err := notifier.NotifyTaskCompleted(context.Background(), port.NotificationMeta{
		EventID:    "evt-no-secret",
		EventType:  "task.completed",
		OccurredAt: time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC),
	}, port.TaskCompletedNotification{
		TaskID:       "task-completed",
		PlanID:       "plan-1",
		TesteeID:     "testee-1",
		AssessmentID: "assessment-1",
		CompletedAt:  time.Date(2026, 4, 2, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("NotifyTaskCompleted: %v", err)
	}
	if got := header.Get("X-QS-Signature"); got != "" {
		t.Fatalf("expected signature header to be empty, got %q", got)
	}
	if got := header.Get("X-QS-Signature-Alg"); got != "" {
		t.Fatalf("expected signature alg header to be empty, got %q", got)
	}
}
