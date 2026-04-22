package handlers

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/worker/port"
)

type recordingNotifier struct {
	completedMeta []port.NotificationMeta
	completed     []port.TaskCompletedNotification
	expiredMeta   []port.NotificationMeta
	expired       []port.TaskExpiredNotification
	canceledMeta  []port.NotificationMeta
	canceled      []port.TaskCanceledNotification
}

func (n *recordingNotifier) NotifyTaskCompleted(_ context.Context, meta port.NotificationMeta, payload port.TaskCompletedNotification) error {
	n.completedMeta = append(n.completedMeta, meta)
	n.completed = append(n.completed, payload)
	return nil
}

func (n *recordingNotifier) NotifyTaskExpired(_ context.Context, meta port.NotificationMeta, payload port.TaskExpiredNotification) error {
	n.expiredMeta = append(n.expiredMeta, meta)
	n.expired = append(n.expired, payload)
	return nil
}

func (n *recordingNotifier) NotifyTaskCanceled(_ context.Context, meta port.NotificationMeta, payload port.TaskCanceledNotification) error {
	n.canceledMeta = append(n.canceledMeta, meta)
	n.canceled = append(n.canceled, payload)
	return nil
}

func TestTaskOpenedDoesNotNotifyWebhookPayloads(t *testing.T) {
	now := time.Date(2026, 4, 2, 11, 0, 0, 0, time.UTC)
	notifier := runTaskHandler(t, taskHandlerCase{
		name:  "opened",
		event: "task.opened",
		data: map[string]any{
			"task_id":   "task-1",
			"plan_id":   "plan-1",
			"testee_id": "testee-1",
			"entry_url": "https://example.com/entry",
			"open_at":   now,
		},
		handle: handleTaskOpened,
	})
	if len(notifier.completed) != 0 || len(notifier.expired) != 0 || len(notifier.canceled) != 0 {
		t.Fatalf("expected no notifier call for task.opened, got completed=%d expired=%d canceled=%d",
			len(notifier.completed), len(notifier.expired), len(notifier.canceled))
	}
}

func TestTaskCompletedNotifiesWebhookPayloads(t *testing.T) {
	now := time.Date(2026, 4, 2, 11, 0, 0, 0, time.UTC)
	notifier := runTaskHandler(t, taskHandlerCase{
		name:  "completed",
		event: "task.completed",
		data: map[string]any{
			"task_id":       "task-2",
			"plan_id":       "plan-2",
			"testee_id":     "testee-2",
			"assessment_id": "assessment-2",
			"completed_at":  now,
		},
		handle: handleTaskCompleted,
	})
	if len(notifier.completed) != 1 {
		t.Fatalf("expected 1 completed notification, got %d", len(notifier.completed))
	}
	if len(notifier.completedMeta) != 1 || notifier.completedMeta[0].EventType != "task.completed" || notifier.completedMeta[0].EventID != "evt-completed" {
		t.Fatalf("unexpected completed meta: %#v", notifier.completedMeta)
	}
	if notifier.completed[0].TaskID != "task-2" || notifier.completed[0].AssessmentID != "assessment-2" || notifier.completed[0].TesteeID != "testee-2" {
		t.Fatalf("unexpected completed notification: %#v", notifier.completed[0])
	}
}

func TestTaskExpiredNotifiesWebhookPayloads(t *testing.T) {
	now := time.Date(2026, 4, 2, 11, 0, 0, 0, time.UTC)
	notifier := runTaskHandler(t, taskHandlerCase{
		name:  "expired",
		event: "task.expired",
		data: map[string]any{
			"task_id":    "task-3",
			"plan_id":    "plan-3",
			"testee_id":  "testee-3",
			"expired_at": now,
		},
		handle: handleTaskExpired,
	})
	if len(notifier.expired) != 1 {
		t.Fatalf("expected 1 expired notification, got %d", len(notifier.expired))
	}
	assertSingleTaskMeta(t, notifier.expiredMeta, "task.expired", "evt-expired")
	assertTaskRecipient(t, notifier.expired[0].TaskID, notifier.expired[0].TesteeID, "task-3", "testee-3")
}

func TestTaskCanceledNotifiesWebhookPayloads(t *testing.T) {
	now := time.Date(2026, 4, 2, 11, 0, 0, 0, time.UTC)
	notifier := runTaskHandler(t, taskHandlerCase{
		name:  "canceled",
		event: "task.canceled",
		data: map[string]any{
			"task_id":     "task-4",
			"plan_id":     "plan-4",
			"testee_id":   "testee-4",
			"canceled_at": now,
		},
		handle: handleTaskCanceled,
	})
	if len(notifier.canceled) != 1 {
		t.Fatalf("expected 1 canceled notification, got %d", len(notifier.canceled))
	}
	assertSingleTaskMeta(t, notifier.canceledMeta, "task.canceled", "evt-canceled")
	assertTaskRecipient(t, notifier.canceled[0].TaskID, notifier.canceled[0].TesteeID, "task-4", "testee-4")
}

type taskHandlerCase struct {
	name   string
	event  string
	data   map[string]any
	handle func(*Dependencies) HandlerFunc
}

func runTaskHandler(t *testing.T, tc taskHandlerCase) *recordingNotifier {
	t.Helper()

	now := time.Date(2026, 4, 2, 11, 0, 0, 0, time.UTC)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	notifier := &recordingNotifier{}
	deps := &Dependencies{
		Logger:   logger,
		Notifier: notifier,
	}

	payload, err := json.Marshal(map[string]any{
		"id":            "evt-" + tc.name,
		"eventType":     tc.event,
		"occurredAt":    now,
		"aggregateType": "AssessmentTask",
		"aggregateID":   tc.data["task_id"],
		"data":          tc.data,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	if err := tc.handle(deps)(context.Background(), tc.event, payload); err != nil {
		t.Fatalf("handler returned error: %v", err)
	}

	return notifier
}

func assertSingleTaskMeta(t *testing.T, meta []port.NotificationMeta, wantType string, wantID string) {
	t.Helper()
	if len(meta) != 1 || meta[0].EventType != wantType || meta[0].EventID != wantID {
		t.Fatalf("unexpected task meta: %#v", meta)
	}
}

func assertTaskRecipient(t *testing.T, gotTaskID string, gotTesteeID string, wantTaskID string, wantTesteeID string) {
	t.Helper()
	if gotTaskID != wantTaskID || gotTesteeID != wantTesteeID {
		t.Fatalf("unexpected task notification: task_id=%q testee_id=%q", gotTaskID, gotTesteeID)
	}
}
