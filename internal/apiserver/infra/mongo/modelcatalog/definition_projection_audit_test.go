package modelcatalog

import (
	"bytes"
	"encoding/json"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	"go.mongodb.org/mongo-driver/bson"
)

func projectionDefinition(family string) *modeldefinition.Definition {
	def := &modeldefinition.Definition{
		Measure:   modeldefinition.MeasureSpec{Factors: []factor.Factor{{Code: "total", Role: factor.FactorRoleTotal}, {Code: "trait", Role: factor.FactorRoleDimension}}, Scoring: []factor.Scoring{{FactorCode: "trait", Strategy: factor.ScoringStrategySum, Sources: []factor.ScoringSource{{Kind: factor.ScoringSourceQuestion, Code: "q1", Sign: 1}}}}},
		Outcomes:  []conclusion.Outcome{{Code: "low", Summary: "private report copy"}, {Code: "high", Summary: "other private copy"}},
		ReportMap: modeldefinition.ReportMap{Sections: []modeldefinition.ReportSection{{Code: "scores", Kind: modeldefinition.ReportSectionKindFactorScores, SourceRefs: []string{"total", "trait"}, TemplateVersion: "v1"}}},
	}
	rules := []conclusion.ScoreRangeOutcome{{MinScore: 0, MaxScore: 10, MaxInclusive: true, OutcomeCode: "low"}}
	switch family {
	case "scale":
		def.Conclusions = []conclusion.Conclusion{conclusion.RiskConclusion{FactorCode: "total", Rules: rules}}
	case "behavioral_rating":
		def.Calibration.NormRefs = []norm.Ref{{FactorCode: "total", NormTableVersion: "test-v1"}}
		def.Conclusions = []conclusion.Conclusion{conclusion.NormConclusion{FactorCode: "total", ScoreBasis: conclusion.ScoreBasisTScore, Primary: true, Rules: rules}}
	case "cognitive":
		def.Execution.SPM = &modeldefinition.SPMSpec{TimeLimitSeconds: 60, TotalFactorCode: "total", ItemSets: []modeldefinition.SPMItemSet{{Code: "A", Items: []modeldefinition.SPMItem{{QuestionCode: "q1", CorrectOptionCode: "A"}}}}}
		def.Conclusions = []conclusion.Conclusion{conclusion.AbilityConclusion{FactorCode: "total", ScoreBasis: conclusion.ScoreBasisRaw, Primary: true, Rules: rules}}
	case "typology":
		def.Conclusions = []conclusion.Conclusion{conclusion.TypeConclusion{FactorCodes: []string{"trait"}, Decision: conclusion.TypeDecision{Kind: binding.DecisionKindTraitProfile}, Profiles: []conclusion.TypeOutcomeProfile{{OutcomeCode: "low", Pattern: "trait", Traits: []string{"private trait"}}}}}
		def.ReportMap.Sections = append(def.ReportMap.Sections, modeldefinition.ReportSection{Code: "type", Kind: "template", TemplateID: "bigfive", AdapterKey: "trait_profile", TemplateVersion: "v1"})
	}
	modeldefinition.MaterializeLayers(def)
	return def
}

func projectionRaw(t *testing.T, def *modeldefinition.Definition) bson.Raw {
	t.Helper()
	data, err := bson.Marshal(definitionToPO(def))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestProjectionAuditFourFamiliesAndNilEmptyStorage(t *testing.T) {
	for _, family := range []string{"scale", "typology", "behavioral_rating", "cognitive"} {
		t.Run(family, func(t *testing.T) {
			raw := projectionRaw(t, projectionDefinition(family))
			original := bytes.Clone(raw)
			result := AuditDefinitionProjections(raw)
			if result.InvalidCount != 0 || result.DifferenceCount != 0 || result.HashError || len(result.CanonicalHash) != 64 {
				t.Fatalf("audit=%+v", result)
			}
			if !bytes.Equal(raw, original) {
				t.Fatal("read-only audit changed raw storage")
			}
		})
	}
	var stored bson.M
	if err := bson.Unmarshal(projectionRaw(t, projectionDefinition("scale")), &stored); err != nil {
		t.Fatal(err)
	}
	assets := stored["interpretation_assets"].(bson.M)
	for _, empty := range []any{nil, bson.A{}} {
		assets["profiles"] = empty
		raw, _ := bson.Marshal(stored)
		if result := AuditDefinitionProjections(raw); result.DifferenceCount != 0 || result.InvalidCount != 0 {
			t.Fatalf("nil/empty=%+v", result)
		}
	}
}

func TestProjectionAuditDoesNotUseStoredPreferenceOrHideStorageLoss(t *testing.T) {
	def := projectionDefinition("scale")
	def.InterpretationAssets.Outcomes[0].Summary = "must never print replacement report body"
	def.DecisionSpec.ScoreRanges[0].Rules[0].MaxScore = 999
	def.InterpretationAssets.ReportSpec.Sections[0].TemplateVersion = "v999"
	result := AuditDefinitionProjections(projectionRaw(t, def))
	for _, path := range []string{"definition_v2.decision_spec.ScoreRanges[0].Rules[0].MaxScore", "definition_v2.interpretation_assets.Outcomes[0].Summary", "definition_v2.interpretation_assets.ReportSpec.Sections[0].TemplateVersion"} {
		if !slices.Contains(result.DifferencePaths, path) {
			t.Fatalf("missing %s: %+v", path, result)
		}
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "report body") || strings.Contains(string(encoded), "private report") || strings.Contains(string(encoded), "999") {
		t.Fatalf("audit disclosed content: %s", encoded)
	}
	typology := projectionDefinition("typology")
	c := typology.Conclusions[0].(conclusion.TypeConclusion)
	c.Profiles[0].Rarity = conclusion.Rarity{Percent: 7.5, Label: "private rarity", OneInX: 13}
	typology.Conclusions[0] = c
	modeldefinition.MaterializeLayers(typology)
	result = AuditDefinitionProjections(projectionRaw(t, typology))
	if result.DifferenceCount != 3 || !slices.Contains(result.DifferencePaths, "definition_v2.interpretation_assets.Profiles[0].Rarity.Percent") {
		t.Fatalf("storage rarity loss hidden: %+v", result)
	}
}

func TestProjectionAuditKeepsOrderAndRejectsDuplicatesUnknownAndInvalid(t *testing.T) {
	def := projectionDefinition("scale")
	slices.Reverse(def.InterpretationAssets.Outcomes)
	if result := AuditDefinitionProjections(projectionRaw(t, def)); result.DifferenceCount == 0 {
		t.Fatal("ordered asset changes were normalized away")
	}
	def = projectionDefinition("scale")
	def.Outcomes = append(def.Outcomes, def.Outcomes[0])
	if result := AuditDefinitionProjections(projectionRaw(t, def)); result.InvalidCount == 0 {
		t.Fatal("duplicate canonical codes accepted")
	}
	def = projectionDefinition("scale")
	def.InterpretationAssets.Outcomes = append(def.InterpretationAssets.Outcomes, def.InterpretationAssets.Outcomes[0])
	if result := AuditDefinitionProjections(projectionRaw(t, def)); result.InvalidCount == 0 {
		t.Fatal("duplicate stored codes accepted")
	}
	var stored bson.M
	_ = bson.Unmarshal(projectionRaw(t, projectionDefinition("scale")), &stored)
	stored["unexamined_schema_field"] = "private copy"
	raw, _ := bson.Marshal(stored)
	if result := AuditDefinitionProjections(raw); result.InvalidCount == 0 {
		t.Fatal("unknown Definition field silently discarded")
	}
	po := definitionToPO(projectionDefinition("scale"))
	po.Conclusions[0].Kind = "future-kind"
	raw, _ = bson.Marshal(po)
	if result := AuditDefinitionProjections(raw); result.InvalidCount == 0 {
		t.Fatal("unknown conclusion silently discarded")
	}
	po = definitionToPO(projectionDefinition("scale"))
	po.Measure.Scoring[0].Constant = math.NaN()
	raw, _ = bson.Marshal(po)
	if result := AuditDefinitionProjections(raw); !result.HashError && result.InvalidCount == 0 {
		t.Fatal("hash encoding error treated as match")
	}
	if result := AuditDefinitionProjections(bson.Raw{1, 2}); result.InvalidCount == 0 {
		t.Fatal("malformed BSON accepted")
	}
}
