package normruntime_test

import (
	"testing"

	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/normruntime"
)

func TestFromCatalogLosslesslyDeepCopiesRuntimeMaterial(t *testing.T) {
	t.Parallel()
	mean, stdDev, standard := 10.5, 2.25, 101.0
	table := &modelnorm.Norm{
		TableVersion: "brief2-parent-2026", FormVariant: "parent",
		Kind: modelcatalog.KindBehavioralRating, Algorithm: modelcatalog.AlgorithmBrief2,
		Factors: []modelnorm.FactorTable{{
			FactorCode: "gec",
			Bands:      []modelnorm.Band{{MinAgeMonths: 60, MaxAgeMonths: 95, Gender: "female", Mean: &mean, StdDev: &stdDev}},
			Lookup: []modelnorm.LookupEntry{{
				RawScoreMin: 1, RawScoreMax: 9, MinAgeMonths: 96, MaxAgeMonths: 120, Gender: "male",
				TScore: 55, Percentile: 69, StandardScore: &standard,
			}},
		}},
	}

	got, err := normruntime.FromCatalog(table)
	if err != nil {
		t.Fatal(err)
	}
	if got.NormTableVersion != table.TableVersion || got.FormVariant != table.FormVariant || len(got.Factors) != 1 {
		t.Fatalf("header/factors = %#v", got)
	}
	band := got.Factors[0].Bands[0]
	if band.MinAgeMonths != 60 || band.MaxAgeMonths != 95 || band.Gender != "female" || band.Mean == nil || *band.Mean != 10.5 || band.StdDev == nil || *band.StdDev != 2.25 {
		t.Fatalf("band = %#v", band)
	}
	lookup := got.Factors[0].Lookup[0]
	if lookup.RawMin != 1 || lookup.RawMax != 9 || lookup.MinAgeMonths != 96 || lookup.MaxAgeMonths != 120 || lookup.Gender != "male" || lookup.TScore != 55 || lookup.Percentile != 69 || lookup.StandardScore == nil || *lookup.StandardScore != 101 {
		t.Fatalf("lookup = %#v", lookup)
	}
	if band.Mean == table.Factors[0].Bands[0].Mean || band.StdDev == table.Factors[0].Bands[0].StdDev || lookup.StandardScore == table.Factors[0].Lookup[0].StandardScore {
		t.Fatal("runtime projection aliases source pointers")
	}
	mean, stdDev, standard = 99, 99, 99
	if *band.Mean != 10.5 || *band.StdDev != 2.25 || *lookup.StandardScore != 101 {
		t.Fatal("runtime projection changed after source mutation")
	}
}

func TestFromCatalogRejectsNormWithoutCanonicalIdentity(t *testing.T) {
	t.Parallel()
	table := &modelnorm.Norm{
		TableVersion: "legacy", FormVariant: "parent",
		Factors: []modelnorm.FactorTable{{FactorCode: "gec", Lookup: []modelnorm.LookupEntry{{RawScoreMin: 1, RawScoreMax: 1, TScore: 50, Percentile: 50}}}},
	}
	_, err := normruntime.FromCatalog(table)
	if kind, ok := calcnorm.ErrorKindOf(err); !ok || kind != calcnorm.ErrorKindInvalid {
		t.Fatalf("err = %v, kind = %q, want norm_invalid", err, kind)
	}
}
