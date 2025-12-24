package statistics

import (
	"context"

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
func (v *validatorService) ValidateConsistency(ctx context.Context) error {
	l := logger.L(ctx)
	l.Infow("开始校验数据一致性", "action", "validate_consistency")

	// 数据一致性校验
	// 1. 从Redis读取统计
	// 2. 从MySQL读取统计
	// 3. 对比差异
	// 4. 修复不一致（以Redis为准）
	// 使用全局常量：org_id 固定为 1（单租户场景）
	orgIDs := []int64{DefaultOrgID}

	statTypes := []statistics.StatisticType{
		statistics.StatisticTypeQuestionnaire,
		statistics.StatisticTypeTestee,
	}

	for _, orgID := range orgIDs {
		for _, statType := range statTypes {
			// 扫描统计键
			keys, err := v.cache.ScanDailyKeys(ctx, orgID, statType)
			if err != nil {
				l.Errorw("扫描统计键失败",
					"org_id", orgID,
					"stat_type", statType,
					"error", err.Error(),
				)
				continue
			}

			// 提取唯一的statKey
			statKeys := make(map[string]bool)
			for _, key := range keys {
				parts := parseDailyKey(key)
				if len(parts) == 6 {
					statKeys[parts[4]] = true
				}
			}

			// 对每个statKey进行校验
			for statKey := range statKeys {
				// 从Redis读取累计统计
				redisTotal, err := v.cache.GetAccumCount(ctx, orgID, statType, statKey, "total_submissions")
				if err != nil {
					l.Warnw("读取Redis统计失败",
						"org_id", orgID,
						"stat_type", statType,
						"stat_key", statKey,
						"error", err.Error(),
					)
					continue
				}

				// 从MySQL读取累计统计
				mysqlPO, err := v.repo.GetAccumulatedStatistics(ctx, orgID, statType, statKey)
				if err != nil {
					l.Warnw("读取MySQL统计失败",
						"org_id", orgID,
						"stat_type", statType,
						"stat_key", statKey,
						"error", err.Error(),
					)
					continue
				}

				// 对比差异
				if mysqlPO == nil {
					l.Warnw("MySQL统计不存在，需要同步",
						"org_id", orgID,
						"stat_type", statType,
						"stat_key", statKey,
					)
					continue
				}

				if redisTotal != mysqlPO.TotalSubmissions {
					l.Warnw("数据不一致，需要修复",
						"org_id", orgID,
						"stat_type", statType,
						"stat_key", statKey,
						"redis_total", redisTotal,
						"mysql_total", mysqlPO.TotalSubmissions,
					)

					// 修复：以Redis为准，更新MySQL
					mysqlPO.TotalSubmissions = redisTotal
					if err := v.repo.UpsertAccumulatedStatistics(ctx, mysqlPO); err != nil {
						l.Errorw("修复MySQL统计失败",
							"org_id", orgID,
							"stat_type", statType,
							"stat_key", statKey,
							"error", err.Error(),
						)
					} else {
						l.Infow("已修复MySQL统计",
							"org_id", orgID,
							"stat_type", statType,
							"stat_key", statKey,
						)
					}
				}
			}
		}
	}

	l.Infow("数据一致性校验完成", "action", "validate_consistency")
	return nil
}
