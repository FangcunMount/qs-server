package norm_test

import (
	"testing"

	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
)

func TestLookupNormScoreDirectTable(t *testing.T) {
	t.Parallel()

	tables := &calcnorm.NormTables{
		Factors: []calcnorm.FactorNormTable{{
			FactorCode: "gec",
			Lookup: []calcnorm.NormLookupEntry{
				{RawMin: 0, RawMax: 8, TScore: 45, Percentile: 30},
				{RawMin: 9, RawMax: 15, TScore: 58, Percentile: 65},
			},
		}},
	}
	score, ok := calcnorm.LookupNormScore(tables, "gec", 10, calcnorm.Subject{})
	if !ok || score.TScore != 58 || score.Percentile != 65 {
		t.Fatalf("score = %#v, ok = %v", score, ok)
	}
}

func TestLookupNormScoreSelectsDemographicDirectLookup(t *testing.T) {
	t.Parallel()

	tables := &calcnorm.NormTables{
		Factors: []calcnorm.FactorNormTable{{
			FactorCode: "gec",
			Lookup: []calcnorm.NormLookupEntry{
				{RawMin: 10, RawMax: 10, MinAgeMonths: 60, MaxAgeMonths: 95, Gender: "male", TScore: 61, Percentile: 87},
				{RawMin: 10, RawMax: 10, MinAgeMonths: 60, MaxAgeMonths: 95, Gender: "female", TScore: 55, Percentile: 69},
				{RawMin: 10, RawMax: 10, TScore: 50, Percentile: 50},
			},
		}},
	}

	score, ok := calcnorm.LookupNormScore(tables, "gec", 10, calcnorm.Subject{AgeMonths: 72, Gender: "female"})
	if !ok || score.TScore != 55 || score.Percentile != 69 {
		t.Fatalf("female lookup = %#v, ok = %v", score, ok)
	}
	score, ok = calcnorm.LookupNormScore(tables, "gec", 10, calcnorm.Subject{AgeMonths: 120, Gender: "female"})
	if !ok || score.TScore != 50 || score.Percentile != 50 {
		t.Fatalf("generic fallback = %#v, ok = %v", score, ok)
	}
}

func TestLookupNormScoreParametricBand(t *testing.T) {
	t.Parallel()

	mean := 10.0
	std := 2.0
	tables := &calcnorm.NormTables{
		Factors: []calcnorm.FactorNormTable{{
			FactorCode: "bri",
			Bands: []calcnorm.NormBand{{
				MinAgeMonths: 60,
				MaxAgeMonths: 120,
				Mean:         &mean,
				StdDev:       &std,
			}},
		}},
	}
	score, ok := calcnorm.LookupNormScore(tables, "bri", 12, calcnorm.Subject{AgeMonths: 72})
	if !ok || score.TScore != 60 {
		t.Fatalf("score = %#v, ok = %v", score, ok)
	}
}

func TestInterpretTScore(t *testing.T) {
	t.Parallel()

	tables := &calcnorm.NormTables{
		TScoreRules: []calcnorm.TScoreInterpretRule{{
			FactorCode: "gec",
			Ranges: []calcnorm.TScoreRange{
				{MinT: 0, MaxT: 59, Level: "low", Conclusion: "正常"},
				{MinT: 60, MaxT: 100, Level: "elevated", Conclusion: "升高"},
			},
		}},
	}
	level, conclusion, _, ok := calcnorm.InterpretTScore(tables, "gec", 65)
	if !ok || level != "elevated" || conclusion != "升高" {
		t.Fatalf("interpret = %q %q ok=%v", level, conclusion, ok)
	}
}
