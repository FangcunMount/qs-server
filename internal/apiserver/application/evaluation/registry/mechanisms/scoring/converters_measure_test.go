package scoring

import (
	"testing"

	calcscoring "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/scoring"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

func TestModelFromSnapshotPrefersCanonicalMeasureSemantics(t *testing.T) {
	t.Parallel()
	snapshot := &scalesnapshot.ScaleSnapshot{
		Code: "SCL",
		Factors: []scalesnapshot.FactorSnapshot{{
			Code: "total", Title: "总分", IsTotalScore: true, ScoringStrategy: "sum",
			// Flat surface lost factor sources — Measure must restore them.
		}, {
			Code: "dim_a", Title: "A", QuestionCodes: []string{"q1"}, ScoringStrategy: "sum",
		}},
		Measure: &definition.MeasureSpec{
			Factors: []factor.Factor{
				{Code: "total", Title: "总分", Role: factor.FactorRoleTotal},
				{Code: "dim_a", Title: "A", Role: factor.FactorRoleDimension},
			},
			FactorGraph: factor.FactorGraph{
				Roots: []string{"total"},
				Edges: []factor.FactorEdge{{ParentCode: "total", ChildCode: "dim_a"}},
			},
			Scoring: []factor.Scoring{{
				FactorCode: "total",
				Strategy:   factor.ScoringStrategySum,
				Sources:    []factor.ScoringSource{{Kind: factor.ScoringSourceFactor, Code: "dim_a"}},
			}, {
				FactorCode: "dim_a",
				Strategy:   factor.ScoringStrategySum,
				Sources: []factor.ScoringSource{{
					Kind: factor.ScoringSourceQuestion, Code: "q1", Sign: -1, Weight: 0.5,
				}},
			}},
		},
	}
	model := modelFromSnapshot(snapshot)
	total := findCalcFactor(model.Factors, "total")
	if total == nil || len(total.ChildCodes) != 1 || total.ChildCodes[0] != "dim_a" {
		t.Fatalf("total = %#v, want ChildCodes=[dim_a]", total)
	}
	if len(total.QuestionCodes) != 0 {
		t.Fatalf("total QuestionCodes = %#v, want empty", total.QuestionCodes)
	}
	dim := findCalcFactor(model.Factors, "dim_a")
	if dim == nil || len(dim.Contributions) != 1 {
		t.Fatalf("dim_a = %#v", dim)
	}
	if dim.Contributions[0].Sign != -1 || dim.Contributions[0].Weight != 0.5 {
		t.Fatalf("contribution = %#v", dim.Contributions[0])
	}
}

func TestModelFromSnapshotRejectsFlatOnlyRuntimeProjection(t *testing.T) {
	t.Parallel()
	snapshot := &scalesnapshot.ScaleSnapshot{
		Code: "FLAT_ONLY",
		Factors: []scalesnapshot.FactorSnapshot{{
			Code: "total", IsTotalScore: true, QuestionCodes: []string{"q1"}, ScoringStrategy: "sum",
		}},
	}
	model := modelFromSnapshot(snapshot)
	if len(model.Factors) != 0 || model.Code != "" {
		t.Fatalf("flat-only runtime projection must not be executable: %#v", model)
	}
}

func findCalcFactor(factors []calcscoring.Factor, code string) *calcscoring.Factor {
	for i := range factors {
		if factors[i].Code == code {
			return &factors[i]
		}
	}
	return nil
}
