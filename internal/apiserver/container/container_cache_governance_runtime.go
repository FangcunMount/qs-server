package container

import (
	"context"
	"fmt"
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	scaleCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
)

// WarmupCache 预热缓存（异步执行，不阻塞）
func (c *Container) WarmupCache(ctx context.Context) error {
	if !c.initialized {
		return fmt.Errorf("container not initialized")
	}
	if c.WarmupCoordinator != nil {
		if err := c.WarmupCoordinator.WarmStartup(ctx); err != nil {
			return fmt.Errorf("cache governance startup warmup failed: %w", err)
		}
		return nil
	}
	return fmt.Errorf("cache governance warmup coordinator is unavailable")
}

func (c *Container) listPublishedScaleCodes(ctx context.Context) ([]string, error) {
	if c == nil || c.ScaleModule == nil || c.ScaleModule.Repo == nil {
		return nil, nil
	}
	const pageSize = 200
	page := 1
	codes := make([]string, 0)
	for {
		items, err := c.ScaleModule.Repo.FindSummaryList(ctx, page, pageSize, map[string]interface{}{
			"status": scale.StatusPublished.Value(),
		})
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			break
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			codes = append(codes, item.GetCode().String())
		}
		if len(items) < pageSize {
			break
		}
		page++
	}
	return codes, nil
}

func (c *Container) listPublishedQuestionnaireCodes(ctx context.Context) ([]string, error) {
	if c == nil || c.SurveyModule == nil || c.SurveyModule.Questionnaire == nil || c.SurveyModule.Questionnaire.Repo == nil {
		return nil, nil
	}
	const pageSize = 200
	page := 1
	codes := make([]string, 0)
	for {
		items, err := c.SurveyModule.Questionnaire.Repo.FindBasePublishedList(ctx, page, pageSize, map[string]interface{}{
			"status": "published",
		})
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			break
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			codes = append(codes, item.GetCode().String())
		}
		if len(items) < pageSize {
			break
		}
		page++
	}
	return codes, nil
}

func (c *Container) lookupScaleQuestionnaireCode(ctx context.Context, code string) (string, error) {
	if c == nil || c.ScaleModule == nil || c.ScaleModule.Repo == nil {
		return "", nil
	}
	item, err := c.ScaleModule.Repo.FindByCode(ctx, code)
	if err != nil || item == nil {
		return "", err
	}
	return item.GetQuestionnaireCode().String(), nil
}

func (c *Container) warmScaleCacheTarget(ctx context.Context, code string) error {
	if c == nil || c.ScaleModule == nil || c.ScaleModule.Repo == nil || strings.TrimSpace(code) == "" {
		return nil
	}
	if cachedRepo, ok := c.ScaleModule.Repo.(*scaleCache.CachedScaleRepository); ok {
		return cachedRepo.WarmupCache(ctx, []string{code})
	}
	_, err := c.ScaleModule.Repo.FindByCode(ctx, code)
	return err
}

func (c *Container) warmQuestionnaireCacheTarget(ctx context.Context, code string) error {
	if c == nil || c.SurveyModule == nil || c.SurveyModule.Questionnaire == nil || c.SurveyModule.Questionnaire.Repo == nil || strings.TrimSpace(code) == "" {
		return nil
	}
	if cachedRepo, ok := c.SurveyModule.Questionnaire.Repo.(*scaleCache.CachedQuestionnaireRepository); ok {
		return cachedRepo.WarmupCache(ctx, []string{code})
	}
	_, err := c.SurveyModule.Questionnaire.Repo.FindBaseByCode(ctx, code)
	return err
}

func (c *Container) warmScaleListTarget(ctx context.Context) error {
	if c == nil || c.ScaleModule == nil || c.ScaleModule.ListCache == nil {
		return nil
	}
	return c.ScaleModule.ListCache.Rebuild(ctx)
}

func (c *Container) warmSystemStatsTarget(ctx context.Context, orgID int64) error {
	if c == nil || c.StatisticsModule == nil || c.StatisticsModule.SystemStatisticsService == nil {
		return nil
	}
	_, err := c.StatisticsModule.SystemStatisticsService.GetSystemStatistics(ctx, orgID)
	return err
}

func (c *Container) warmQuestionnaireStatsTarget(ctx context.Context, orgID int64, code string) error {
	if c == nil || c.StatisticsModule == nil || c.StatisticsModule.QuestionnaireStatisticsService == nil {
		return nil
	}
	_, err := c.StatisticsModule.QuestionnaireStatisticsService.GetQuestionnaireStatistics(ctx, orgID, code)
	return err
}

func (c *Container) warmPlanStatsTarget(ctx context.Context, orgID int64, planID uint64) error {
	if c == nil || c.StatisticsModule == nil || c.StatisticsModule.PlanStatisticsService == nil {
		return nil
	}
	_, err := c.StatisticsModule.PlanStatisticsService.GetPlanStatistics(ctx, orgID, planID)
	return err
}
