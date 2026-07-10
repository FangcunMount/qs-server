package modelcatalog

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	assessmentModelApp "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	quesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/questionnairecatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventpayload"
	"github.com/FangcunMount/qs-server/pkg/event"
)

// ScaleCacheSignalNotifier publishes best-effort invalidation notices after a
// successful scale lifecycle transition.
type ScaleCacheSignalNotifier interface {
	NotifyScaleCacheChanged(context.Context, string, string)
}

// LifecycleDeps holds catalog-management collaborators. It deliberately has
// no family command service or legacy list-cache dependency.
type LifecycleDeps struct {
	QuestionnaireCatalog   questionnairecatalog.Catalog
	QuestionnairePublisher quesApp.QuestionnaireLifecycleService
	EventPublisher         event.EventPublisher
	CacheSignalNotifier    ScaleCacheSignalNotifier
}

func questionnaireBindingPolicies(deps Deps) assessmentModelApp.QuestionnaireBindingPolicies {
	return assessmentModelApp.NewQuestionnaireBindingPolicies(
		assessmentModelApp.ScaleQuestionnaireBindingPolicy{
			Models:         deps.Typology.ModelRepo,
			Questionnaires: deps.Lifecycle.QuestionnaireCatalog,
			PublishQuestionnaire: func(ctx context.Context, code string) (string, error) {
				if deps.Lifecycle.QuestionnairePublisher == nil {
					return "", nil
				}
				result, err := deps.Lifecycle.QuestionnairePublisher.Publish(ctx, code)
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
	if deps.Lifecycle.EventPublisher != nil {
		if changeAction, ok := scaleChangeAction(action); ok {
			evt := event.New(eventcatalog.ScaleChanged, "MedicalScale", "0", eventpayload.ScaleChangedData{
				Code:      model.Code,
				Version:   fmt.Sprintf("v%d", model.Revision()),
				Name:      model.Title,
				Action:    changeAction,
				ChangedAt: time.Now().UTC(),
			})
			eventing.PublishCollectedEvents(ctx, deps.Lifecycle.EventPublisher, eventing.Collect(evt), nil, nil)
		}
	}
	if deps.Lifecycle.CacheSignalNotifier != nil {
		deps.Lifecycle.CacheSignalNotifier.NotifyScaleCacheChanged(ctx, model.Code, string(action))
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
