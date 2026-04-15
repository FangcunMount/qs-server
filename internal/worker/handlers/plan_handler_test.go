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

func TestTaskHandlersNotifyWebhookPayloads(t *testing.T) {
	now := time.Date(2026, 4, 2, 11, 0, 0, 0, time.UTC)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		name   string
		event  string
		data   map[string]any
		handle func(*Dependencies) HandlerFunc
		assert func(t *testing.T, notifier *recordingNotifier)
	}{
		{
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
			assert: func(t *testing.T, notifier *recordingNotifier) {
				if len(notifier.completed) != 0 || len(notifier.expired) != 0 || len(notifier.canceled) != 0 {
					t.Fatalf("expected no notifier call for task.opened, got completed=%d expired=%d canceled=%d",
						len(notifier.completed), len(notifier.expired), len(notifier.canceled))
				}
			},
		},
		{
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
			assert: func(t *testing.T, notifier *recordingNotifier) {
				if len(notifier.completed) != 1 {
					t.Fatalf("expected 1 completed notification, got %d", len(notifier.completed))
				}
				if len(notifier.completedMeta) != 1 || notifier.completedMeta[0].EventType != "task.completed" || notifier.completedMeta[0].EventID != "evt-completed" {
					t.Fatalf("unexpected completed meta: %#v", notifier.completedMeta)
				}
				if notifier.completed[0].TaskID != "task-2" || notifier.completed[0].AssessmentID != "assessment-2" || notifier.completed[0].TesteeID != "testee-2" {
					t.Fatalf("unexpected completed notification: %#v", notifier.completed[0])
				}
			},
		},
		{
			name:  "expired",
			event: "task.expired",
			data: map[string]any{
				"task_id":    "task-3",
				"plan_id":    "plan-3",
				"testee_id":  "testee-3",
				"expired_at": now,
			},
			handle: handleTaskExpired,
			assert: func(t *testing.T, notifier *recordingNotifier) {
				if len(notifier.expired) != 1 {
					t.Fatalf("expected 1 expired notification, got %d", len(notifier.expired))
				}
				if len(notifier.expiredMeta) != 1 || notifier.expiredMeta[0].EventType != "task.expired" || notifier.expiredMeta[0].EventID != "evt-expired" {
					t.Fatalf("unexpected expired meta: %#v", notifier.expiredMeta)
				}
				if notifier.expired[0].TaskID != "task-3" || notifier.expired[0].TesteeID != "testee-3" {
					t.Fatalf("unexpected expired notification: %#v", notifier.expired[0])
				}
			},
		},
		{
			name:  "canceled",
			event: "task.canceled",
			data: map[string]any{
				"task_id":     "task-4",
				"plan_id":     "plan-4",
				"testee_id":   "testee-4",
				"canceled_at": now,
			},
			handle: handleTaskCanceled,
			assert: func(t *testing.T, notifier *recordingNotifier) {
				if len(notifier.canceled) != 1 {
					t.Fatalf("expected 1 canceled notification, got %d", len(notifier.canceled))
				}
				if len(notifier.canceledMeta) != 1 || notifier.canceledMeta[0].EventType != "task.canceled" || notifier.canceledMeta[0].EventID != "evt-canceled" {
					t.Fatalf("unexpected canceled meta: %#v", notifier.canceledMeta)
				}
				if notifier.canceled[0].TaskID != "task-4" || notifier.canceled[0].TesteeID != "testee-4" {
					t.Fatalf("unexpected canceled notification: %#v", notifier.canceled[0])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notifier := &recordingNotifier{}
			deps := &Dependencies{
				Logger:   logger,
				Notifier: notifier,
			}

			payload, err := json.Marshal(map[string]any{
				"id":            "evt-" + tt.name,
				"eventType":     tt.event,
				"occurredAt":    now,
				"aggregateType": "AssessmentTask",
				"aggregateID":   tt.data["task_id"],
				"data":          tt.data,
			})
			if err != nil {
				t.Fatalf("marshal payload: %v", err)
			}

			if err := tt.handle(deps)(context.Background(), tt.event, payload); err != nil {
				t.Fatalf("handler returned error: %v", err)
			}

			tt.assert(t, notifier)
		})
	}
}
