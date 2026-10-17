package modelcatalog

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"go.mongodb.org/mongo-driver/bson"
)

func durableProjectionDefinition() *domain.Definition {
	return &domain.Definition{
		Outcomes: []conclusion.Outcome{{Code: "result", Title: "title", Summary: "summary", Description: "description"}},
		Conclusions: []conclusion.Conclusion{conclusion.TypeConclusion{Profiles: []conclusion.TypeOutcomeProfile{{
			OutcomeCode: "result", Pattern: "I-N-T-J", Traits: []string{"t"}, Strengths: []string{"s"}, Weaknesses: []string{"w"}, Suggestions: []string{"a"},
			ImageURL: "url", Image: "image", Rarity: conclusion.Rarity{Percent: 2.1, Label: "rare", OneInX: 48}, IsSpecial: true, Trigger: "trigger", Commentary: "commentary",
		}}}},
		ReportMap: modeldefinition.ReportMap{Sections: []modeldefinition.ReportSection{{Code: "main", Title: "title", SourceRefs: []string{"result", "factor"}, Kind: "personality_type", AdapterKey: "personality_type", TemplateID: "mbti", TemplateVersion: "v1", CategoryLabel: "category"}}},
	}
}

func TestReportAssetsLegacyProjectionMatchesActualBSONCodecForFourFamilies(t *testing.T) {
	for _, family := range []string{"factor_scoring", "factor_norm", "task_performance", "factor_classification"} {
		t.Run(family, func(t *testing.T) {
			def := durableProjectionDefinition()
			rules := []conclusion.ScoreRangeOutcome{{OutcomeCode: "inline", Title: "inline title", Summary: "inline summary", Description: "inline description"}}
			switch family {
			case "factor_scoring":
				def.Conclusions = []conclusion.Conclusion{conclusion.RiskConclusion{FactorCode: "total", Rules: rules}}
			case "factor_norm":
				def.Conclusions = []conclusion.Conclusion{conclusion.NormConclusion{FactorCode: "total", Rules: rules}}
			case "task_performance":
				def.Conclusions = []conclusion.Conclusion{conclusion.AbilityConclusion{FactorCode: "total", Rules: rules}}
			}
			modeldefinition.MaterializeLayers(def)
			canonicalBefore, _ := json.Marshal(def)
			raw, err := bson.Marshal(definitionToPO(def))
			if err != nil {
				t.Fatal(err)
			}
			var stored DefinitionPO
			if err := bson.Unmarshal(raw, &stored); err != nil {
				t.Fatal(err)
			}
			read := definitionFromPO(&stored)
			legacy, err := modeldefinition.ReportAssetsProjectionFrom(read, interpretationassets.ProjectionLegacyV1)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(legacy, read.InterpretationAssets) {
				t.Fatal("frozen legacy codec differs from actual durable whole assets")
			}
			if got := modeldefinition.CheckReportAssetsProjection(read, 1); got != modeldefinition.ReportAssetsProjectionMatched {
				t.Fatal(got)
			}
			if family == "factor_classification" {
				if read.Conclusions[0].(conclusion.TypeConclusion).Profiles[0].Rarity.Label != "rare" {
					t.Fatal("canonical Rarity lost")
				}
				if got := modeldefinition.CheckReportAssetsProjection(read, 2); got != modeldefinition.ReportAssetsProjectionMismatched {
					t.Fatal("V2 hid durable Rarity loss")
				}
			}
			canonicalAfter, _ := json.Marshal(def)
			if string(canonicalBefore) != string(canonicalAfter) {
				t.Fatal("shadow mutated source Definition")
			}
		})
	}
}

func TestReportAssetsProjectionMarkerRoundTripsOnlyAtRecordTopLevel(t *testing.T) {
	def := definitionFromPO(definitionToPO(durableProjectionDefinition()))
	wantHash, _ := modeldefinition.CanonicalContentHash(def)
	for _, version := range []interpretationassets.ProjectionVersion{0, 1, 2, 99} {
		draft := &domain.AssessmentModel{Code: "M", Kind: domain.KindTypology, DefinitionV2: def, ReportAssetsProjectionVersion: version}
		published := &port.PublishedModel{Code: "M", Version: "v1", Kind: domain.KindTypology, DefinitionV2: def, ReportAssetsProjectionVersion: version}
		draftPO := NewDraftMapper().ToPO(draft)
		publishedPO := NewMapper().ToPO(published)
		for _, po := range []any{draftPO, publishedPO} {
			raw, err := bson.Marshal(po)
			if err != nil {
				t.Fatal(err)
			}
			var record bson.M
			if err := bson.Unmarshal(raw, &record); err != nil {
				t.Fatal(err)
			}
			definitionRaw, _ := bson.Marshal(record["definition_v2"])
			if bson.Raw(definitionRaw).Lookup("report_assets_projection_version").Type != 0 {
				t.Fatal("marker entered Definition")
			}
			if version == 0 && record["report_assets_projection_version"] != nil {
				t.Fatal("legacy absent marker assigned")
			}
		}
		gotDraft := NewDraftMapper().ToDomain(draftPO)
		gotPublished := NewMapper().ToPublished(publishedPO)
		if gotDraft.ReportAssetsProjectionVersion != version || gotPublished.ReportAssetsProjectionVersion != version {
			t.Fatal("record marker changed on read")
		}
		for _, value := range []*domain.Definition{gotDraft.DefinitionV2, gotPublished.DefinitionV2} {
			hash, _ := modeldefinition.CanonicalContentHash(value)
			if hash != wantHash {
				t.Fatal("record marker changed canonical hash")
			}
			raw, _ := json.Marshal(value)
			if strings.Contains(string(raw), "projection_version") {
				t.Fatal("marker entered public Definition JSON")
			}
		}
	}
}

func TestLegacyUnassignedMarkerIsIdempotentWithoutEquatingV2(t *testing.T) {
	a := &port.PublishedModel{Code: "M", Version: "v1", DefinitionV2: durableProjectionDefinition()}
	b := *a
	b.ReportAssetsProjectionVersion = 1
	if !sameImmutablePublishedContent(a, &b) {
		t.Fatal("legacy repeat publication became a conflict")
	}
	if a.ReportAssetsProjectionVersion != 0 {
		t.Fatal("comparison assigned persisted marker")
	}
	for _, version := range []interpretationassets.ProjectionVersion{2, 99} {
		b.ReportAssetsProjectionVersion = version
		if sameImmutablePublishedContent(a, &b) {
			t.Fatal("different encoding equated to legacy")
		}
	}
}
