package statistics

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	statisticsInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/statistics"
	statisticsCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/statistics"
	"gorm.io/gorm"
)

// planStatisticsService 计划统计服务实现
type planStatisticsService struct {
	db         *gorm.DB
	repo       *statisticsInfra.StatisticsRepository
	cache      *statisticsCache.StatisticsCache
	aggregator *statistics.Aggregator
}

// NewPlanStatisticsService 创建计划统计服务
func NewPlanStatisticsService(
	db *gorm.DB,
	repo *statisticsInfra.StatisticsRepository,
	cache *statisticsCache.StatisticsCache,
) PlanStatisticsService {
	return &planStatisticsService{
		db:         db,
		repo:       repo,
		cache:      cache,
		aggregator: statistics.NewAggregator(),
	}
}

// GetPlanStatistics 获取计划统计
func (s *planStatisticsService) GetPlanStatistics(
	ctx context.Context,
	orgID int64,
	planID uint64,
) (*statistics.PlanStatistics, error) {
	l := logger.L(ctx)
	l.Infow("获取计划统计", "org_id", orgID, "plan_id", planID)

	// 1. 优先从Redis缓存查询
	if s.cache != nil {
		cacheKey := fmt.Sprintf("plan:%d:%d", orgID, planID)
		cached, err := s.cache.GetQueryCache(ctx, cacheKey)
		if err == nil && cached != "" {
			var stats statistics.PlanStatistics
			if err := json.Unmarshal([]byte(cached), &stats); err == nil {
				l.Debugw("从Redis缓存获取计划统计", "cache_key", cacheKey)
				return &stats, nil
			}
		}
	}

	// 2. 其次从MySQL统计表查询
	if s.repo != nil {
		po, err := s.repo.GetPlanStatistics(ctx, orgID, planID)
		if err == nil && po != nil {
			stats := s.convertPlanPOToPlanStatistics(po)

			// 缓存结果（TTL=5分钟）
			if s.cache != nil {
				if data, err := json.Marshal(stats); err == nil {
					cacheKey := fmt.Sprintf("plan:%d:%d", orgID, planID)
					s.cache.SetQueryCache(ctx, cacheKey, string(data), 5*time.Minute)
				}
			}

			l.Debugw("从MySQL统计表获取计划统计")
			return stats, nil
		}
	}

	// 3. 最后从原始表实时聚合
	l.Debugw("从原始表实时聚合计划统计")
	result := &statistics.PlanStatistics{
		OrgID:  orgID,
		PlanID: planID,
	}

	// 从 assessment_tasks 表聚合
	var taskStats struct {
		TotalTasks     int64
		CompletedTasks int64
		PendingTasks   int64
		ExpiredTasks   int64
	}

	if err := s.db.WithContext(ctx).
		Table("assessment_task").
		Select(`
			COUNT(*) as total_tasks,
			SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) as completed_tasks,
			SUM(CASE WHEN status IN ('pending', 'opened') THEN 1 ELSE 0 END) as pending_tasks,
			SUM(CASE WHEN status = 'expired' THEN 1 ELSE 0 END) as expired_tasks
		`).
		Where("org_id = ? AND plan_id = ? AND deleted_at IS NULL", orgID, planID).
		Scan(&taskStats).Error; err != nil {
		return nil, err
	}

	result.TotalTasks = taskStats.TotalTasks
	result.CompletedTasks = taskStats.CompletedTasks
	result.PendingTasks = taskStats.PendingTasks
	result.ExpiredTasks = taskStats.ExpiredTasks
	result.CompletionRate = s.aggregator.CalculateCompletionRate(taskStats.TotalTasks, taskStats.CompletedTasks)

	// 受试者统计
	var testeeStats struct {
		EnrolledTestees int64
		ActiveTestees   int64
	}

	if err := s.db.WithContext(ctx).
		Table("assessment_task").
		Select(`
			COUNT(DISTINCT testee_id) as enrolled_testees,
			COUNT(DISTINCT CASE WHEN status = 'completed' THEN testee_id END) as active_testees
		`).
		Where("org_id = ? AND plan_id = ? AND deleted_at IS NULL", orgID, planID).
		Scan(&testeeStats).Error; err != nil {
		return nil, err
	}

	result.EnrolledTestees = testeeStats.EnrolledTestees
	result.ActiveTestees = testeeStats.ActiveTestees

	// 缓存结果（TTL=5分钟）
	if s.cache != nil {
		if data, err := json.Marshal(result); err == nil {
			cacheKey := fmt.Sprintf("plan:%d:%d", orgID, planID)
			s.cache.SetQueryCache(ctx, cacheKey, string(data), 5*time.Minute)
		}
	}

	return result, nil
}

// convertPlanPOToPlanStatistics 转换计划统计PO为计划统计领域对象
func (s *planStatisticsService) convertPlanPOToPlanStatistics(
	po *statisticsInfra.StatisticsPlanPO,
) *statistics.PlanStatistics {
	result := &statistics.PlanStatistics{
		OrgID:           int64(po.OrgID),
		PlanID:          po.PlanID,
		TotalTasks:      po.TotalTasks,
		CompletedTasks:  po.CompletedTasks,
		PendingTasks:    po.PendingTasks,
		ExpiredTasks:    po.ExpiredTasks,
		EnrolledTestees: po.EnrolledTestees,
		ActiveTestees:   po.ActiveTestees,
	}

	result.CompletionRate = s.aggregator.CalculateCompletionRate(po.TotalTasks, po.CompletedTasks)

	return result
}
