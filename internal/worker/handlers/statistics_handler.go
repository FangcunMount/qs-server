package handlers

import (
	"context"
	"fmt"
	"log/slog"
	"time"

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
//   - 滑动窗口统计（stats:window）
//   - 累计统计（stats:accum）
//   - 分布统计（stats:dist）
func handleStatisticsAssessmentSubmitted(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data AssessmentSubmittedPayload
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse assessment submitted event: %w", err)
		}

		deps.Logger.Info("processing statistics for assessment submitted",
			slog.String("event_id", env.ID),
			slog.Int64("assessment_id", data.AssessmentID),
			slog.Uint64("testee_id", data.TesteeID),
			slog.String("questionnaire_code", data.QuestionnaireCode),
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

		// 使用全局常量：org_id 固定为 1（单租户场景）
		// 注意：worker 模块无法直接引用 apiserver 包，使用硬编码值
		orgID := int64(1)
		originType := ""

		// TODO: 从事件数据中提取origin_type
		// 当前暂时跳过，后续可以在事件数据中添加OriginType字段

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

		// 2. 更新滑动窗口统计
		windows := []string{"last7d", "last15d", "last30d"}
		for _, window := range windows {
			if err := cache.IncrementWindowCount(ctx, orgID, statistics.StatisticTypeQuestionnaire, data.QuestionnaireCode, window); err != nil {
				deps.Logger.Error("failed to increment window count",
					slog.String("event_id", env.ID),
					slog.String("window", window),
					slog.String("error", err.Error()),
				)
			}
		}

		// 3. 更新累计统计
		if err := cache.IncrementAccumCount(ctx, orgID, statistics.StatisticTypeQuestionnaire, data.QuestionnaireCode, "total_submissions"); err != nil {
			deps.Logger.Error("failed to increment accum count",
				slog.String("event_id", env.ID),
				slog.String("error", err.Error()),
			)
		}

		// 4. 更新来源分布统计
		if originType != "" {
			if err := cache.IncrementDistribution(ctx, orgID, statistics.StatisticTypeQuestionnaire, data.QuestionnaireCode, "origin", originType); err != nil {
				deps.Logger.Error("failed to increment origin distribution",
					slog.String("event_id", env.ID),
					slog.String("error", err.Error()),
				)
			}
		}

		// 5. 更新受试者统计
		if err := cache.IncrementAccumCount(ctx, orgID, statistics.StatisticTypeTestee, fmt.Sprintf("%d", data.TesteeID), "total_assessments"); err != nil {
			deps.Logger.Error("failed to increment testee accum count",
				slog.String("event_id", env.ID),
				slog.String("error", err.Error()),
			)
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
//   - 累计统计（完成数）
//   - 风险分布统计
func handleStatisticsAssessmentInterpreted(deps *Dependencies) HandlerFunc {
	return func(ctx context.Context, eventType string, payload []byte) error {
		var data AssessmentInterpretedPayload
		env, err := ParseEventData(payload, &data)
		if err != nil {
			return fmt.Errorf("failed to parse assessment interpreted event: %w", err)
		}

		deps.Logger.Info("processing statistics for assessment interpreted",
			slog.String("event_id", env.ID),
			slog.Int64("assessment_id", data.AssessmentID),
			slog.Uint64("testee_id", data.TesteeID),
			slog.String("scale_code", data.ScaleCode),
			slog.String("risk_level", data.RiskLevel),
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

		// 使用全局常量：org_id 固定为 1（单租户场景）
		// 注意：worker 模块无法直接引用 apiserver 包，使用硬编码值
		orgID := int64(1)
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

		// 2. 更新累计统计（完成数）
		if questionnaireCode != "" {
			if err := cache.IncrementAccumCount(ctx, orgID, statistics.StatisticTypeQuestionnaire, questionnaireCode, "total_completions"); err != nil {
				deps.Logger.Error("failed to increment accum completion count",
					slog.String("event_id", env.ID),
					slog.String("error", err.Error()),
				)
			}
		}

		// 3. 更新风险分布统计
		if data.RiskLevel != "" {
			// 问卷维度风险分布
			if questionnaireCode != "" {
				if err := cache.IncrementDistribution(ctx, orgID, statistics.StatisticTypeQuestionnaire, questionnaireCode, "risk", data.RiskLevel); err != nil {
					deps.Logger.Error("failed to increment risk distribution",
						slog.String("event_id", env.ID),
						slog.String("error", err.Error()),
					)
				}
			}

			// 受试者维度风险分布
			if err := cache.IncrementDistribution(ctx, orgID, statistics.StatisticTypeTestee, fmt.Sprintf("%d", data.TesteeID), "risk", data.RiskLevel); err != nil {
				deps.Logger.Error("failed to increment testee risk distribution",
					slog.String("event_id", env.ID),
					slog.String("error", err.Error()),
				)
			}
		}

		// 4. 更新受试者统计（完成数）
		if err := cache.IncrementAccumCount(ctx, orgID, statistics.StatisticTypeTestee, fmt.Sprintf("%d", data.TesteeID), "completed_assessments"); err != nil {
			deps.Logger.Error("failed to increment testee completed count",
				slog.String("event_id", env.ID),
				slog.String("error", err.Error()),
			)
		}

		// 标记事件已处理（TTL=7天）
		if err := cache.MarkEventProcessed(ctx, env.ID, 7*24*time.Hour); err != nil {
			deps.Logger.Error("failed to mark event as processed",
				slog.String("event_id", env.ID),
				slog.String("error", err.Error()),
			)
		}

		deps.Logger.Info("statistics updated for assessment interpreted",
			slog.String("event_id", env.ID),
			slog.Int64("assessment_id", data.AssessmentID),
		)

		return nil
	}
}
