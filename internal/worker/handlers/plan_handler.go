package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

func init() {
	Register("plan_created_handler", func(deps *Dependencies) HandlerFunc {
		return handlePlanCreated(deps)
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
}

// ==================== Payload 定义 ====================

// PlanCreatedPayload 计划创建事件数据
type PlanCreatedPayload struct {
	PlanID    string    `json:"plan_id"`
	ScaleID   string    `json:"scale_id"`
	CreatedAt time.Time `json:"created_at"`
}

// TaskOpenedPayload 任务开放事件数据
type TaskOpenedPayload struct {
	TaskID   string    `json:"task_id"`
	PlanID   string    `json:"plan_id"`
	TesteeID string    `json:"testee_id"`
	EntryURL string    `json:"entry_url"`
	OpenAt   time.Time `json:"open_at"`
}

// TaskCompletedPayload 任务完成事件数据
type TaskCompletedPayload struct {
	TaskID       string    `json:"task_id"`
	PlanID       string    `json:"plan_id"`
	AssessmentID string    `json:"assessment_id"`
	CompletedAt  time.Time `json:"completed_at"`
}

// TaskExpiredPayload 任务过期事件数据
type TaskExpiredPayload struct {
	TaskID    string    `json:"task_id"`
	PlanID    string    `json:"plan_id"`
	ExpiredAt time.Time `json:"expired_at"`
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
			slog.String("scale_id", data.ScaleID),
			slog.Time("created_at", data.CreatedAt),
		)

		// TODO: 更新统计指标
		// - 计划创建数量
		// - 按量表分组的计划数量

		// TODO: 可选：预热计划相关缓存

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
			slog.String("entry_url", data.EntryURL),
			slog.Time("open_at", data.OpenAt),
		)

		// TODO: 发送通知给受试者
		// 通知内容：
		// - 标题：测评提醒
		// - 内容：您有一个新的测评任务，请点击链接完成测评
		// - 链接：data.EntryURL
		// - 截止时间：从任务信息中获取（需要查询任务详情）
		//
		// 实现方式：
		// - 方案A：调用通知服务 API（HTTP/gRPC）
		// - 方案B：直接发送到消息队列（如 NSQ/RabbitMQ）
		// - 方案C：集成第三方通知服务（如极光推送、阿里云短信）

		// TODO: 更新统计指标
		// - 任务开放数量
		// - 按计划分组的任务开放数量

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
			slog.String("assessment_id", data.AssessmentID),
			slog.Time("completed_at", data.CompletedAt),
		)

		// TODO: 更新统计指标
		// - 任务完成数量
		// - 计划完成率（已完成任务数 / 总任务数）
		// - 按计划分组的完成率
		// - 按受试者分组的完成进度

		// TODO: 可选：发送完成确认通知
		// 通知内容：
		// - 标题：测评已完成
		// - 内容：您已完成本次测评，感谢您的配合

		// TODO: 可选：触发报告生成流程
		// 如果计划配置了自动生成报告，调用报告生成服务
		// 注意：报告生成可能需要等待评估完成（assessment.interpreted 事件）

		// TODO: 可选：检查测评结果风险等级
		// 如果测评结果风险等级高，触发预警流程
		// 注意：需要等待 assessment.interpreted 事件，获取风险等级信息
		// 可以通过查询 assessment 状态或订阅 assessment.interpreted 事件来实现

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
			slog.Time("expired_at", data.ExpiredAt),
		)

		// TODO: 更新统计指标
		// - 任务过期数量
		// - 计划完成率（需要排除过期任务）
		// - 按计划分组的过期率

		// TODO: 可选：发送过期提醒通知
		// 通知内容：
		// - 标题：测评已过期
		// - 内容：您的测评任务已过期，如有疑问请联系管理员

		// TODO: 可选：分析过期原因
		// - 记录过期任务的相关信息
		// - 生成过期原因分析报告

		return nil
	}
}
