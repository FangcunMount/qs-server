package container

import (
	"context"
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/cachebootstrap"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	scaleCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/redisplane"
)

type cacheGovernanceAdapter struct {
	container *Container
}

func newCacheGovernanceAdapter(c *Container) cacheGovernanceAdapter {
	return cacheGovernanceAdapter{container: c}
}

func (a cacheGovernanceAdapter) bindings() cachebootstrap.GovernanceBindings {
	c := a.container
	var warmScale func(context.Context, string) error
	var warmQuestionnaire func(context.Context, string) error
	var warmScaleList func(context.Context) error
	if c != nil && c.CacheClient(redisplane.FamilyStatic) != nil {
		warmScale = a.warmScaleCacheTarget
		warmQuestionnaire = a.warmQuestionnaireCacheTarget
		warmScaleList = a.warmScaleListTarget
	}

	var warmStatsSystem func(context.Context, int64) error
	var warmStatsQuestionnaire func(context.Context, int64, string) error
	var warmStatsPlan func(context.Context, int64, uint64) error
	if c != nil && c.CacheClient(redisplane.FamilyQuery) != nil && !c.cacheOptions.DisableStatisticsCache {
		warmStatsSystem = a.warmSystemStatsTarget
		warmStatsQuestionnaire = a.warmQuestionnaireStatsTarget
		warmStatsPlan = a.warmPlanStatsTarget
	}

	return cachebootstrap.GovernanceBindings{
		ListPublishedScaleCodes:         a.listPublishedScaleCodes,
		ListPublishedQuestionnaireCodes: a.listPublishedQuestionnaireCodes,
		LookupScaleQuestionnaireCode:    a.lookupScaleQuestionnaireCode,
		WarmScale:                       warmScale,
		WarmQuestionnaire:               warmQuestionnaire,
		WarmScaleList:                   warmScaleList,
		WarmStatsSystem:                 warmStatsSystem,
		WarmStatsQuestionnaire:          warmStatsQuestionnaire,
		WarmStatsPlan:                   warmStatsPlan,
	}
}

func (a cacheGovernanceAdapter) listPublishedScaleCodes(ctx context.Context) ([]string, error) {
	c := a.container
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

func (a cacheGovernanceAdapter) listPublishedQuestionnaireCodes(ctx context.Context) ([]string, error) {
	c := a.container
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

func (a cacheGovernanceAdapter) lookupScaleQuestionnaireCode(ctx context.Context, code string) (string, error) {
	c := a.container
	if c == nil || c.ScaleModule == nil || c.ScaleModule.Repo == nil {
		return "", nil
	}
	item, err := c.ScaleModule.Repo.FindByCode(ctx, code)
	if err != nil || item == nil {
		return "", err
	}
	return item.GetQuestionnaireCode().String(), nil
}

func (a cacheGovernanceAdapter) warmScaleCacheTarget(ctx context.Context, code string) error {
	c := a.container
	if c == nil || c.ScaleModule == nil || c.ScaleModule.Repo == nil || strings.TrimSpace(code) == "" {
		return nil
	}
	if cachedRepo, ok := c.ScaleModule.Repo.(*scaleCache.CachedScaleRepository); ok {
		return cachedRepo.WarmupCache(ctx, []string{code})
	}
	_, err := c.ScaleModule.Repo.FindByCode(ctx, code)
	return err
}

func (a cacheGovernanceAdapter) warmQuestionnaireCacheTarget(ctx context.Context, code string) error {
	c := a.container
	if c == nil || c.SurveyModule == nil || c.SurveyModule.Questionnaire == nil || c.SurveyModule.Questionnaire.Repo == nil || strings.TrimSpace(code) == "" {
		return nil
	}
	if cachedRepo, ok := c.SurveyModule.Questionnaire.Repo.(*scaleCache.CachedQuestionnaireRepository); ok {
		return cachedRepo.WarmupCache(ctx, []string{code})
	}
	_, err := c.SurveyModule.Questionnaire.Repo.FindBaseByCode(ctx, code)
	return err
}

func (a cacheGovernanceAdapter) warmScaleListTarget(ctx context.Context) error {
	c := a.container
	if c == nil || c.ScaleModule == nil || c.ScaleModule.ListCache == nil {
		return nil
	}
	return c.ScaleModule.ListCache.Rebuild(ctx)
}

func (a cacheGovernanceAdapter) warmSystemStatsTarget(ctx context.Context, orgID int64) error {
	c := a.container
	if c == nil || c.StatisticsModule == nil || c.StatisticsModule.SystemStatisticsService == nil {
		return nil
	}
	_, err := c.StatisticsModule.SystemStatisticsService.GetSystemStatistics(ctx, orgID)
	return err
}

func (a cacheGovernanceAdapter) warmQuestionnaireStatsTarget(ctx context.Context, orgID int64, code string) error {
	c := a.container
	if c == nil || c.StatisticsModule == nil || c.StatisticsModule.QuestionnaireStatisticsService == nil {
		return nil
	}
	_, err := c.StatisticsModule.QuestionnaireStatisticsService.GetQuestionnaireStatistics(ctx, orgID, code)
	return err
}

func (a cacheGovernanceAdapter) warmPlanStatsTarget(ctx context.Context, orgID int64, planID uint64) error {
	c := a.container
	if c == nil || c.StatisticsModule == nil || c.StatisticsModule.PlanStatisticsService == nil {
		return nil
	}
	_, err := c.StatisticsModule.PlanStatisticsService.GetPlanStatistics(ctx, orgID, planID)
	return err
}
