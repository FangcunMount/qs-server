package modelcatalog

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// LifecycleAction identifies a committed catalogue lifecycle transition.
type LifecycleAction string

const (
	LifecycleActionPublished   LifecycleAction = "publish"
	LifecycleActionUnpublished LifecycleAction = "unpublish"
	LifecycleActionArchived    LifecycleAction = "archive"
)

// LifecycleEffect contains best-effort work that is performed only after the
// aggregate and its published head have been persisted successfully.
type LifecycleEffect interface {
	Supports(domain.Identity) bool
	AfterTransition(context.Context, *domain.AssessmentModel, LifecycleAction)
}

// LifecycleEffectsRegistry resolves effects by model identity. Definition
// strategy resolution and lifecycle side effects deliberately remain separate.
type LifecycleEffectsRegistry struct {
	effects []LifecycleEffect
}

func NewLifecycleEffectsRegistry(effects ...LifecycleEffect) LifecycleEffectsRegistry {
	result := LifecycleEffectsRegistry{effects: make([]LifecycleEffect, 0, len(effects))}
	for _, effect := range effects {
		if effect != nil {
			result.effects = append(result.effects, effect)
		}
	}
	return result
}

func (r LifecycleEffectsRegistry) AfterTransition(ctx context.Context, model *domain.AssessmentModel, action LifecycleAction) {
	if model == nil {
		return
	}
	identity := domain.Identity{Kind: model.Kind, SubKind: model.SubKind, Algorithm: model.Algorithm}
	for _, effect := range r.effects {
		if effect.Supports(identity) {
			effect.AfterTransition(ctx, model, action)
		}
	}
}

// LifecycleEffectFunc is an adapter for composition-root effects.
type LifecycleEffectFunc struct {
	Match func(domain.Identity) bool
	Run   func(context.Context, *domain.AssessmentModel, LifecycleAction)
}

func (f LifecycleEffectFunc) Supports(identity domain.Identity) bool {
	return f.Match != nil && f.Match(identity)
}

func (f LifecycleEffectFunc) AfterTransition(ctx context.Context, model *domain.AssessmentModel, action LifecycleAction) {
	if f.Run != nil {
		f.Run(ctx, model, action)
	}
}
