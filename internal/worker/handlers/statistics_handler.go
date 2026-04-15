package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	domainAssessment "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	statisticsCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/statistics"
)

func init() {
	Register("statistics_assessment_submitted_handler", func(deps *Dependencies) HandlerFunc {
		return handleStatisticsAssessmentSubmitted(deps)
	})
	Register("statistics_assessment_interpreted_handler", func(deps *Dependencies) HandlerFunc {
		return handleStatisticsAssessmentInterpreted(deps)
	})
}

// ==================== Handler 实现 ====================

// handleStatisticsAssessmentSubmitted 处理测评提交事件（统计更新）
// 业务逻辑：
// 1. 幂等性检查
// 2. 更新Redis预聚合数据
//   - 每日统计（stats:daily）
func handleStatisticsAssessmentSubmitted(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data domainAssessment.AssessmentSubmittedData
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse assessment submitted event: %w", err)
		}

		deps.Logger.Debug("assessment submitted statistics payload",
			"event_id", env.ID,
			"org_id", data.OrgID,
			"assessment_id", data.AssessmentID,
			"testee_id", data.TesteeID,
			"questionnaire_code", data.QuestionnaireCode,
		)

		// 检查Redis是否可用
		if deps.RedisCache == nil {
			deps.Logger.Warn("Redis cache is not available, skipping statistics update",
				slog.String("event_id", env.ID),
			)
			return nil
		}

		// 创建统计缓存实例
		cache := statisticsCache.NewStatisticsCache(deps.RedisCache)

		// 幂等性检查
		processed, err := cache.IsEventProcessed(ctx, env.ID)
		if err != nil {
			deps.Logger.Error("failed to check event processed status",
				slog.String("event_id", env.ID),
				slog.String("error", err.Error()),
			)
			// 继续处理，不因检查失败而中断
		} else if processed {
			deps.Logger.Info("event already processed, skipping",
				slog.String("event_id", env.ID),
			)
			return nil
		}

		orgID := data.OrgID
		if orgID <= 0 {
			deps.Logger.Warn("missing org_id in assessment submitted event, skipping statistics update",
				slog.String("event_id", env.ID),
				slog.Int64("assessment_id", data.AssessmentID),
			)
			return nil
		}
		// 获取当前日期
		now := time.Now()
		today := now

		// 1. 更新每日统计（提交数）
		if err := cache.IncrementDailyCount(ctx, orgID, statistics.StatisticTypeQuestionnaire, data.QuestionnaireCode, today, "submission"); err != nil {
			deps.Logger.Error("failed to increment daily count",
				slog.String("event_id", env.ID),
				slog.String("error", err.Error()),
			)
			// 继续处理其他统计，不因单个失败而中断
		}

		// 标记事件已处理（TTL=7天）
		if err := cache.MarkEventProcessed(ctx, env.ID, 7*24*time.Hour); err != nil {
			deps.Logger.Error("failed to mark event as processed",
				slog.String("event_id", env.ID),
				slog.String("error", err.Error()),
			)
		}

		deps.Logger.Info("statistics updated for assessment submitted",
			slog.String("event_id", env.ID),
			slog.Int64("assessment_id", data.AssessmentID),
		)

		return nil
	}
}

// handleStatisticsAssessmentInterpreted 处理测评解读完成事件（统计更新）
// 业务逻辑：
// 1. 幂等性检查
// 2. 更新Redis预聚合数据
//   - 每日统计（完成数）
func handleStatisticsAssessmentInterpreted(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data domainAssessment.AssessmentInterpretedData
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse assessment interpreted event: %w", err)
		}

		deps.Logger.Debug("assessment interpreted statistics payload",
			"event_id", env.ID,
			"org_id", data.OrgID,
			"assessment_id", data.AssessmentID,
			"testee_id", data.TesteeID,
			"scale_code", data.ScaleCode,
			"risk_level", data.RiskLevel,
		)

		// 检查Redis是否可用
		if deps.RedisCache == nil {
			deps.Logger.Warn("Redis cache is not available, skipping statistics update",
				slog.String("event_id", env.ID),
			)
			return nil
		}

		// 创建统计缓存实例
		cache := statisticsCache.NewStatisticsCache(deps.RedisCache)

		// 幂等性检查
		processed, err := cache.IsEventProcessed(ctx, env.ID)
		if err != nil {
			deps.Logger.Error("failed to check event processed status",
				slog.String("event_id", env.ID),
				slog.String("error", err.Error()),
			)
		} else if processed {
			deps.Logger.Info("event already processed, skipping",
				slog.String("event_id", env.ID),
			)
			return nil
		}

		orgID := data.OrgID
		if orgID <= 0 {
			deps.Logger.Warn("missing org_id in assessment interpreted event, skipping statistics update",
				slog.String("event_id", env.ID),
				slog.Int64("assessment_id", data.AssessmentID),
			)
			return nil
		}
		// ScaleCode 通常等同于 QuestionnaireCode（量表就是问卷）
		questionnaireCode := data.ScaleCode

		// 获取当前日期
		now := time.Now()
		today := now

		// 1. 更新每日统计（完成数）
		if questionnaireCode != "" {
			if err := cache.IncrementDailyCount(ctx, orgID, statistics.StatisticTypeQuestionnaire, questionnaireCode, today, "completion"); err != nil {
				deps.Logger.Error("failed to increment daily completion count",
					slog.String("event_id", env.ID),
					slog.String("error", err.Error()),
				)
			}
		}

		// 标记事件已处理（TTL=7天）
		if err := cache.MarkEventProcessed(ctx, env.ID, 7*24*time.Hour); err != nil {
			deps.Logger.Error("failed to mark event as processed",
				slog.String("event_id", env.ID),
				slog.String("error", err.Error()),
			)
		}

		deps.Logger.Debug("statistics updated for assessment interpreted",
			"event_id", env.ID,
			"assessment_id", data.AssessmentID,
		)

		return nil
	}
}
