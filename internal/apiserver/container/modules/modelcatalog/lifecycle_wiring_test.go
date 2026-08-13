package modelcatalog

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/lifecycle"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type modelCacheEffectRecorder struct {
	steps    []string
	kind     string
	code     string
	action   string
	generic  int
	scale    int
	typology int
}

func (r *modelCacheEffectRecorder) InvalidatePublishedModel(context.Context, domain.Kind, string) {
	r.steps = append(r.steps, "invalidate_l2")
}

func (r *modelCacheEffectRecorder) NotifyAssessmentModelCacheChanged(_ context.Context, kind, code, action string) {
	r.steps = append(r.steps, "notify_generic")
	r.kind, r.code, r.action = kind, code, action
	r.generic++
}

func (r *modelCacheEffectRecorder) NotifyScaleCacheChanged(context.Context, string, string) {
	r.scale++
}

func (r *modelCacheEffectRecorder) NotifyTypologyModelCacheChanged(context.Context, string, string) {
	r.typology++
}

func TestLifecyclePublishesGenericCacheSignalAfterL2InvalidationForEveryKindAndAction(t *testing.T) {
	t.Parallel()

	kinds := []domain.Kind{domain.KindScale, domain.KindTypology, domain.KindBehavioralRating, domain.KindCognitive}
	actions := []lifecycle.Action{lifecycle.ActionPublished, lifecycle.ActionUnpublished, lifecycle.ActionArchived}
	for _, kind := range kinds {
		for _, action := range actions {
			kind, action := kind, action
			t.Run(string(kind)+"/"+string(action), func(t *testing.T) {
				recorder := &modelCacheEffectRecorder{}
				deps := Deps{
					Lifecycle: LifecycleDeps{CacheSignalNotifier: recorder},
					Catalog:   CatalogDeps{CacheInvalidator: recorder, CacheSignalNotifier: recorder},
				}
				lifecycleEffects(deps).AfterTransition(context.Background(), &domain.AssessmentModel{
					Kind: kind, Code: "model-a",
				}, action)

				if recorder.generic != 1 || recorder.kind != string(kind) || recorder.code != "model-a" || recorder.action != string(action) {
					t.Fatalf("generic signal = count:%d kind:%q code:%q action:%q", recorder.generic, recorder.kind, recorder.code, recorder.action)
				}
				if len(recorder.steps) < 2 || recorder.steps[0] != "invalidate_l2" || recorder.steps[1] != "notify_generic" {
					t.Fatalf("cache effect order = %v", recorder.steps)
				}
				if want := kind == domain.KindScale; (recorder.scale == 1) != want {
					t.Fatalf("scale compatibility signal count = %d", recorder.scale)
				}
				if want := kind == domain.KindTypology; (recorder.typology == 1) != want {
					t.Fatalf("typology compatibility signal count = %d", recorder.typology)
				}
			})
		}
	}
}
