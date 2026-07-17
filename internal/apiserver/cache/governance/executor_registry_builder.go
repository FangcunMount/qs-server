package cachegovernance

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/target"
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
	return registry
}
