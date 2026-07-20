package evaluationinput_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func TestAttachCanonicalDefinitionMaterializesAssets(t *testing.T) {
	t.Parallel()
	def := &modeldefinition.Definition{
		Measure:  modeldefinition.MeasureSpec{Factors: []factor.Factor{{Code: "total", Title: "总分"}}},
		Outcomes: []conclusion.Outcome{{Code: "low", Title: "低", Summary: "摘要"}},
	}
	modeldefinition.MaterializeLayers(def)
	snapshot := &evaluationinput.InputSnapshot{}
	evaluationinput.AttachCanonicalDefinition(snapshot, def)
	if snapshot.DefinitionV2 == nil {
		t.Fatal("expected definition v2")
	}
	if snapshot.InterpretationAssets == nil {
		t.Fatal("expected interpretation assets")
	}
	got, ok := snapshot.InterpretationAssets.FindOutcome("low")
	if !ok || got.Summary != "摘要" {
		t.Fatalf("assets = %#v ok=%v", got, ok)
	}
}

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

func TestInterpretationAssetsFromSnapshotPrefersCanonical(t *testing.T) {
	t.Parallel()
	def := &modeldefinition.Definition{
		Outcomes: []conclusion.Outcome{{Code: "low", Summary: "canonical"}},
	}
	modeldefinition.MaterializeLayers(def)
	snapshot := &evaluationinput.InputSnapshot{DefinitionV2: def}
	assets, ok := evaluationinput.InterpretationAssetsFromSnapshot(snapshot)
	got, found := assets.FindOutcome("low")
	if !ok || !found || got.Summary != "canonical" {
		t.Fatalf("assets = %#v ok=%v found=%v", assets, ok, found)
	}
	_ = interpretationassets.Assets{}
}
