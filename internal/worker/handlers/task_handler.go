package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

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

func init() {
	Register("task_opened_handler", func(deps *Dependencies) HandlerFunc {
		return handleTaskOpened(deps)
	})
	Register("task_completed_handler", func(deps *Dependencies) HandlerFunc {
		return handleTaskCompleted(deps)
	})
	Register("task_expired_handler", func(deps *Dependencies) HandlerFunc {
		return handleTaskExpired(deps)
	})
	Register("task_canceled_handler", func(deps *Dependencies) HandlerFunc {
		return handleTaskCanceled(deps)
	})
}

func handleTaskOpened(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
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
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data domainPlan.TaskCompletedData
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse task completed event: %w", err)
		}

		deps.Logger.Info("processing task completed",
			slog.String("event_id", env.ID),
			slog.String("task_id", data.TaskID),
			slog.String("plan_id", data.PlanID),
			slog.String("testee_id", data.TesteeID),
			slog.String("assessment_id", data.AssessmentID),
			slog.Time("completed_at", data.CompletedAt),
		)

		if deps.Notifier != nil {
			if err := deps.Notifier.NotifyTaskCompleted(ctx, notificationMetaFromEnvelope(env), port.TaskCompletedNotification{
				TaskID:       data.TaskID,
				PlanID:       data.PlanID,
				TesteeID:     data.TesteeID,
				AssessmentID: data.AssessmentID,
				CompletedAt:  data.CompletedAt,
			}); err != nil {
				deps.Logger.Warn("failed to notify task completed",
					slog.String("task_id", data.TaskID),
					slog.String("testee_id", data.TesteeID),
					slog.String("error", err.Error()),
				)
			}
		}

		return nil
	}
}

func handleTaskExpired(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data domainPlan.TaskExpiredData
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse task expired event: %w", err)
		}

		deps.Logger.Info("processing task expired",
			slog.String("event_id", env.ID),
			slog.String("task_id", data.TaskID),
			slog.String("plan_id", data.PlanID),
			slog.String("testee_id", data.TesteeID),
			slog.Time("expired_at", data.ExpiredAt),
		)

		if deps.Notifier != nil {
			if err := deps.Notifier.NotifyTaskExpired(ctx, notificationMetaFromEnvelope(env), port.TaskExpiredNotification{
				TaskID:    data.TaskID,
				PlanID:    data.PlanID,
				TesteeID:  data.TesteeID,
				ExpiredAt: data.ExpiredAt,
			}); err != nil {
				deps.Logger.Warn("failed to notify task expired",
					slog.String("task_id", data.TaskID),
					slog.String("testee_id", data.TesteeID),
					slog.String("error", err.Error()),
				)
			}
		}

		return nil
	}
}

func handleTaskCanceled(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data domainPlan.TaskCanceledData
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse task canceled event: %w", err)
		}

		deps.Logger.Info("processing task canceled",
			slog.String("event_id", env.ID),
			slog.String("task_id", data.TaskID),
			slog.String("plan_id", data.PlanID),
			slog.String("testee_id", data.TesteeID),
			slog.Time("canceled_at", data.CanceledAt),
		)

		if deps.Notifier != nil {
			if err := deps.Notifier.NotifyTaskCanceled(ctx, notificationMetaFromEnvelope(env), port.TaskCanceledNotification{
				TaskID:     data.TaskID,
				PlanID:     data.PlanID,
				TesteeID:   data.TesteeID,
				CanceledAt: data.CanceledAt,
			}); err != nil {
				deps.Logger.Warn("failed to notify task canceled",
					slog.String("task_id", data.TaskID),
					slog.String("testee_id", data.TesteeID),
					slog.String("error", err.Error()),
				)
			}
		}
		return nil
	}
}
