package cognitive_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/cognitive"
)

func TestNormTablesFromCatalogPreservesNormLookupContract(t *testing.T) {
	t.Parallel()

	standardScore := 108.0
	table := &norm.Norm{
		TableVersion: "spm-cn-2026",
		FormVariant:  "standard",
		Kind:         identity.KindCognitive,
		Algorithm:    identity.AlgorithmSPM,
		Factors: []norm.FactorTable{{
			FactorCode: "total",
			Lookup: []norm.LookupEntry{{
				RawScoreMin: 10, RawScoreMax: 10,
				MinAgeMonths: 60, MaxAgeMonths: 95, Gender: "female",
				TScore: 55, Percentile: 69, StandardScore: &standardScore,
			}},
		}},
	}

	tables, err := cognitive.NormTablesFromCatalog(table)
	if err != nil {
		t.Fatalf("NormTablesFromCatalog: %v", err)
	}
	if tables == nil || len(tables.Factors) != 1 || len(tables.Factors[0].Lookup) != 1 {
		t.Fatalf("NormTablesFromCatalog = %#v", tables)
	}
	got := tables.Factors[0].Lookup[0]
	if got.MinAgeMonths != 60 || got.MaxAgeMonths != 95 || got.Gender != "female" {
		t.Fatalf("demographic scope = %#v", got)
	}
	if got.StandardScore == nil || *got.StandardScore != standardScore {
		t.Fatalf("standard score = %#v", got.StandardScore)
	}
	if got.StandardScore == table.Factors[0].Lookup[0].StandardScore {
		t.Fatal("standard score pointer aliases catalog storage")
	}
}
