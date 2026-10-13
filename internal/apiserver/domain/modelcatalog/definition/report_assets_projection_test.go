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

func TestReportAssetsShadowChecksAllFieldsWithoutLooseningOrderOrDuplicates(t *testing.T) {
	mutations := map[string]func(*interpretationassets.Assets){
		"outcome code":        func(a *interpretationassets.Assets) { a.Outcomes[0].OutcomeCode += "x" },
		"outcome title":       func(a *interpretationassets.Assets) { a.Outcomes[0].Title += "x" },
		"outcome summary":     func(a *interpretationassets.Assets) { a.Outcomes[0].Summary += "x" },
		"outcome description": func(a *interpretationassets.Assets) { a.Outcomes[0].Description += "x" },
		"profile code":        func(a *interpretationassets.Assets) { a.Profiles[0].OutcomeCode += "x" },
		"pattern":             func(a *interpretationassets.Assets) { a.Profiles[0].Pattern += "x" },
		"traits":              func(a *interpretationassets.Assets) { a.Profiles[0].Traits[0] += "x" },
		"strengths":           func(a *interpretationassets.Assets) { a.Profiles[0].Strengths[0] += "x" },
		"weaknesses":          func(a *interpretationassets.Assets) { a.Profiles[0].Weaknesses[0] += "x" },
		"suggestions":         func(a *interpretationassets.Assets) { a.Profiles[0].Suggestions[0] += "x" },
		"image URL":           func(a *interpretationassets.Assets) { a.Profiles[0].ImageURL += "x" },
		"image":               func(a *interpretationassets.Assets) { a.Profiles[0].Image += "x" },
		"special":             func(a *interpretationassets.Assets) { a.Profiles[0].IsSpecial = false },
		"trigger":             func(a *interpretationassets.Assets) { a.Profiles[0].Trigger += "x" },
		"commentary":          func(a *interpretationassets.Assets) { a.Profiles[0].Commentary += "x" },
		"rarity percent":      func(a *interpretationassets.Assets) { a.Profiles[0].Rarity.Percent = 1 },
		"rarity label":        func(a *interpretationassets.Assets) { a.Profiles[0].Rarity.Label = "x" },
		"rarity one in x":     func(a *interpretationassets.Assets) { a.Profiles[0].Rarity.OneInX = 1 },
		"section code":        func(a *interpretationassets.Assets) { a.ReportSpec.Sections[0].Code += "x" },
		"section title":       func(a *interpretationassets.Assets) { a.ReportSpec.Sections[0].Title += "x" },
		"section source":      func(a *interpretationassets.Assets) { a.ReportSpec.Sections[0].SourceRefs[0] += "x" },
		"section kind":        func(a *interpretationassets.Assets) { a.ReportSpec.Sections[0].Kind += "x" },
		"adapter":             func(a *interpretationassets.Assets) { a.ReportSpec.Sections[0].AdapterKey += "x" },
		"template":            func(a *interpretationassets.Assets) { a.ReportSpec.Sections[0].TemplateID += "x" },
		"template version":    func(a *interpretationassets.Assets) { a.ReportSpec.Sections[0].TemplateVersion += "x" },
		"category":            func(a *interpretationassets.Assets) { a.ReportSpec.Sections[0].CategoryLabel += "x" },
		"duplicate code":      func(a *interpretationassets.Assets) { a.Outcomes = append(a.Outcomes, a.Outcomes[0]) },
		"array order":         func(a *interpretationassets.Assets) { a.Outcomes[0], a.Outcomes[1] = a.Outcomes[1], a.Outcomes[0] },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			def := projectionDefinition()
			def.Outcomes = append(def.Outcomes, conclusion.Outcome{Code: "ENFP", Title: "second"})
			def.InterpretationAssets, _ = ReportAssetsProjectionFrom(def, interpretationassets.ProjectionLegacyV1)
			if got := CheckReportAssetsProjection(def, 1); got != ReportAssetsProjectionMatched {
				t.Fatalf("before = %s", got)
			}
			mutate(&def.InterpretationAssets)
			if got := CheckReportAssetsProjection(def, 1); got != ReportAssetsProjectionMismatched {
				t.Fatalf("after = %s", got)
			}
		})
	}
}

func TestReportAssetsShadowOnlyNormalizesNilEmptyAndNeverAssignsVersion(t *testing.T) {
	def := projectionDefinition()
	def.InterpretationAssets, _ = ReportAssetsProjectionFrom(def, 1)
	def.InterpretationAssets.Profiles[0].Traits = nil
	def.Conclusions[0].(conclusion.TypeConclusion).Profiles[0].Traits = []string{}
	if got := CheckReportAssetsProjection(def, 1); got != ReportAssetsProjectionMatched {
		t.Fatal(got)
	}
	for version, want := range map[interpretationassets.ProjectionVersion]ReportAssetsProjectionStatus{0: ReportAssetsProjectionUnassigned, 99: ReportAssetsProjectionUnknown, 2: ReportAssetsProjectionMismatched} {
		if got := CheckReportAssetsProjection(def, version); got != want {
			t.Fatalf("%d: %s", version, got)
		}
	}
	if got := CheckReportAssetsProjection(nil, 1); got != ReportAssetsProjectionMissing {
		t.Fatal(got)
	}
}
