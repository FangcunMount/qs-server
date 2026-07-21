package norm_test

import (
	"errors"
	"reflect"
	"testing"

	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
)

func TestResolveNormScoreGoldenMatches(t *testing.T) {
	t.Parallel()

	mean, std := 10.0, 2.0
	tables := &calcnorm.NormTables{Factors: []calcnorm.FactorNormTable{{
		FactorCode: "total",
		Lookup: []calcnorm.NormLookupEntry{
			{RawMin: 10, RawMax: 10, MinAgeMonths: 60, MaxAgeMonths: 95, Gender: "female", TScore: 65, Percentile: 92},
			{RawMin: 10, RawMax: 10, TScore: 50, Percentile: 50},
		},
		Bands: []calcnorm.NormBand{{MinAgeMonths: 60, MaxAgeMonths: 95, Gender: "male", Mean: &mean, StdDev: &std}},
	}}}

	tests := []struct {
		name      string
		raw       float64
		subject   calcnorm.Subject
		wantT     float64
		wantMatch calcnorm.MatchKind
	}{
		{name: "age lower bound inclusive", raw: 10, subject: calcnorm.Subject{AgeMonths: knownAge(60), Gender: "female"}, wantT: 65, wantMatch: calcnorm.MatchKindSpecific},
		{name: "age upper bound inclusive", raw: 10, subject: calcnorm.Subject{AgeMonths: knownAge(95), Gender: "female"}, wantT: 65, wantMatch: calcnorm.MatchKindSpecific},
		{name: "explicit generic fallback for missing subject", raw: 10, subject: calcnorm.Subject{}, wantT: 50, wantMatch: calcnorm.MatchKindGeneric},
		{name: "direct generic precedes parametric specific", raw: 10, subject: calcnorm.Subject{AgeMonths: knownAge(72), Gender: "male"}, wantT: 50, wantMatch: calcnorm.MatchKindGeneric},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := calcnorm.ResolveNormScore(tables, "total", tt.raw, tt.subject)
			if err != nil {
				t.Fatalf("ResolveNormScore: %v", err)
			}
			if got.Score.TScore != tt.wantT || got.MatchKind != tt.wantMatch {
				t.Fatalf("resolution = %#v, want T=%v match=%s", got, tt.wantT, tt.wantMatch)
			}
		})
	}
}

func TestResolveNormScoreDistinguishesKnownZeroAgeFromUnknown(t *testing.T) {
	t.Parallel()

	tables := &calcnorm.NormTables{Factors: []calcnorm.FactorNormTable{{
		FactorCode: "infant",
		Lookup:     []calcnorm.NormLookupEntry{{RawMin: 0, RawMax: 3, MaxAgeMonths: 5, TScore: 55, Percentile: 70}},
	}}}
	known, err := calcnorm.ResolveNormScore(tables, "infant", 1, calcnorm.Subject{AgeMonths: knownAge(0)})
	if err != nil || known.MatchKind != calcnorm.MatchKindSpecific {
		t.Fatalf("known zero age = %#v, err=%v", known, err)
	}

	_, err = calcnorm.ResolveNormScore(tables, "infant", 1, calcnorm.Subject{})
	assertResolutionError(t, err, calcnorm.ErrorKindSubjectMissing, []string{"age_months"})
}

func TestResolveNormScoreGoldenFailures(t *testing.T) {
	t.Parallel()

	mean, std := 10.0, 2.0
	tests := []struct {
		name        string
		tables      *calcnorm.NormTables
		factorCode  string
		raw         float64
		subject     calcnorm.Subject
		wantKind    calcnorm.ErrorKind
		wantMissing []string
	}{
		{
			name: "missing fields are stable", factorCode: "total", raw: 10,
			tables:   &calcnorm.NormTables{Factors: []calcnorm.FactorNormTable{{FactorCode: "total", Lookup: []calcnorm.NormLookupEntry{{RawMin: 10, RawMax: 10, MinAgeMonths: 60, MaxAgeMonths: 95, Gender: "female", TScore: 60, Percentile: 84}}}}},
			wantKind: calcnorm.ErrorKindSubjectMissing, wantMissing: []string{"age_months", "gender"},
		},
		{
			name: "wrong gender is cohort miss", factorCode: "total", raw: 10,
			tables:  &calcnorm.NormTables{Factors: []calcnorm.FactorNormTable{{FactorCode: "total", Bands: []calcnorm.NormBand{{MinAgeMonths: 60, MaxAgeMonths: 95, Gender: "female", Mean: &mean, StdDev: &std}}}}},
			subject: calcnorm.Subject{AgeMonths: knownAge(72), Gender: "male"}, wantKind: calcnorm.ErrorKindCohortNotFound,
		},
		{
			name: "complete subject outside age cohort", factorCode: "total", raw: 10,
			tables:  &calcnorm.NormTables{Factors: []calcnorm.FactorNormTable{{FactorCode: "total", Bands: []calcnorm.NormBand{{MinAgeMonths: 60, MaxAgeMonths: 95, Gender: "female", Mean: &mean, StdDev: &std}}}}},
			subject: calcnorm.Subject{AgeMonths: knownAge(120), Gender: "female"}, wantKind: calcnorm.ErrorKindCohortNotFound,
		},
		{
			name: "direct raw score miss", factorCode: "total", raw: 11,
			tables:   &calcnorm.NormTables{Factors: []calcnorm.FactorNormTable{{FactorCode: "total", Lookup: []calcnorm.NormLookupEntry{{RawMin: 0, RawMax: 10, TScore: 50, Percentile: 50}}}}},
			wantKind: calcnorm.ErrorKindRawScoreOutOfRange,
		},
		{name: "nil table is invalid", factorCode: "total", raw: 1, wantKind: calcnorm.ErrorKindInvalid},
		{
			name: "invalid band parameters", factorCode: "total", raw: 1,
			tables:   &calcnorm.NormTables{Factors: []calcnorm.FactorNormTable{{FactorCode: "total", Bands: []calcnorm.NormBand{{Mean: &mean}}}}},
			wantKind: calcnorm.ErrorKindInvalid,
		},
		{
			name: "missing factor is invalid", factorCode: "missing", raw: 1,
			tables:   &calcnorm.NormTables{Factors: []calcnorm.FactorNormTable{{FactorCode: "total", Lookup: []calcnorm.NormLookupEntry{{RawMin: 0, RawMax: 10, TScore: 50, Percentile: 50}}}}},
			wantKind: calcnorm.ErrorKindInvalid,
		},
		{
			name: "ambiguous lookup rows are invalid", factorCode: "total", raw: 5,
			tables: &calcnorm.NormTables{Factors: []calcnorm.FactorNormTable{{
				FactorCode: "total",
				Lookup: []calcnorm.NormLookupEntry{
					{RawMin: 0, RawMax: 10, MinAgeMonths: 60, MaxAgeMonths: 95, TScore: 50, Percentile: 50},
					{RawMin: 5, RawMax: 15, MinAgeMonths: 70, MaxAgeMonths: 100, TScore: 55, Percentile: 70},
				},
			}}},
			wantKind: calcnorm.ErrorKindInvalid,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := calcnorm.ResolveNormScore(tt.tables, tt.factorCode, tt.raw, tt.subject)
			assertResolutionError(t, err, tt.wantKind, tt.wantMissing)
		})
	}
}

func assertResolutionError(t *testing.T, err error, kind calcnorm.ErrorKind, missing []string) {
	t.Helper()
	var resolutionErr *calcnorm.ResolutionError
	if !errors.As(err, &resolutionErr) {
		t.Fatalf("error = %T %v, want ResolutionError", err, err)
	}
	if resolutionErr.Kind != kind || !reflect.DeepEqual(resolutionErr.MissingFields, missing) {
		t.Fatalf("error = %#v, want kind=%s missing=%v", resolutionErr, kind, missing)
	}
}
