package definition

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
)

func projectionDefinition() *Definition {
	return &Definition{
		Outcomes: []conclusion.Outcome{{Code: "INTJ", Title: "title", Summary: "summary", Description: "description"}},
		Conclusions: []conclusion.Conclusion{conclusion.TypeConclusion{Profiles: []conclusion.TypeOutcomeProfile{{
			OutcomeCode: "INTJ", Pattern: "I-N-T-J", Traits: []string{"t"}, Strengths: []string{"s"}, Weaknesses: []string{"w"}, Suggestions: []string{"a"},
			ImageURL: "image-url", Image: "image", Rarity: conclusion.Rarity{Percent: 2.1, Label: "rare", OneInX: 48},
			IsSpecial: true, Trigger: "trigger", Commentary: "commentary",
		}}}},
		ReportMap: ReportMap{Sections: []ReportSection{{Code: "type", Title: "report", SourceRefs: []string{"INTJ"}, Kind: "personality_type", AdapterKey: "personality_type", TemplateID: "mbti", TemplateVersion: "v1", CategoryLabel: "category"}}},
	}
}

func TestReportAssetsProjectionVersionsPreserveCanonicalAndLegacySemantics(t *testing.T) {
	def := projectionDefinition()
	before, _ := json.Marshal(def)
	legacy, err := ReportAssetsProjectionFrom(def, interpretationassets.ProjectionLegacyV1)
	if err != nil {
		t.Fatal(err)
	}
	full, err := ReportAssetsProjectionFrom(def, interpretationassets.ProjectionCanonicalV2)
	if err != nil {
		t.Fatal(err)
	}
	if legacy.Profiles[0].Rarity != (interpretationassets.RarityPresentation{}) {
		t.Fatal("legacy codec gained Rarity")
	}
	if full.Profiles[0].Rarity != (interpretationassets.RarityPresentation{Percent: 2.1, Label: "rare", OneInX: 48}) {
		t.Fatal("canonical Rarity lost")
	}
	full.Profiles[0].Rarity = interpretationassets.RarityPresentation{}
	if !reflect.DeepEqual(full, legacy) {
		t.Fatal("legacy codec lost another asset field")
	}
	legacy.Profiles[0].Traits[0] = "mutated"
	legacy.ReportSpec.Sections[0].SourceRefs[0] = "mutated"
	after, _ := json.Marshal(def)
	if string(before) != string(after) {
		t.Fatal("pure projection mutated canonical input")
	}
	for _, version := range []interpretationassets.ProjectionVersion{0, -1, 99} {
		if _, err := ReportAssetsProjectionFrom(def, version); !errors.Is(err, ErrUnknownReportAssetsProjection) {
			t.Fatalf("version %d: %v", version, err)
		}
	}
}

func TestResolveReportAssetsRequiresKnownVersionAndPreservesProjection(t *testing.T) {
	for _, version := range []interpretationassets.ProjectionVersion{1, 2} {
		def := projectionDefinition()
		want, err := ReportAssetsProjectionFrom(def, version)
		if err != nil {
			t.Fatal(err)
		}
		got, err := ResolveReportAssetsProjection(def, version)
		if err != nil || !reflect.DeepEqual(want, got) {
			t.Fatalf("version %d: %v", version, err)
		}
		if _, err := ResolveReportAssetsProjection(nil, version); !errors.Is(err, ErrMissingReportAssetsDefinition) {
			t.Fatal(err)
		}
	}
	for _, version := range []interpretationassets.ProjectionVersion{0, -1, 99} {
		if _, err := ResolveReportAssetsProjection(projectionDefinition(), version); !errors.Is(err, ErrUnknownReportAssetsProjection) {
			t.Fatalf("%d: %v", version, err)
		}
	}
}
