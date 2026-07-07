package modelcatalog

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/behavior"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/behavioral_rating"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/cognitive"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type channelListBehaviorStub struct {
	behaviorCommandStub
}

func (s *channelListBehaviorStub) List(context.Context, behavior.ListInput) (*behavior.ListResult, error) {
	return &behavior.ListResult{
		Total: 1,
		Items: []behavior.Model{{Code: "LEGACY-001", Title: "legacy scale"}},
	}, nil
}

type channelListBehavioralRatingStub struct {
	behavioralRatingCommandStub
}

func (s *channelListBehavioralRatingStub) List(context.Context, behavioral_rating.ListInput) (*behavioral_rating.ModelListResult, error) {
	return &behavioral_rating.ModelListResult{
		Total: 1,
		Items: []behavioral_rating.ModelSummary{{Code: "BR-001", Kind: behavioral_rating.KindBehavioralRating, Title: "BRIEF-2"}},
	}, nil
}

type channelListCognitiveStub struct {
	cognitiveCommandStub
}

func (s *channelListCognitiveStub) List(context.Context, cognitive.ListInput) (*cognitive.ModelListResult, error) {
	return &cognitive.ModelListResult{
		Total: 1,
		Items: []cognitive.ModelSummary{{Code: "COG-001", Kind: cognitive.KindCognitive, Title: "SPM"}},
	}, nil
}

func TestListBehaviorAbilityChannelAggregatesFamilies(t *testing.T) {
	t.Parallel()

	svc := NewService(Dependencies{
		BehaviorCommand:         &channelListBehaviorStub{},
		BehavioralRatingCommand: &channelListBehavioralRatingStub{},
		CognitiveCommand:        &channelListCognitiveStub{},
	})

	result, err := svc.List(context.Background(), ListModelsDTO{
		Kind:     KindBehaviorAbility,
		Page:     1,
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.Total != 3 {
		t.Fatalf("total = %d, want 3", result.Total)
	}
	if len(result.Items) != 3 {
		t.Fatalf("items = %#v", result.Items)
	}
	kinds := map[string]bool{}
	for _, item := range result.Items {
		kinds[item.Kind] = true
	}
	for _, want := range []string{KindBehaviorAbility, KindBehavioralRating, KindCognitive} {
		if !kinds[want] {
			t.Fatalf("missing kind %q in %#v", want, kinds)
		}
	}
}

func TestListBehaviorAbilityChannelFamilyFilter(t *testing.T) {
	t.Parallel()

	svc := NewService(Dependencies{
		BehaviorCommand:         &channelListBehaviorStub{},
		BehavioralRatingCommand: &channelListBehavioralRatingStub{},
		CognitiveCommand:        &channelListCognitiveStub{},
	})

	result, err := svc.List(context.Background(), ListModelsDTO{
		Kind:        KindBehaviorAbility,
		ModelFamily: string(domain.KindBehavioralRating),
		Page:        1,
		PageSize:    20,
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if result.Total != 1 || len(result.Items) != 1 || result.Items[0].Kind != KindBehavioralRating {
		t.Fatalf("result = %#v", result)
	}
}

func TestOptionsExposeBehaviorAbilityModelFamilies(t *testing.T) {
	t.Parallel()

	svc := NewService(Dependencies{})
	result, err := svc.Options(context.Background(), KindBehaviorAbility)
	if err != nil {
		t.Fatalf("Options: %v", err)
	}
	if len(result.ModelFamilies) != 3 {
		t.Fatalf("model families = %#v", result.ModelFamilies)
	}
}
