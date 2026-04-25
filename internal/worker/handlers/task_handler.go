package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	domainPlan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/worker/port"
)

func notificationMetaFromEnvelope(env *EventEnvelope) port.NotificationMeta {
	if env == nil {
		return port.NotificationMeta{}
	}
	return port.NotificationMeta{
		EventID:       env.ID,
		EventType:     env.EventType,
		AggregateType: env.AggregateType,
		AggregateID:   env.AggregateID,
		OccurredAt:    env.OccurredAt,
	}
}

type taskNotificationCallbacks[T any] struct {
	parseErrorLabel     string
	logMessage          string
	logFields           func(env *EventEnvelope, data *T) []any
	notify              func(ctx context.Context, notifier port.TaskNotifier, meta port.NotificationMeta, data *T) error
	notifyFailureLog    string
	notifyFailureFields func(data *T) []any
}

func handleTaskOpened(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, _ string, payload []byte) error {
		var data domainPlan.TaskOpenedData
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse task opened event: %w", err)
		}

		deps.Logger.Info("processing task opened",
			slog.String("event_id", env.ID),
			slog.String("task_id", data.TaskID),
			slog.String("plan_id", data.PlanID),
			slog.String("testee_id", data.TesteeID),
			slog.String("entry_url", data.EntryURL),
			slog.Time("open_at", data.OpenAt),
		)

		if deps.InternalClient != nil {
			testeeID, parseErr := strconv.ParseUint(data.TesteeID, 10, 64)
			if parseErr != nil {
				deps.Logger.Warn("failed to parse testee id for mini program notification",
					slog.String("task_id", data.TaskID),
					slog.String("testee_id", data.TesteeID),
					slog.String("error", parseErr.Error()),
				)
			} else {
				resp, notifyErr := deps.InternalClient.SendTaskOpenedMiniProgramNotification(
					ctx,
					0,
					data.TaskID,
					testeeID,
					data.EntryURL,
					data.OpenAt,
				)
				if notifyErr != nil {
					deps.Logger.Warn("failed to send task opened mini program notification",
						slog.String("task_id", data.TaskID),
						slog.String("testee_id", data.TesteeID),
						slog.String("error", notifyErr.Error()),
					)
				} else {
					deps.Logger.Info("task opened mini program notification processed",
						slog.String("task_id", data.TaskID),
						slog.Int("sent_count", int(resp.GetSentCount())),
						slog.Bool("skipped", resp.GetSkipped()),
						slog.String("recipient_source", resp.GetRecipientSource()),
						slog.String("message", resp.GetMessage()),
					)
				}
			}
		}

		return nil
	}
}

func handleTaskCompleted(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, _ string, payload []byte) error {
		return handleTaskNotificationEvent(ctx, deps, payload, taskNotificationCallbacks[domainPlan.TaskCompletedData]{
			parseErrorLabel: "task completed event",
			logMessage:      "processing task completed",
			logFields: func(env *EventEnvelope, data *domainPlan.TaskCompletedData) []any {
				return []any{
					slog.String("event_id", env.ID),
					slog.String("task_id", data.TaskID),
					slog.String("plan_id", data.PlanID),
					slog.String("testee_id", data.TesteeID),
					slog.String("assessment_id", data.AssessmentID),
					slog.Time("completed_at", data.CompletedAt),
				}
			},
			notify: func(ctx context.Context, notifier port.TaskNotifier, meta port.NotificationMeta, data *domainPlan.TaskCompletedData) error {
				return notifier.NotifyTaskCompleted(ctx, meta, port.TaskCompletedNotification{
					TaskID:       data.TaskID,
					PlanID:       data.PlanID,
					TesteeID:     data.TesteeID,
					AssessmentID: data.AssessmentID,
					CompletedAt:  data.CompletedAt,
				})
			},
			notifyFailureLog: "failed to notify task completed",
			notifyFailureFields: func(data *domainPlan.TaskCompletedData) []any {
				return []any{
					slog.String("task_id", data.TaskID),
					slog.String("testee_id", data.TesteeID),
				}
			},
		})
	}
}

func handleTaskExpired(deps *Dependencies) HandlerFunc {
	return handleTimedTaskNotificationHandler(deps, taskTimedNotificationCallbacks[domainPlan.TaskExpiredData]{
		parseErrorLabel:  "task expired event",
		logMessage:       "processing task expired",
		timeFieldName:    "expired_at",
		taskID:           taskExpiredID,
		planID:           taskExpiredPlanID,
		testeeID:         taskExpiredTesteeID,
		timestamp:        taskExpiredAt,
		notify:           notifyTaskExpired,
		notifyFailureLog: "failed to notify task expired",
	})
}

func handleTaskCanceled(deps *Dependencies) HandlerFunc {
	return handleTimedTaskNotificationHandler(deps, taskTimedNotificationCallbacks[domainPlan.TaskCanceledData]{
		parseErrorLabel:  "task canceled event",
		logMessage:       "processing task canceled",
		timeFieldName:    "canceled_at",
		taskID:           taskCanceledID,
		planID:           taskCanceledPlanID,
		testeeID:         taskCanceledTesteeID,
		timestamp:        taskCanceledAt,
		notify:           notifyTaskCanceled,
		notifyFailureLog: "failed to notify task canceled",
	})
}

func taskExpiredID(data *domainPlan.TaskExpiredData) string       { return data.TaskID }
func taskExpiredPlanID(data *domainPlan.TaskExpiredData) string   { return data.PlanID }
func taskExpiredTesteeID(data *domainPlan.TaskExpiredData) string { return data.TesteeID }
func taskExpiredAt(data *domainPlan.TaskExpiredData) time.Time    { return data.ExpiredAt }

func notifyTaskExpired(
	ctx context.Context,
	notifier port.TaskNotifier,
	meta port.NotificationMeta,
	data *domainPlan.TaskExpiredData,
) error {
	return notifier.NotifyTaskExpired(ctx, meta, port.TaskExpiredNotification{
		TaskID:    data.TaskID,
		PlanID:    data.PlanID,
		TesteeID:  data.TesteeID,
		ExpiredAt: data.ExpiredAt,
	})
}

func taskCanceledID(data *domainPlan.TaskCanceledData) string       { return data.TaskID }
func taskCanceledPlanID(data *domainPlan.TaskCanceledData) string   { return data.PlanID }
func taskCanceledTesteeID(data *domainPlan.TaskCanceledData) string { return data.TesteeID }
func taskCanceledAt(data *domainPlan.TaskCanceledData) time.Time    { return data.CanceledAt }

func notifyTaskCanceled(
	ctx context.Context,
	notifier port.TaskNotifier,
	meta port.NotificationMeta,
	data *domainPlan.TaskCanceledData,
) error {
	return notifier.NotifyTaskCanceled(ctx, meta, port.TaskCanceledNotification{
		TaskID:     data.TaskID,
		PlanID:     data.PlanID,
		TesteeID:   data.TesteeID,
		CanceledAt: data.CanceledAt,
	})
}

func handleTaskNotificationEvent[T any](
	ctx context.Context,
	deps *Dependencies,
	payload []byte,
	callbacks taskNotificationCallbacks[T],
) error {
	data := new(T)
	env, err := ParseEventData(payload, data)
	if err != nil {
		return fmt.Errorf("failed to parse %s: %w", callbacks.parseErrorLabel, err)
	}

	deps.Logger.Info(callbacks.logMessage, callbacks.logFields(env, data)...)
	if deps.Notifier == nil {
		return nil
	}

	if err := callbacks.notify(ctx, deps.Notifier, notificationMetaFromEnvelope(env), data); err != nil {
		fields := callbacks.notifyFailureFields(data)
		fields = append(fields, slog.String("error", err.Error()))
		deps.Logger.Warn(callbacks.notifyFailureLog, fields...)
	}

	return nil
}

type taskTimedNotificationCallbacks[T any] struct {
	parseErrorLabel  string
	logMessage       string
	timeFieldName    string
	taskID           func(data *T) string
	planID           func(data *T) string
	testeeID         func(data *T) string
	timestamp        func(data *T) time.Time
	notify           func(ctx context.Context, notifier port.TaskNotifier, meta port.NotificationMeta, data *T) error
	notifyFailureLog string
}

func handleTimedTaskNotificationHandler[T any](
	deps *Dependencies,
	callbacks taskTimedNotificationCallbacks[T],
) HandlerFunc {
	return func(ctx context.Context, _ string, payload []byte) error {
		return handleTaskNotificationEvent(ctx, deps, payload, taskNotificationCallbacks[T]{
			parseErrorLabel: callbacks.parseErrorLabel,
			logMessage:      callbacks.logMessage,
			logFields: func(env *EventEnvelope, data *T) []any {
				return []any{
					slog.String("event_id", env.ID),
					slog.String("task_id", callbacks.taskID(data)),
					slog.String("plan_id", callbacks.planID(data)),
					slog.String("testee_id", callbacks.testeeID(data)),
					slog.Time(callbacks.timeFieldName, callbacks.timestamp(data)),
				}
			},
			notify:           callbacks.notify,
			notifyFailureLog: callbacks.notifyFailureLog,
			notifyFailureFields: func(data *T) []any {
				return []any{
					slog.String("task_id", callbacks.taskID(data)),
					slog.String("testee_id", callbacks.testeeID(data)),
				}
			},
		})
	}
}
