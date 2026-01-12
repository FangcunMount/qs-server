package cache

import (
	"context"
	"fmt"

	"github.com/FangcunMount/component-base/pkg/logger"
	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
)

// StatisticsWarmupConfig 统计缓存预热配置
type StatisticsWarmupConfig struct {
	// OrgIDs 需要预热的组织ID列表
	OrgIDs []int64
	// QuestionnaireCodes 需要预热的问卷编码列表（可选）
	QuestionnaireCodes []string
	// PlanIDs 需要预热的计划ID列表（可选）
	PlanIDs []uint64
}

// WarmupService 缓存预热服务
type WarmupService struct {
	scaleRepo         scale.Repository
	questionnaireRepo questionnaire.Repository
}

// NewWarmupService 创建缓存预热服务
func NewWarmupService(scaleRepo scale.Repository) *WarmupService {
	return &WarmupService{
		scaleRepo: scaleRepo,
	}
}

// NewWarmupServiceWithQuestionnaire 创建包含问卷的缓存预热服务
func NewWarmupServiceWithQuestionnaire(scaleRepo scale.Repository, questionnaireRepo questionnaire.Repository) *WarmupService {
	return &WarmupService{
		scaleRepo:         scaleRepo,
		questionnaireRepo: questionnaireRepo,
	}
}

// WarmupScaleCache 预热量表缓存
// hotScaleCodes: 热点量表编码列表（如 ["SDS", "SAS", "Conners"]）
func (s *WarmupService) WarmupScaleCache(ctx context.Context, hotScaleCodes []string) error {
	l := logger.L(ctx)
	l.Infow("开始预热量表缓存", "count", len(hotScaleCodes))

	// 检查是否为缓存装饰器
	cachedRepo, ok := s.scaleRepo.(*CachedScaleRepository)
	if !ok {
		l.Debugw("量表 Repository 未使用缓存装饰器，跳过预热")
		return nil
	}

	if err := cachedRepo.WarmupCache(ctx, hotScaleCodes); err != nil {
		return fmt.Errorf("预热量表缓存失败: %w", err)
	}

	l.Infow("量表缓存预热完成", "count", len(hotScaleCodes))
	return nil
}

// WarmupDefaultScales 预热默认热点量表
// 根据业务实际情况配置常用量表编码
func (s *WarmupService) WarmupDefaultScales(ctx context.Context) error {
	// 默认热点量表编码（可根据实际业务调整）
	defaultHotScales := []string{
		"SDS",     // 抑郁自评量表
		"SAS",     // 焦虑自评量表
		"Conners", // Conners 量表
		// 可根据实际使用情况添加更多
	}

	return s.WarmupScaleCache(ctx, defaultHotScales)
}

// WarmupAllPublished 预热所有已发布的量表及其关联的问卷
// 注意：需要底层 Repository 支持 FindSummaryList/FindBaseList
func (s *WarmupService) WarmupAllPublished(ctx context.Context) error {
	l := logger.L(ctx)

	// 预热量表缓存
	publishedScaleCodes, questionnaireCodesFromScales, err := s.listAllPublishedScales(ctx)
	if err != nil {
		return fmt.Errorf("list published scales failed: %w", err)
	}
	if len(publishedScaleCodes) > 0 {
		if err := s.WarmupScaleCache(ctx, publishedScaleCodes); err != nil {
			l.Warnw("预热量表缓存失败", "error", err)
		}
	}

	// 预热问卷缓存：集合了“量表关联问卷”和“已发布问卷”
	qCodes := make(map[string]struct{})
	for _, c := range questionnaireCodesFromScales {
		if c != "" {
			qCodes[c] = struct{}{}
		}
	}
	publishedQuestionnaireCodes, err := s.listAllPublishedQuestionnaires(ctx)
	if err != nil {
		l.Warnw("列出已发布问卷失败，跳过问卷预热", "error", err)
	} else {
		for _, c := range publishedQuestionnaireCodes {
			if c != "" {
				qCodes[c] = struct{}{}
			}
		}
	}

	if len(qCodes) > 0 {
		hot := make([]string, 0, len(qCodes))
		for code := range qCodes {
			hot = append(hot, code)
		}
		if err := s.WarmupQuestionnaireCache(ctx, hot); err != nil {
			l.Warnw("预热问卷缓存失败", "error", err)
		}
	}

	return nil
}

// listAllPublishedScales 分页获取已发布量表编码及关联问卷编码
func (s *WarmupService) listAllPublishedScales(ctx context.Context) (scaleCodes []string, questionnaireCodes []string, err error) {
	const pageSize = 200
	page := 1
	for {
		items, e := s.scaleRepo.FindSummaryList(ctx, page, pageSize, map[string]interface{}{
			"status": scale.StatusPublished.Value(),
		})
		if e != nil {
			err = e
			return
		}
		if len(items) == 0 {
			break
		}
		for _, it := range items {
			if it == nil {
				continue
			}
			scaleCodes = append(scaleCodes, it.GetCode().String())
			questionnaireCodes = append(questionnaireCodes, it.GetQuestionnaireCode().String())
		}
		if len(items) < pageSize {
			break
		}
		page++
	}
	return
}

// listAllPublishedQuestionnaires 获取已发布问卷编码
func (s *WarmupService) listAllPublishedQuestionnaires(ctx context.Context) ([]string, error) {
	if s.questionnaireRepo == nil {
		return nil, nil
	}

	const pageSize = 200
	page := 1
	var codes []string
	for {
		items, err := s.questionnaireRepo.FindBaseList(ctx, page, pageSize, map[string]interface{}{
			"status": questionnaire.STATUS_PUBLISHED.String(),
		})
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			break
		}
		for _, it := range items {
			if it != nil {
				codes = append(codes, it.GetCode().String())
			}
		}
		if len(items) < pageSize {
			break
		}
		page++
	}
	return codes, nil
}

// WarmupQuestionnaireCache 预热问卷缓存
// hotQuestionnaireCodes: 热点问卷编码列表
func (s *WarmupService) WarmupQuestionnaireCache(ctx context.Context, hotQuestionnaireCodes []string) error {
	if s.questionnaireRepo == nil {
		return nil // 未提供问卷 Repository，跳过
	}

	l := logger.L(ctx)
	l.Infow("开始预热问卷缓存", "count", len(hotQuestionnaireCodes))

	// 检查是否为缓存装饰器
	cachedRepo, ok := s.questionnaireRepo.(*CachedQuestionnaireRepository)
	if !ok {
		l.Debugw("问卷 Repository 未使用缓存装饰器，跳过预热")
		return nil
	}

	if err := cachedRepo.WarmupCache(ctx, hotQuestionnaireCodes); err != nil {
		return fmt.Errorf("预热问卷缓存失败: %w", err)
	}

	l.Infow("问卷缓存预热完成", "count", len(hotQuestionnaireCodes))
	return nil
}

// WarmupStatisticsCache 预热统计查询结果缓存
// 注意：统计查询结果缓存 TTL 较短（5分钟），预热主要用于减少首次查询延迟
// 建议：只在有明确需求时使用（如已知活跃组织、常用问卷等）
func WarmupStatisticsCache(
	ctx context.Context,
	config StatisticsWarmupConfig,
	systemService statisticsApp.SystemStatisticsService,
	questionnaireService statisticsApp.QuestionnaireStatisticsService,
	planService statisticsApp.PlanStatisticsService,
) error {
	l := logger.L(ctx)
	l.Infow("开始预热统计查询结果缓存", "org_count", len(config.OrgIDs))

	// 预热系统统计（所有组织）
	for _, orgID := range config.OrgIDs {
		if _, err := systemService.GetSystemStatistics(ctx, orgID); err != nil {
			l.Warnw("预热系统统计失败", "org_id", orgID, "error", err)
			// 继续处理其他组织，不中断
		}
	}

	// 预热问卷统计（如果配置了问卷编码）
	if len(config.QuestionnaireCodes) > 0 {
		for _, orgID := range config.OrgIDs {
			for _, code := range config.QuestionnaireCodes {
				if _, err := questionnaireService.GetQuestionnaireStatistics(ctx, orgID, code); err != nil {
					l.Warnw("预热问卷统计失败", "org_id", orgID, "questionnaire_code", code, "error", err)
					// 继续处理，不中断
				}
			}
		}
	}

	// 预热计划统计（如果配置了计划ID）
	if len(config.PlanIDs) > 0 {
		for _, orgID := range config.OrgIDs {
			for _, planID := range config.PlanIDs {
				if _, err := planService.GetPlanStatistics(ctx, orgID, planID); err != nil {
					l.Warnw("预热计划统计失败", "org_id", orgID, "plan_id", planID, "error", err)
					// 继续处理，不中断
				}
			}
		}
	}

	l.Infow("统计查询结果缓存预热完成")
	return nil
}
