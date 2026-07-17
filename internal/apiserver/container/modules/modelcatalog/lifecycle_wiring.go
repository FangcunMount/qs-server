package modelcatalog

import (
	"context"
	"fmt"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	appbinding "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/lifecycle"
	quesApp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/questionnairecatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
)

// ScaleCacheSignalNotifier 发布最佳努力无效通知
// 在成功的时间生命周期过渡后发布最佳努力无效通知
type ScaleCacheSignalNotifier interface {
	NotifyScaleCacheChanged(context.Context, string, string)
}

// LifecycleDeps 包含模型目录的管理依赖
// 故意没有家族命令服务或遗留列表缓存依赖
type LifecycleDeps struct {
	QuestionnaireCatalog   questionnairecatalog.Catalog
	QuestionnairePublisher quesApp.QuestionnaireLifecycleService
	EventPublisher         event.EventPublisher
	CacheSignalNotifier    ScaleCacheSignalNotifier
}

// questionnaireBindingPolicies 问卷绑定策略
func questionnaireBindingPolicies(deps Deps) appbinding.Policies {
	return appbinding.NewPolicies(
		appbinding.ScalePolicy{
			Models:         deps.Catalog.ModelRepo,
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
		appbinding.TypologyPolicy{Questionnaires: deps.Catalog.QuestionnaireQuery},
	)
}

// lifecycleEffects 生命周期效果
func lifecycleEffects(deps Deps) lifecycle.EffectsRegistry {
	// 模型生命周期
	modelEffect := lifecycle.EffectFunc{
		Match: func(domain.Identity) bool { return true },
		Run: func(ctx context.Context, model *domain.AssessmentModel, action lifecycle.Action) {
			publishAssessmentModelLifecycleEffect(ctx, deps, model, action)
		},
	}
	// 模型算法
	typologyEffect := lifecycle.EffectFunc{
		Match: func(identity domain.Identity) bool { return identity.Kind == domain.KindTypology },
		Run: func(ctx context.Context, model *domain.AssessmentModel, action lifecycle.Action) {
			if deps.Catalog.CacheSignalNotifier != nil && model != nil {
				deps.Catalog.CacheSignalNotifier.NotifyTypologyModelCacheChanged(ctx, model.Code, string(action))
			}
		},
	}
	// 组合生命周期效果
	return lifecycle.NewEffectsRegistry(modelEffect, typologyEffect)
}

// publishAssessmentModelLifecycleEffect 发布模型生命周期效果
func publishAssessmentModelLifecycleEffect(ctx context.Context, deps Deps, model *domain.AssessmentModel, action lifecycle.Action) {
	if model == nil {
		return
	}
	if deps.Catalog.CacheInvalidator != nil {
		deps.Catalog.CacheInvalidator.InvalidatePublishedModel(ctx, model.Kind, model.Code)
	}
	if deps.Lifecycle.EventPublisher != nil {
		if changeAction, ok := assessmentModelChangeAction(action); ok {
			evt := event.New(eventcatalog.AssessmentModelChanged, "AssessmentModel", model.Code, eventpayload.AssessmentModelChangedData{
				Kind:      string(model.Kind),
				Code:      model.Code,
				Version:   fmt.Sprintf("v%d", model.Revision()),
				Title:     model.Title,
				Action:    changeAction,
				ChangedAt: time.Now().UTC(),
			})
			eventing.PublishCollectedEvents(ctx, deps.Lifecycle.EventPublisher, eventing.Collect(evt), nil, nil)
		}
	}
	if deps.Lifecycle.CacheSignalNotifier != nil && model.Kind == domain.KindScale {
		deps.Lifecycle.CacheSignalNotifier.NotifyScaleCacheChanged(ctx, model.Code, string(action))
	}
}

// assessmentModelChangeAction 模型生命周期动作
func assessmentModelChangeAction(action lifecycle.Action) (eventpayload.AssessmentModelChangeAction, bool) {
	switch action {
	case lifecycle.ActionPublished:
		return eventpayload.AssessmentModelChangeActionPublished, true
	case lifecycle.ActionUnpublished:
		return eventpayload.AssessmentModelChangeActionUnpublished, true
	case lifecycle.ActionArchived:
		return eventpayload.AssessmentModelChangeActionArchived, true
	default:
		return "", false
	}
}
