package modelcatalog

import (
	"reflect"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
)

func TestDefinitionLayersRoundTripPO(t *testing.T) {
	t.Parallel()
	def := &domain.Definition{
		Outcomes: []conclusion.Outcome{{Code: "low", Title: "低", Summary: "摘要"}},
		Conclusions: []conclusion.Conclusion{conclusion.RiskConclusion{
			FactorCode: "total",
			Rules:      []conclusion.ScoreRangeOutcome{{MinScore: 0, MaxScore: 10, MaxInclusive: true, OutcomeCode: "low"}},
		}, conclusion.TypeConclusion{
			Decision: conclusion.TypeDecision{Kind: binding.DecisionKindPoleComposition},
			Profiles: []conclusion.TypeOutcomeProfile{{OutcomeCode: "ENTJ", Pattern: "E-N-T-J"}},
		}},
		ReportMap: modeldefinition.ReportMap{Sections: []modeldefinition.ReportSection{{
			Code: "main", AdapterKey: "personality_type",
		}}},
	}
	modeldefinition.MaterializeLayers(def)
	got := definitionFromPO(definitionToPO(def))
	if got == nil || !got.DecisionSpec.IsMaterialized() || !got.InterpretationAssets.IsMaterialized() {
		t.Fatalf("layers = decision:%#v assets:%#v", got.DecisionSpec, got.InterpretationAssets)
	}
	if !reflect.DeepEqual(got.DecisionSpec, def.DecisionSpec) {
		t.Fatalf("decision spec = %#v, want %#v", got.DecisionSpec, def.DecisionSpec)
	}
	if len(got.InterpretationAssets.Outcomes) == 0 || len(got.InterpretationAssets.Profiles) != 1 {
		t.Fatalf("interpretation assets = %#v", got.InterpretationAssets)
	}
}
