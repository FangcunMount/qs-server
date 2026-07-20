package norm_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
)

func TestProjectionKeepsOutcomeCodeWithoutPresentationCopy(t *testing.T) {
	t.Parallel()
	const outcomeCode = "elevated"
	const longCopy = "问卷提示日常执行功能表现的整体水平方面的困难较为明显，可能已影响学习、生活自理、情绪行为和人际适应的整体功能。"
	tables := &calcnorm.NormTables{
		Factors: []calcnorm.FactorNormTable{{
			FactorCode: "gec",
			Lookup: []calcnorm.NormLookupEntry{{
				RawMin: 0, RawMax: 100, TScore: 65, Percentile: 90,
			}},
		}},
		TScoreRules: []calcnorm.TScoreInterpretRule{{
			FactorCode: "gec",
			Ranges: []calcnorm.TScoreRange{{
				MinT: 60, MaxT: 100, MaxInclusive: true,
				Level: outcomeCode, Conclusion: longCopy, Suggestion: "建议关注",
			}},
		}},
	}
	result := calcnorm.Projection{
		Tables:               tables,
		PrimaryDimensionCode: "gec",
	}.Apply(&calculation.Result{
		Dimensions: []calculation.DimensionResult{{
			Code:  "gec",
			Score: &calculation.ScoreValue{Value: 10},
		}},
	})
	dim := result.Dimensions[0]
	if dim.Level == nil || dim.Level.Code != outcomeCode {
		t.Fatalf("level code = %#v, want %q", dim.Level, outcomeCode)
	}
	if dim.Level.Label != "" || dim.Description != "" || dim.Suggestion != "" {
		t.Fatalf("dimension carries presentation copy: level=%#v desc=%q sugg=%q", dim.Level, dim.Description, dim.Suggestion)
	}
	if result.PrimaryLabel != "" {
		t.Fatalf("PrimaryLabel = %q, want unset", result.PrimaryLabel)
	}
}
