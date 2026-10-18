package evaluationinput_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func TestFactorCatalogFromDefinitionUsesMeasure(t *testing.T) {
	t.Parallel()
	max := 27.0
	catalog := evaluationinput.FactorCatalogFromDefinition(modeldefinition.MeasureSpec{
		Factors: []factor.Factor{{Code: "total", Title: "总分", Role: factor.FactorRoleTotal}},
		Scoring: []factor.Scoring{{FactorCode: "total", MaxScore: &max}},
	})
	if len(catalog) != 1 || catalog[0].Title != "总分" || catalog[0].MaxScore == nil || *catalog[0].MaxScore != 27 {
		t.Fatalf("catalog = %#v", catalog)
	}
}

func TestInterpretationAssetsFromSnapshotRequiresFrozenAssets(t *testing.T) {
	t.Parallel()
	def := &modeldefinition.Definition{
		Outcomes: []conclusion.Outcome{{Code: "low", Summary: "canonical"}},
	}
	snapshot := &evaluationinput.InputSnapshot{DefinitionV2: def}
	if _, ok := evaluationinput.InterpretationAssetsFromSnapshot(snapshot); ok {
		t.Fatal("unversioned Definition became report assets")
	}
	frozen := interpretationassets.Assets{Outcomes: []interpretationassets.OutcomePresentation{{OutcomeCode: "low", Summary: "frozen"}}}
	snapshot.InterpretationAssets = &frozen
	assets, ok := evaluationinput.InterpretationAssetsFromSnapshot(snapshot)
	got, found := assets.FindOutcome("low")
	if !ok || !found || got.Summary != "frozen" {
		t.Fatal("frozen report assets lost")
	}
}
