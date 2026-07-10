package modelcatalog

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
)

func TestNormPORoundTripPreservesReferenceData(t *testing.T) {
	mean := 50.0
	stdDev := 10.0
	original := &norm.Norm{
		TableVersion: "brief2-parent-2024",
		FormVariant:  "parent",
		Factors: []norm.FactorTable{{
			FactorCode: "gec",
			Bands:      []norm.Band{{MinAgeMonths: 60, MaxAgeMonths: 72, Gender: "female", Mean: &mean, StdDev: &stdDev}},
			Lookup:     []norm.LookupEntry{{RawScoreMin: 0, RawScoreMax: 10, TScore: 50, Percentile: 50}},
		}},
	}

	got := normFromPO(normToPO(original))
	if got == nil || got.TableVersion != original.TableVersion || got.FormVariant != original.FormVariant {
		t.Fatalf("norm = %#v", got)
	}
	if len(got.Factors) != 1 || len(got.Factors[0].Bands) != 1 || len(got.Factors[0].Lookup) != 1 {
		t.Fatalf("norm factors = %#v", got.Factors)
	}
	if got.Factors[0].Bands[0].Mean == original.Factors[0].Bands[0].Mean || *got.Factors[0].Bands[0].Mean != mean {
		t.Fatalf("band mean = %#v", got.Factors[0].Bands[0].Mean)
	}
}
