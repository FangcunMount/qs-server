package cachegovernance

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/cachetarget"
)

// ExecutorRegistryBuilder 注册 cache governance 预热执行器。
type ExecutorRegistryBuilder struct{}

func (ExecutorRegistryBuilder) Build(deps Dependencies) *WarmupRegistry {
	registry := NewWarmupRegistry()
	registry.Register(cachetarget.WarmupKindStaticScale, func(ctx context.Context, target cachetarget.WarmupTarget) error {
		if deps.WarmScale == nil {
			return nil
		}
		code, ok := cachetarget.ParseStaticScaleScope(target.Scope)
		if !ok {
			return fmt.Errorf("invalid static scale warmup scope: %s", target.Scope)
		}
		return deps.WarmScale(ctx, code)
	})
	registry.Register(cachetarget.WarmupKindStaticQuestionnaire, func(ctx context.Context, target cachetarget.WarmupTarget) error {
		if deps.WarmQuestionnaire == nil {
			return nil
		}
		code, ok := cachetarget.ParseStaticQuestionnaireScope(target.Scope)
		if !ok {
			return fmt.Errorf("invalid static questionnaire warmup scope: %s", target.Scope)
		}
		return deps.WarmQuestionnaire(ctx, code)
	})
	registry.Register(cachetarget.WarmupKindStaticScaleList, func(ctx context.Context, _ cachetarget.WarmupTarget) error {
		if deps.WarmScaleList == nil {
			return nil
		}
		return deps.WarmScaleList(ctx)
	})
	registry.Register(cachetarget.WarmupKindStaticTypologyModel, func(ctx context.Context, target cachetarget.WarmupTarget) error {
		if deps.WarmPublishedTypologyModel == nil {
			return nil
		}
		code, ok := cachetarget.ParseStaticTypologyModelScope(target.Scope)
		if !ok {
			return fmt.Errorf("invalid static typology model warmup scope: %s", target.Scope)
		}
		return deps.WarmPublishedTypologyModel(ctx, code)
	})
	registry.Register(cachetarget.WarmupKindQueryStatsOverview, func(ctx context.Context, target cachetarget.WarmupTarget) error {
		if deps.WarmStatsOverview == nil {
			return nil
		}
		orgID, preset, ok := cachetarget.ParseQueryStatsOverviewScope(target.Scope)
		if !ok {
			return fmt.Errorf("invalid stats overview warmup scope: %s", target.Scope)
		}
		return deps.WarmStatsOverview(ctx, orgID, preset)
	})
	registry.Register(cachetarget.WarmupKindQueryStatsSystem, func(ctx context.Context, target cachetarget.WarmupTarget) error {
		if deps.WarmStatsSystem == nil {
			return nil
		}
		orgID, ok := cachetarget.ParseQueryStatsSystemScope(target.Scope)
		if !ok {
			return fmt.Errorf("invalid stats system warmup scope: %s", target.Scope)
		}
		return deps.WarmStatsSystem(ctx, orgID)
	})
	registry.Register(cachetarget.WarmupKindQueryStatsQuestionnaire, func(ctx context.Context, target cachetarget.WarmupTarget) error {
		if deps.WarmStatsQuestionnaire == nil {
			return nil
		}
		orgID, code, ok := cachetarget.ParseQueryStatsQuestionnaireScope(target.Scope)
		if !ok {
			return fmt.Errorf("invalid stats questionnaire warmup scope: %s", target.Scope)
		}
		return deps.WarmStatsQuestionnaire(ctx, orgID, code)
	})
	registry.Register(cachetarget.WarmupKindQueryStatsPlan, func(ctx context.Context, target cachetarget.WarmupTarget) error {
		if deps.WarmStatsPlan == nil {
			return nil
		}
		orgID, planID, ok := cachetarget.ParseQueryStatsPlanScope(target.Scope)
		if !ok {
			return fmt.Errorf("invalid stats plan warmup scope: %s", target.Scope)
		}
		return deps.WarmStatsPlan(ctx, orgID, planID)
	})
	return registry
}
