package container

import (
	"context"
	"strings"

	statisticsApp "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"github.com/FangcunMount/qs-server/internal/apiserver/cachebootstrap"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/scale"
	scaleCache "github.com/FangcunMount/qs-server/internal/apiserver/infra/cache"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/scalereadmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/surveyreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/cacheplane"
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
	if c != nil && c.CacheClient(cacheplane.FamilyStatic) != nil {
		warmScale = a.warmScaleCacheTarget
		warmQuestionnaire = a.warmQuestionnaireCacheTarget
		warmScaleList = a.warmScaleListTarget
	}

	var warmStatsSystem func(context.Context, int64) error
	var warmStatsOverview func(context.Context, int64, string) error
	var warmStatsQuestionnaire func(context.Context, int64, string) error
	var warmStatsPlan func(context.Context, int64, uint64) error
	if c != nil && c.CacheClient(cacheplane.FamilyQuery) != nil && !c.cacheOptions.DisableStatisticsCache {
		warmStatsOverview = a.warmOverviewStatsTarget
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
		WarmStatsOverview:               warmStatsOverview,
		WarmStatsSystem:                 warmStatsSystem,
		WarmStatsQuestionnaire:          warmStatsQuestionnaire,
		WarmStatsPlan:                   warmStatsPlan,
	}
}

func (a cacheGovernanceAdapter) listPublishedScaleCodes(ctx context.Context) ([]string, error) {
	infra := a.containerSurveyScaleInfra()
	if infra == nil || infra.scaleReader == nil {
		return nil, nil
	}
	const pageSize = 200
	page := 1
	codes := make([]string, 0)
	for {
		items, err := infra.scaleReader.ListScales(ctx, scalereadmodel.ScaleFilter{Status: scale.StatusPublished.Value()}, scalereadmodel.PageRequest{Page: page, PageSize: pageSize})
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			break
		}
		for _, item := range items {
			codes = append(codes, item.Code)
		}
		if len(items) < pageSize {
			break
		}
		page++
	}
	return codes, nil
}

func (a cacheGovernanceAdapter) listPublishedQuestionnaireCodes(ctx context.Context) ([]string, error) {
	infra := a.containerSurveyScaleInfra()
	if infra == nil || infra.questionnaireReader == nil {
		return nil, nil
	}
	const pageSize = 200
	page := 1
	codes := make([]string, 0)
	for {
		items, err := infra.questionnaireReader.ListPublishedQuestionnaires(ctx, surveyreadmodel.QuestionnaireFilter{Status: "published"}, surveyreadmodel.PageRequest{Page: page, PageSize: pageSize})
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			break
		}
		for _, item := range items {
			codes = append(codes, item.Code)
		}
		if len(items) < pageSize {
			break
		}
		page++
	}
	return codes, nil
}

func (a cacheGovernanceAdapter) lookupScaleQuestionnaireCode(ctx context.Context, code string) (string, error) {
	infra := a.containerSurveyScaleInfra()
	if infra == nil || infra.scaleRepo == nil {
		return "", nil
	}
	item, err := infra.scaleRepo.FindByCode(ctx, code)
	if err != nil || item == nil {
		return "", err
	}
	return item.GetQuestionnaireCode().String(), nil
}

func (a cacheGovernanceAdapter) warmScaleCacheTarget(ctx context.Context, code string) error {
	infra := a.containerSurveyScaleInfra()
	if infra == nil || infra.scaleRepo == nil || strings.TrimSpace(code) == "" {
		return nil
	}
	if cachedRepo, ok := infra.scaleRepo.(*scaleCache.CachedScaleRepository); ok {
		return cachedRepo.WarmupCache(ctx, []string{code})
	}
	_, err := infra.scaleRepo.FindByCode(ctx, code)
	return err
}

func (a cacheGovernanceAdapter) warmQuestionnaireCacheTarget(ctx context.Context, code string) error {
	infra := a.containerSurveyScaleInfra()
	if infra == nil || infra.questionnaireRepo == nil || strings.TrimSpace(code) == "" {
		return nil
	}
	if cachedRepo, ok := infra.questionnaireRepo.(*scaleCache.CachedQuestionnaireRepository); ok {
		return cachedRepo.WarmupCache(ctx, []string{code})
	}
	_, err := infra.questionnaireRepo.FindBaseByCode(ctx, code)
	return err
}

func (a cacheGovernanceAdapter) warmScaleListTarget(ctx context.Context) error {
	infra := a.containerSurveyScaleInfra()
	if infra == nil || infra.scaleListCache == nil {
		return nil
	}
	return infra.scaleListCache.Rebuild(ctx)
}

func (a cacheGovernanceAdapter) containerSurveyScaleInfra() *surveyScaleInfra {
	if a.container == nil {
		return nil
	}
	return a.container.surveyScaleInfra
}

func (a cacheGovernanceAdapter) warmSystemStatsTarget(ctx context.Context, orgID int64) error {
	c := a.container
	if c == nil || c.StatisticsModule == nil || c.StatisticsModule.SystemStatisticsService == nil {
		return nil
	}
	_, err := c.StatisticsModule.SystemStatisticsService.GetSystemStatistics(ctx, orgID)
	return err
}

func (a cacheGovernanceAdapter) warmOverviewStatsTarget(ctx context.Context, orgID int64, preset string) error {
	c := a.container
	if c == nil || c.StatisticsModule == nil || c.StatisticsModule.ReadService == nil {
		return nil
	}
	_, err := c.StatisticsModule.ReadService.GetOverview(ctx, orgID, statisticsApp.QueryFilter{Preset: preset})
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
