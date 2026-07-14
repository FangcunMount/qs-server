package norm_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
)

func TestValidateImportAcceptsVersionedLookupTable(t *testing.T) {
	table := validTable()
	if err := norm.ValidateImport(table); err != nil {
		t.Fatalf("ValidateImport() error = %v", err)
	}
}

func TestValidateImportRejectsOverlappingRows(t *testing.T) {
	table := validTable()
	table.Factors[0].Lookup = append(table.Factors[0].Lookup, norm.LookupEntry{RawScoreMin: 10, RawScoreMax: 12, TScore: 60, Percentile: 84})
	if err := norm.ValidateImport(table); err == nil {
		t.Fatal("ValidateImport() error = nil, want overlapping range error")
	}
}

func TestValidateImportRejectsIncompatibleIdentity(t *testing.T) {
	table := validTable()
	table.Kind = identity.KindCognitive
	if err := norm.ValidateImport(table); err == nil {
		t.Fatal("ValidateImport() error = nil, want identity error")
	}
}

func TestValidateImportRejectsRavenSPMNorms(t *testing.T) {
	table := validTable()
	table.Kind = identity.KindCognitive
	table.Algorithm = identity.AlgorithmSPM
	if err := norm.ValidateImport(table); err == nil {
		t.Fatal("ValidateImport() error = nil, want Raven SPM identity error")
	}
}

func TestValidateImportAllowsGenericLookupFallbackWithSpecificRows(t *testing.T) {
	table := validTable()
	table.Factors[0].Lookup = append(table.Factors[0].Lookup, norm.LookupEntry{RawScoreMin: 10, RawScoreMax: 10, MinAgeMonths: 60, MaxAgeMonths: 95, Gender: "female", TScore: 60, Percentile: 84})
	if err := norm.ValidateImport(table); err != nil {
		t.Fatalf("ValidateImport() error = %v", err)
	}
}

func TestValidateImportRejectsAmbiguousDemographicLookups(t *testing.T) {
	table := validTable()
	table.Factors[0].Lookup = []norm.LookupEntry{
		{RawScoreMin: 10, RawScoreMax: 10, MinAgeMonths: 60, MaxAgeMonths: 95, Gender: "female", TScore: 55, Percentile: 69},
		{RawScoreMin: 10, RawScoreMax: 10, MinAgeMonths: 72, MaxAgeMonths: 120, Gender: "female", TScore: 60, Percentile: 84},
	}
	if err := norm.ValidateImport(table); err == nil {
		t.Fatal("ValidateImport() error = nil, want ambiguous demographic lookup error")
	}
}

func TestValidateImportRejectsOverlappingParametricBands(t *testing.T) {
	mean, stdDev := 10.0, 2.0
	table := validTable()
	table.Factors[0].Lookup = nil
	table.Factors[0].Bands = []norm.Band{
		{MinAgeMonths: 60, MaxAgeMonths: 95, Gender: "female", Mean: &mean, StdDev: &stdDev},
		{MinAgeMonths: 90, MaxAgeMonths: 120, Gender: "female", Mean: &mean, StdDev: &stdDev},
	}
	if err := norm.ValidateImport(table); err == nil {
		t.Fatal("ValidateImport() error = nil, want overlapping band error")
	}
}

func validTable() *norm.Norm {
	return &norm.Norm{
		TableVersion: "brief2-parent-2026", FormVariant: "parent",
		Kind: identity.KindBehavioralRating, Algorithm: identity.AlgorithmBrief2,
		Factors: []norm.FactorTable{{FactorCode: "gec", Lookup: []norm.LookupEntry{{RawScoreMin: 10, RawScoreMax: 10, TScore: 55, Percentile: 69}}}},
	}
}
