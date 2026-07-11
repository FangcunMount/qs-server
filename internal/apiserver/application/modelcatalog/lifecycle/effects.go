package lifecycle

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// Action identifies a committed catalogue lifecycle transition.
type Action string

const (
	ActionPublished   Action = "publish"
	ActionUnpublished Action = "unpublish"
	ActionArchived    Action = "archive"
)

// Effect contains best-effort work that is performed only after the
// aggregate and its published head have been persisted successfully.
type Effect interface {
	Supports(domain.Identity) bool
	AfterTransition(context.Context, *domain.AssessmentModel, Action)
}

// EffectsRegistry resolves effects by model identity. Definition
// strategy resolution and lifecycle side effects deliberately remain separate.
type EffectsRegistry struct {
	effects []Effect
}

func NewEffectsRegistry(effects ...Effect) EffectsRegistry {
	result := EffectsRegistry{effects: make([]Effect, 0, len(effects))}
	for _, effect := range effects {
		if effect != nil {
			result.effects = append(result.effects, effect)
		}
	}
	return result
}

func (r EffectsRegistry) AfterTransition(ctx context.Context, model *domain.AssessmentModel, action Action) {
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

// EffectFunc is an adapter for composition-root effects.
type EffectFunc struct {
	Match func(domain.Identity) bool
	Run   func(context.Context, *domain.AssessmentModel, Action)
}

func (f EffectFunc) Supports(identity domain.Identity) bool {
	return f.Match != nil && f.Match(identity)
}

func (f EffectFunc) AfterTransition(ctx context.Context, model *domain.AssessmentModel, action Action) {
	if f.Run != nil {
		f.Run(ctx, model, action)
	}
}
