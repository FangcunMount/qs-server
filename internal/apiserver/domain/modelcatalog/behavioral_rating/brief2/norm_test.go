package brief2_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/behavioral_rating/brief2"
)

func TestLookupNormScoreDirectTable(t *testing.T) {
	t.Parallel()

	tables := &brief2.NormTables{
		Factors: []brief2.FactorNormTable{{
			FactorCode: "gec",
			Lookup: []brief2.NormLookupEntry{
				{RawMin: 0, RawMax: 8, TScore: 45, Percentile: 30},
				{RawMin: 9, RawMax: 15, TScore: 58, Percentile: 65},
			},
		}},
	}
	score, ok := brief2.LookupNormScore(tables, "gec", 10, brief2.Subject{})
	if !ok || score.TScore != 58 || score.Percentile != 65 {
		t.Fatalf("score = %#v, ok = %v", score, ok)
	}
}

func TestLookupNormScoreParametricBand(t *testing.T) {
	t.Parallel()

	mean := 10.0
	std := 2.0
	tables := &brief2.NormTables{
		Factors: []brief2.FactorNormTable{{
			FactorCode: "bri",
			Bands: []brief2.NormBand{{
				MinAgeMonths: 60,
				MaxAgeMonths: 120,
				Mean:         &mean,
				StdDev:       &std,
			}},
		}},
	}
	score, ok := brief2.LookupNormScore(tables, "bri", 12, brief2.Subject{AgeMonths: 72})
	if !ok || score.TScore != 60 {
		t.Fatalf("score = %#v, ok = %v", score, ok)
	}
}

func TestInterpretTScore(t *testing.T) {
	t.Parallel()

	tables := &brief2.NormTables{
		TScoreRules: []brief2.TScoreInterpretRule{{
			FactorCode: "gec",
			Ranges: []brief2.TScoreRange{
				{MinT: 0, MaxT: 59, Level: "low", Conclusion: "正常"},
				{MinT: 60, MaxT: 100, Level: "elevated", Conclusion: "升高"},
			},
		}},
	}
	level, conclusion, _, ok := brief2.InterpretTScore(tables, "gec", 65)
	if !ok || level != "elevated" || conclusion != "升高" {
		t.Fatalf("interpret = %q %q ok=%v", level, conclusion, ok)
	}
}
