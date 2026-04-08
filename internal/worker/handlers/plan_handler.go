package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	planApp "github.com/FangcunMount/qs-server/internal/apiserver/application/plan"
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
	Register("plan_created_handler", func(deps *Dependencies) HandlerFunc {
		return handlePlanCreated(deps)
	})
	Register("plan_testee_enrolled_handler", func(deps *Dependencies) HandlerFunc {
		return handlePlanTesteeEnrolled(deps)
	})
	Register("plan_testee_terminated_handler", func(deps *Dependencies) HandlerFunc {
		return handlePlanTesteeTerminated(deps)
	})
	Register("plan_paused_handler", func(deps *Dependencies) HandlerFunc {
		return handlePlanPaused(deps)
	})
	Register("plan_resumed_handler", func(deps *Dependencies) HandlerFunc {
		return handlePlanResumed(deps)
	})
	Register("plan_canceled_handler", func(deps *Dependencies) HandlerFunc {
		return handlePlanCanceled(deps)
	})
	Register("plan_finished_handler", func(deps *Dependencies) HandlerFunc {
		return handlePlanFinished(deps)
	})
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

// ==================== Payload 定义 ====================

// PlanCreatedPayload 计划创建事件数据
type PlanCreatedPayload struct {
	PlanID    string    `json:"plan_id"`
	ScaleCode string    `json:"scale_code"`
	CreatedAt time.Time `json:"created_at"`
}

// PlanTesteeEnrolledPayload 受试者加入计划事件数据
type PlanTesteeEnrolledPayload struct {
	PlanID           string    `json:"plan_id"`
	TesteeID         string    `json:"testee_id"`
	OrgID            int64     `json:"org_id"`
	Idempotent       bool      `json:"idempotent"`
	CreatedTaskCount int       `json:"created_task_count"`
	OccurredAt       time.Time `json:"occurred_at"`
}

// PlanTesteeTerminatedPayload 受试者终止计划参与事件数据
type PlanTesteeTerminatedPayload struct {
	PlanID            string    `json:"plan_id"`
	TesteeID          string    `json:"testee_id"`
	OrgID             int64     `json:"org_id"`
	AffectedTaskCount int       `json:"affected_task_count"`
	OccurredAt        time.Time `json:"occurred_at"`
}

// PlanPausedPayload 计划暂停事件数据
type PlanPausedPayload struct {
	PlanID    string    `json:"plan_id"`
	ScaleCode string    `json:"scale_code"`
	PausedAt  time.Time `json:"paused_at"`
}

// PlanResumedPayload 计划恢复事件数据
type PlanResumedPayload struct {
	PlanID    string    `json:"plan_id"`
	ScaleCode string    `json:"scale_code"`
	ResumedAt time.Time `json:"resumed_at"`
}

// PlanCanceledPayload 计划取消事件数据
type PlanCanceledPayload struct {
	PlanID     string    `json:"plan_id"`
	ScaleCode  string    `json:"scale_code"`
	CanceledAt time.Time `json:"canceled_at"`
}

// PlanFinishedPayload 计划完成事件数据
type PlanFinishedPayload struct {
	PlanID     string    `json:"plan_id"`
	ScaleCode  string    `json:"scale_code"`
	FinishedAt time.Time `json:"finished_at"`
}

// TaskOpenedPayload 任务开放事件数据
type TaskOpenedPayload struct {
	TaskID   string    `json:"task_id"`
	PlanID   string    `json:"plan_id"`
	TesteeID string    `json:"testee_id"`
	EntryURL string    `json:"entry_url"`
	OpenAt   time.Time `json:"open_at"`
	Source   string    `json:"source,omitempty"`
}

// TaskCompletedPayload 任务完成事件数据
type TaskCompletedPayload struct {
	TaskID       string    `json:"task_id"`
	PlanID       string    `json:"plan_id"`
	TesteeID     string    `json:"testee_id"`
	AssessmentID string    `json:"assessment_id"`
	CompletedAt  time.Time `json:"completed_at"`
}

// TaskExpiredPayload 任务过期事件数据
type TaskExpiredPayload struct {
	TaskID    string    `json:"task_id"`
	PlanID    string    `json:"plan_id"`
	TesteeID  string    `json:"testee_id"`
	ExpiredAt time.Time `json:"expired_at"`
}

// TaskCanceledPayload 任务取消事件数据
type TaskCanceledPayload struct {
	TaskID     string    `json:"task_id"`
	PlanID     string    `json:"plan_id"`
	TesteeID   string    `json:"testee_id"`
	CanceledAt time.Time `json:"canceled_at"`
}

// ==================== Handler 实现 ====================

// handlePlanCreated 处理计划创建事件
// 业务逻辑：
// 1. 记录计划创建日志
// 2. 更新统计指标（计划创建数量）
// 3. 可选：预热相关缓存
func handlePlanCreated(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data PlanCreatedPayload
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse plan created event: %w", err)
		}

		deps.Logger.Info("processing plan created",
			slog.String("event_id", env.ID),
			slog.String("plan_id", data.PlanID),
			slog.String("scale_code", data.ScaleCode),
			slog.Time("created_at", data.CreatedAt),
		)

		return nil
	}
}

// handlePlanTesteeEnrolled 处理受试者加入计划事件
func handlePlanTesteeEnrolled(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data PlanTesteeEnrolledPayload
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse plan testee enrolled event: %w", err)
		}

		deps.Logger.Info("processing plan testee enrolled",
			slog.String("event_id", env.ID),
			slog.String("plan_id", data.PlanID),
			slog.String("testee_id", data.TesteeID),
			slog.Int64("org_id", data.OrgID),
			slog.Bool("idempotent", data.Idempotent),
			slog.Int("created_task_count", data.CreatedTaskCount),
			slog.Time("occurred_at", data.OccurredAt),
		)

		return nil
	}
}

// handlePlanTesteeTerminated 处理受试者终止计划参与事件
func handlePlanTesteeTerminated(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data PlanTesteeTerminatedPayload
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse plan testee terminated event: %w", err)
		}

		deps.Logger.Info("processing plan testee terminated",
			slog.String("event_id", env.ID),
			slog.String("plan_id", data.PlanID),
			slog.String("testee_id", data.TesteeID),
			slog.Int64("org_id", data.OrgID),
			slog.Int("affected_task_count", data.AffectedTaskCount),
			slog.Time("occurred_at", data.OccurredAt),
		)

		return nil
	}
}

// handlePlanPaused 处理计划暂停事件
func handlePlanPaused(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data PlanPausedPayload
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse plan paused event: %w", err)
		}

		deps.Logger.Info("processing plan paused",
			slog.String("event_id", env.ID),
			slog.String("plan_id", data.PlanID),
			slog.String("scale_code", data.ScaleCode),
			slog.Time("paused_at", data.PausedAt),
		)
		return nil
	}
}

// handlePlanResumed 处理计划恢复事件
func handlePlanResumed(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data PlanResumedPayload
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse plan resumed event: %w", err)
		}

		deps.Logger.Info("processing plan resumed",
			slog.String("event_id", env.ID),
			slog.String("plan_id", data.PlanID),
			slog.String("scale_code", data.ScaleCode),
			slog.Time("resumed_at", data.ResumedAt),
		)
		return nil
	}
}

// handlePlanCanceled 处理计划取消事件
func handlePlanCanceled(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data PlanCanceledPayload
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse plan canceled event: %w", err)
		}

		deps.Logger.Info("processing plan canceled",
			slog.String("event_id", env.ID),
			slog.String("plan_id", data.PlanID),
			slog.String("scale_code", data.ScaleCode),
			slog.Time("canceled_at", data.CanceledAt),
		)
		return nil
	}
}

// handlePlanFinished 处理计划完成事件
func handlePlanFinished(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data PlanFinishedPayload
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse plan finished event: %w", err)
		}

		deps.Logger.Info("processing plan finished",
			slog.String("event_id", env.ID),
			slog.String("plan_id", data.PlanID),
			slog.String("scale_code", data.ScaleCode),
			slog.Time("finished_at", data.FinishedAt),
		)
		return nil
	}
}

// handleTaskOpened 处理任务开放事件
// 业务逻辑：
// 1. 记录任务开放日志
// 2. 发送通知给受试者（短信/小程序推送/邮件）
//   - 通知内容：测评入口链接、截止时间、提醒文案
//
// 3. 更新统计指标（任务开放数量）
func handleTaskOpened(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data TaskOpenedPayload
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse task opened event: %w", err)
		}

		deps.Logger.Info("processing task opened",
			slog.String("event_id", env.ID),
			slog.String("task_id", data.TaskID),
			slog.String("plan_id", data.PlanID),
			slog.String("testee_id", data.TesteeID),
			slog.String("source", data.Source),
			slog.String("entry_url", data.EntryURL),
			slog.Time("open_at", data.OpenAt),
		)

		if data.Source == planApp.TaskSchedulerSourceSeedData {
			deps.Logger.Info("skip task opened mini program notification for seeddata source",
				slog.String("task_id", data.TaskID),
				slog.String("source", data.Source),
			)
			return nil
		}

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

// handleTaskCompleted 处理任务完成事件
// 业务逻辑：
// 1. 记录任务完成日志
// 2. 更新统计指标（任务完成数量、计划完成率）
// 3. 可选：发送完成确认通知给受试者
// 4. 可选：触发报告生成流程（如果计划配置了自动生成报告）
// 5. 可选：检查测评结果，如果风险等级高，触发预警流程
func handleTaskCompleted(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data TaskCompletedPayload
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

// handleTaskExpired 处理任务过期事件
// 业务逻辑：
// 1. 记录任务过期日志
// 2. 更新统计指标（任务过期数量、完成率）
// 3. 可选：发送过期提醒通知给受试者
// 4. 可选：分析过期原因，生成报告
func handleTaskExpired(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data TaskExpiredPayload
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

// handleTaskCanceled 处理任务取消事件
func handleTaskCanceled(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data TaskCanceledPayload
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
