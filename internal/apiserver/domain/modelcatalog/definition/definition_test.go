package definition_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
)

func TestDefinitionComposesTargetLayers(t *testing.T) {
	t.Parallel()

	def := definition.Definition{
		Measure: definition.MeasureSpec{
			Factors: []factor.Factor{{Code: "total", Title: "总分", Role: factor.FactorRoleTotal}},
			Scoring: []factor.Scoring{{FactorCode: "total", Strategy: factor.ScoringStrategySum}},
		},
		Calibration: definition.Calibration{
			NormRefs: []norm.Ref{{FactorCode: "total", NormTableVersion: "2026"}},
		},
		Conclusions: []conclusion.Conclusion{
			conclusion.RiskConclusion{FactorCode: "total"},
		},
		Outcomes: []conclusion.Outcome{{Code: "low", Title: "低风险"}},
		ReportMap: definition.ReportMap{
			Sections: []definition.ReportSection{{Code: "summary", SourceRefs: []string{"total"}}},
		},
	}

	if len(def.Measure.Factors) != 1 || len(def.Calibration.NormRefs) != 1 ||
		len(def.Conclusions) != 1 || len(def.Outcomes) != 1 || len(def.ReportMap.Sections) != 1 {
		t.Fatalf("definition layers not composed: %#v", def)
	}
}

func TestReportMapFactorScoreSourcesDistinguishesAbsentAndEmpty(t *testing.T) {
	t.Parallel()

	if _, configured := (definition.ReportMap{}).FactorScoreSources(); configured {
		t.Fatal("absent factor score section must not be configured")
	}
	mapWithEmpty := definition.ReportMap{Sections: []definition.ReportSection{{
		Code: definition.ReportSectionKindFactorScores,
		Kind: definition.ReportSectionKindFactorScores,
	}}}
	codes, configured := mapWithEmpty.FactorScoreSources()
	if !configured || codes != nil {
		t.Fatalf("empty factor score section = (%#v, %v), want (nil, true)", codes, configured)
	}
}
