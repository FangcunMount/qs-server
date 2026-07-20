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
	if score.Reference.MinAgeMonths != 60 || score.Reference.MaxAgeMonths != 95 || score.Reference.Gender != "female" {
		t.Fatalf("selected norm reference = %#v", score.Reference)
	}
	score, ok = calcnorm.LookupNormScore(tables, "gec", 10, calcnorm.Subject{AgeMonths: 120, Gender: "female"})
	if !ok || score.TScore != 50 || score.Percentile != 50 {
		t.Fatalf("generic fallback = %#v, ok = %v", score, ok)
	}
	if score.Reference != (calcnorm.NormReference{}) {
		t.Fatalf("generic norm reference = %#v, want empty cohort", score.Reference)
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

func TestLookupNormScoreParametricRejectsMissingDemographics(t *testing.T) {
	t.Parallel()

	mean := 10.0
	std := 2.0
	tables := &calcnorm.NormTables{
		Factors: []calcnorm.FactorNormTable{{
			FactorCode: "bri",
			Bands: []calcnorm.NormBand{{
				MinAgeMonths: 60,
				MaxAgeMonths: 120,
				Gender:       "female",
				Mean:         &mean,
				StdDev:       &std,
			}},
		}},
	}

	if _, ok := calcnorm.LookupNormScore(tables, "bri", 12, calcnorm.Subject{}); ok {
		t.Fatal("empty subject must not match demographic band")
	}
	if _, ok := calcnorm.LookupNormScore(tables, "bri", 12, calcnorm.Subject{AgeMonths: 72}); ok {
		t.Fatal("missing gender must not match gender-scoped band")
	}
	if _, ok := calcnorm.LookupNormScore(tables, "bri", 12, calcnorm.Subject{Gender: "female"}); ok {
		t.Fatal("missing age must not match age-scoped band")
	}
	if _, ok := calcnorm.LookupNormScore(tables, "bri", 12, calcnorm.Subject{AgeMonths: 40, Gender: "female"}); ok {
		t.Fatal("below age lower bound must not match")
	}
	if _, ok := calcnorm.LookupNormScore(tables, "bri", 12, calcnorm.Subject{AgeMonths: 130, Gender: "female"}); ok {
		t.Fatal("above age upper bound must not match")
	}
	if _, ok := calcnorm.LookupNormScore(tables, "bri", 12, calcnorm.Subject{AgeMonths: 72, Gender: "male"}); ok {
		t.Fatal("gender mismatch must not match")
	}

	score, ok := calcnorm.LookupNormScore(tables, "bri", 12, calcnorm.Subject{AgeMonths: 72, Gender: "female"})
	if !ok || score.TScore != 60 {
		t.Fatalf("matching subject = %#v, ok = %v", score, ok)
	}
	if score.Reference.MinAgeMonths != 60 || score.Reference.MaxAgeMonths != 120 || score.Reference.Gender != "female" {
		t.Fatalf("selected band reference = %#v", score.Reference)
	}
}

func TestLookupNormScoreParametricFallsBackToGenericBand(t *testing.T) {
	t.Parallel()

	specificMean, specificStd := 8.0, 2.0
	genericMean, genericStd := 10.0, 2.0
	tables := &calcnorm.NormTables{
		Factors: []calcnorm.FactorNormTable{{
			FactorCode: "bri",
			Bands: []calcnorm.NormBand{
				{MinAgeMonths: 60, MaxAgeMonths: 120, Gender: "female", Mean: &specificMean, StdDev: &specificStd},
				{Mean: &genericMean, StdDev: &genericStd},
			},
		}},
	}

	score, ok := calcnorm.LookupNormScore(tables, "bri", 12, calcnorm.Subject{})
	if !ok || score.TScore != 60 {
		t.Fatalf("generic fallback = %#v, ok = %v", score, ok)
	}
	if score.Reference != (calcnorm.NormReference{}) {
		t.Fatalf("generic band reference = %#v, want empty cohort", score.Reference)
	}

	score, ok = calcnorm.LookupNormScore(tables, "bri", 12, calcnorm.Subject{AgeMonths: 72, Gender: "female"})
	if !ok || score.TScore != 70 {
		t.Fatalf("demographic band preferred = %#v, ok = %v", score, ok)
	}
}

func TestInterpretTScore(t *testing.T) {
	t.Parallel()

	tables := &calcnorm.NormTables{
		TScoreRules: []calcnorm.TScoreInterpretRule{{
			FactorCode: "gec",
			Ranges: []calcnorm.TScoreRange{
				{MinT: 0, MaxT: 60, Level: "low", Conclusion: "正常"},
				{MinT: 60, MaxT: 100, Level: "elevated", Conclusion: "升高"},
			},
		}},
	}
	level, conclusion, _, ok := calcnorm.InterpretTScore(tables, "gec", 65)
	if !ok || level != "elevated" || conclusion != "升高" {
		t.Fatalf("interpret = %q %q ok=%v", level, conclusion, ok)
	}
	level, conclusion, _, ok = calcnorm.InterpretTScore(tables, "gec", 60)
	if !ok || level != "elevated" {
		t.Fatalf("boundary 60 = %q %q ok=%v, want elevated via half-open adjacency", level, conclusion, ok)
	}
	level, conclusion, _, ok = calcnorm.InterpretTScore(tables, "gec", 100)
	if !ok || level != "elevated" {
		t.Fatalf("upper bound 100 = %q %q ok=%v, want elevated via last-range inclusive max", level, conclusion, ok)
	}
}
