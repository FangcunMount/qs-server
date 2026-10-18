package definition

import (
	"encoding/json"
	"errors"
	"fmt"
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

func TestResolveReportAssetsChecksAllFieldsWithoutLooseningOrderOrDuplicates(t *testing.T) {
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
			if _, err := ResolveReportAssetsProjection(def, 1); err != nil {
				t.Fatal(err)
			}
			mutate(&def.InterpretationAssets)
			if _, err := ResolveReportAssetsProjection(def, 1); !errors.Is(err, ErrReportAssetsProjectionMismatch) {
				t.Fatalf("after mutation: %v", err)
			}
		})
	}
}

func TestResolveReportAssetsOnlyNormalizesNilEmptyAndRejectsUnknownVersions(t *testing.T) {
	def := projectionDefinition()
	def.InterpretationAssets, _ = ReportAssetsProjectionFrom(def, 1)
	def.InterpretationAssets.Profiles[0].Traits = nil
	def.Conclusions[0].(conclusion.TypeConclusion).Profiles[0].Traits = []string{}
	if _, err := ResolveReportAssetsProjection(def, 1); err != nil {
		t.Fatal(err)
	}
	for _, version := range []interpretationassets.ProjectionVersion{0, 99} {
		if _, err := ResolveReportAssetsProjection(def, version); !errors.Is(err, ErrUnknownReportAssetsProjection) {
			t.Fatalf("%d: %v", version, err)
		}
	}
	if _, err := ResolveReportAssetsProjection(def, 2); !errors.Is(err, ErrReportAssetsProjectionMismatch) {
		t.Fatal(err)
	}
	if _, err := ResolveReportAssetsProjection(nil, 1); !errors.Is(err, ErrMissingReportAssetsDefinition) {
		t.Fatal(err)
	}

}

func TestResolveReportAssetsProjectionKeepsLegacyAfterStoredFieldsRetire(t *testing.T) {
	for _, version := range []interpretationassets.ProjectionVersion{1, 2} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			def := projectionDefinition()
			def.InterpretationAssets, _ = ReportAssetsProjectionFrom(def, version)
			expected, _ := json.Marshal(def.InterpretationAssets)
			if _, err := ResolveReportAssetsProjection(def, version); err != nil {
				t.Fatal(err)
			}
			def.InterpretationAssets = interpretationassets.Assets{}
			got, err := ResolveReportAssetsProjection(def, version)
			if err != nil {
				t.Fatal(err)
			}
			actual, _ := json.Marshal(got)
			if string(actual) != string(expected) {
				t.Fatal("stored-field retirement changed versioned projection")
			}
			if version == 1 && got.Profiles[0].Rarity != (interpretationassets.RarityPresentation{}) {
				t.Fatal("old version gained rarity")
			}
			if version == 2 && got.Profiles[0].Rarity.OneInX != 48 {
				t.Fatal("new version lost rarity")
			}
		})
	}
	if _, err := ResolveReportAssetsProjection(nil, 1); !errors.Is(err, ErrMissingReportAssetsDefinition) {
		t.Fatal(err)
	}
	for _, version := range []interpretationassets.ProjectionVersion{0, 99} {
		if _, err := ResolveReportAssetsProjection(projectionDefinition(), version); !errors.Is(err, ErrUnknownReportAssetsProjection) {
			t.Fatal(err)
		}
	}
	def := projectionDefinition()
	def.InterpretationAssets, _ = ReportAssetsProjectionFrom(def, 1)
	if _, err := ResolveReportAssetsProjection(def, 2); !errors.Is(err, ErrReportAssetsProjectionMismatch) {
		t.Fatal("v2 silently replaced v1 stored assets", err)
	}
}
