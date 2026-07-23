package query

import (
	"testing"

	modelcatalog "github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestKindsFromListInput(t *testing.T) {
	tests := []struct {
		name  string
		input modelcatalog.ListModelsDTO
		want  []domain.Kind
		fail  bool
	}{
		{name: "deduplicates kinds", input: modelcatalog.ListModelsDTO{Kinds: []string{"cognitive", "behavioral_rating", "cognitive"}}, want: []domain.Kind{domain.KindBehavioralRating, domain.KindCognitive}},
		{name: "legacy product channel maps to kinds", input: modelcatalog.ListModelsDTO{ProductChannel: "behavior_ability"}, want: []domain.Kind{domain.KindBehavioralRating, domain.KindCognitive}},
		{name: "legacy sub kind maps to typology", input: modelcatalog.ListModelsDTO{SubKind: "typology"}, want: []domain.Kind{domain.KindTypology}},
		{name: "kind and kinds are exclusive", input: modelcatalog.ListModelsDTO{Kind: "scale", Kinds: []string{"scale"}}, fail: true},
		{name: "conflicting legacy filter is rejected", input: modelcatalog.ListModelsDTO{Kind: "scale", ProductChannel: "behavior_ability"}, fail: true},
		{name: "invalid kind is rejected", input: modelcatalog.ListModelsDTO{Kinds: []string{"unknown"}}, fail: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := kindsFromListInput(tt.input)
			if tt.fail {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("kinds=%#v want=%#v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("kinds=%#v want=%#v", got, tt.want)
				}
			}
		})
	}
}
