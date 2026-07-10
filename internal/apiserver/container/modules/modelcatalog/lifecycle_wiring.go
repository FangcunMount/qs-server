package modelcatalog

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/logger"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	assessmentModelApp "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventpayload"
	"github.com/FangcunMount/qs-server/pkg/event"
)

func questionnaireBindingPolicies(deps Deps) assessmentModelApp.QuestionnaireBindingPolicies {
	return assessmentModelApp.NewQuestionnaireBindingPolicies(
		assessmentModelApp.ScaleQuestionnaireBindingPolicy{
			Models:         deps.Scoring.ModelRepo,
			Questionnaires: deps.Scoring.QuestionnaireCatalog,
			PublishQuestionnaire: func(ctx context.Context, code string) (string, error) {
				if deps.Scoring.QuestionnairePublisher == nil {
					return "", nil
				}
				result, err := deps.Scoring.QuestionnairePublisher.Publish(ctx, code)
				if err != nil || result == nil {
					return "", err
				}
				return result.Version, nil
			},
		},
		assessmentModelApp.TypologyQuestionnaireBindingPolicy{Questionnaires: deps.Typology.QuestionnaireQuery},
	)
}

func lifecycleEffects(deps Deps) assessmentModelApp.LifecycleEffectsRegistry {
	scaleEffect := assessmentModelApp.LifecycleEffectFunc{
		Match: func(identity domain.Identity) bool { return identity.Kind == domain.KindScale },
		Run: func(ctx context.Context, model *domain.AssessmentModel, action assessmentModelApp.LifecycleAction) {
			publishScaleLifecycleEffect(ctx, deps, model, action)
		},
	}
	typologyEffect := assessmentModelApp.LifecycleEffectFunc{
		Match: func(identity domain.Identity) bool { return identity.Kind == domain.KindTypology },
		Run: func(ctx context.Context, model *domain.AssessmentModel, action assessmentModelApp.LifecycleAction) {
			if deps.Typology.CacheSignalNotifier != nil && model != nil {
				deps.Typology.CacheSignalNotifier.NotifyTypologyModelCacheChanged(ctx, model.Code, string(action))
			}
		},
	}
	return assessmentModelApp.NewLifecycleEffectsRegistry(scaleEffect, typologyEffect)
}

func publishScaleLifecycleEffect(ctx context.Context, deps Deps, model *domain.AssessmentModel, action assessmentModelApp.LifecycleAction) {
	if model == nil {
		return
	}
	if deps.Scoring.EventPublisher != nil {
		if changeAction, ok := scaleChangeAction(action); ok {
			evt := event.New(eventcatalog.ScaleChanged, "MedicalScale", "0", eventpayload.ScaleChangedData{
				Code:      model.Code,
				Version:   fmt.Sprintf("v%d", model.Revision()),
				Name:      model.Title,
				Action:    changeAction,
				ChangedAt: time.Now().UTC(),
			})
			eventing.PublishCollectedEvents(ctx, deps.Scoring.EventPublisher, eventing.Collect(evt), nil, nil)
		}
	}
	if deps.Scoring.ListCache != nil {
		if err := deps.Scoring.ListCache.Rebuild(ctx); err != nil {
			logger.L(ctx).Errorw("rebuild scale list cache after lifecycle transition", "code", model.Code, "action", action, "error", err)
		}
	}
	if deps.Scoring.CacheSignalNotifier != nil {
		deps.Scoring.CacheSignalNotifier.NotifyScaleCacheChanged(ctx, model.Code, string(action))
	}
}

func scaleChangeAction(action assessmentModelApp.LifecycleAction) (eventpayload.ScaleChangeAction, bool) {
	switch action {
	case assessmentModelApp.LifecycleActionPublished:
		return eventpayload.ScaleChangeActionPublished, true
	case assessmentModelApp.LifecycleActionUnpublished:
		return eventpayload.ScaleChangeActionUnpublished, true
	case assessmentModelApp.LifecycleActionArchived:
		return eventpayload.ScaleChangeActionArchived, true
	default:
		return "", false
	}
}
