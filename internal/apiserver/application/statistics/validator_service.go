package statistics

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	statisticsInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics"
	statisticsCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/statistics"
)

// validatorService 统计校验服务实现
type validatorService struct {
	repo  *statisticsInfra.StatisticsRepository
	cache *statisticsCache.StatisticsCache
}

// NewValidatorService 创建统计校验服务
func NewValidatorService(
	repo *statisticsInfra.StatisticsRepository,
	cache *statisticsCache.StatisticsCache,
) StatisticsValidatorService {
	return &validatorService{
		repo:  repo,
		cache: cache,
	}
}

// ValidateConsistency 校验数据一致性（Redis vs MySQL）
func (v *validatorService) ValidateConsistency(ctx context.Context, orgID int64) error {
	l := logger.L(ctx)
	l.Infow("开始校验数据一致性", "action", "validate_consistency", "org_id", orgID)

	if orgID <= 0 {
		l.Warnw("无效的 org_id，跳过一致性校验", "org_id", orgID)
		return nil
	}

	keys, err := v.cache.ScanDailyKeys(ctx, orgID, statistics.StatisticTypeQuestionnaire)
	if err != nil {
		l.Errorw("扫描统计键失败",
			"org_id", orgID,
			"stat_type", statistics.StatisticTypeQuestionnaire,
			"error", err.Error(),
		)
		return err
	}

	aggregates := make(map[string]*statisticsInfra.StatisticsAccumulatedPO)
	todayStart, _ := currentDayBounds(time.Now())
	last7d := todayStart.AddDate(0, 0, -7)
	last15d := todayStart.AddDate(0, 0, -15)
	last30d := todayStart.AddDate(0, 0, -30)

	for _, key := range keys {
		parts := parseDailyKey(key)
		if len(parts) != 6 {
			l.Warnw("每日统计键格式不正确，跳过", "key", key)
			continue
		}

		statKey := parts[4]
		date, err := time.ParseInLocation("2006-01-02", parts[5], time.Local)
		if err != nil {
			l.Warnw("日期格式不正确，跳过",
				"key", key,
				"date", parts[5],
				"error", err.Error(),
			)
			continue
		}

		submissionCount, completionCount, err := v.cache.GetDailyCount(ctx, orgID, statistics.StatisticTypeQuestionnaire, statKey, date)
		if err != nil {
			l.Warnw("读取Redis每日统计失败",
				"org_id", orgID,
				"stat_type", statistics.StatisticTypeQuestionnaire,
				"stat_key", statKey,
				"date", parts[5],
				"error", err.Error(),
			)
			continue
		}

		aggregate := aggregates[statKey]
		if aggregate == nil {
			aggregate = &statisticsInfra.StatisticsAccumulatedPO{
				OrgID:         orgID,
				StatisticType: string(statistics.StatisticTypeQuestionnaire),
				StatisticKey:  statKey,
			}
			aggregates[statKey] = aggregate
		}

		aggregate.TotalSubmissions += submissionCount
		aggregate.TotalCompletions += completionCount
		if !date.Before(last7d) {
			aggregate.Last7dSubmissions += submissionCount
		}
		if !date.Before(last15d) {
			aggregate.Last15dSubmissions += submissionCount
		}
		if !date.Before(last30d) {
			aggregate.Last30dSubmissions += submissionCount
		}
	}

	for statKey, aggregate := range aggregates {
		mysqlPO, err := v.repo.GetAccumulatedStatistics(ctx, orgID, statistics.StatisticTypeQuestionnaire, statKey)
		if err != nil {
			l.Warnw("读取MySQL统计失败",
				"org_id", orgID,
				"stat_type", statistics.StatisticTypeQuestionnaire,
				"stat_key", statKey,
				"error", err.Error(),
			)
			continue
		}

		if mysqlPO != nil {
			aggregate.Distribution = mysqlPO.Distribution
			aggregate.FirstOccurredAt = mysqlPO.FirstOccurredAt
			aggregate.LastOccurredAt = mysqlPO.LastOccurredAt
		}

		needsRepair := mysqlPO == nil ||
			mysqlPO.TotalSubmissions != aggregate.TotalSubmissions ||
			mysqlPO.TotalCompletions != aggregate.TotalCompletions ||
			mysqlPO.Last7dSubmissions != aggregate.Last7dSubmissions ||
			mysqlPO.Last15dSubmissions != aggregate.Last15dSubmissions ||
			mysqlPO.Last30dSubmissions != aggregate.Last30dSubmissions

		if !needsRepair {
			continue
		}

		l.Warnw("数据不一致，需要修复",
			"org_id", orgID,
			"stat_type", statistics.StatisticTypeQuestionnaire,
			"stat_key", statKey,
			"redis_total", aggregate.TotalSubmissions,
			"mysql_total", func() int64 {
				if mysqlPO == nil {
					return 0
				}
				return mysqlPO.TotalSubmissions
			}(),
		)

		if err := v.repo.UpsertAccumulatedStatistics(ctx, aggregate); err != nil {
			l.Errorw("修复MySQL统计失败",
				"org_id", orgID,
				"stat_type", statistics.StatisticTypeQuestionnaire,
				"stat_key", statKey,
				"error", err.Error(),
			)
			continue
		}

		l.Infow("已修复MySQL统计",
			"org_id", orgID,
			"stat_type", statistics.StatisticTypeQuestionnaire,
			"stat_key", statKey,
		)
	}

	l.Infow("数据一致性校验完成", "action", "validate_consistency", "org_id", orgID)
	return nil
}
