package behavioral_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
)

func TestSnapshotFromDefinitionPreservesNormLookupContract(t *testing.T) {
	t.Parallel()

	standardScore := 108.0
	table := &norm.Norm{
		TableVersion: "brief2-parent-2026",
		FormVariant:  "parent",
		Kind:         identity.KindBehavioralRating,
		Algorithm:    identity.AlgorithmBrief2,
		Factors: []norm.FactorTable{{
			FactorCode: "gec",
			Lookup: []norm.LookupEntry{{
				RawScoreMin: 10, RawScoreMax: 10,
				MinAgeMonths: 60, MaxAgeMonths: 95, Gender: "female",
				TScore: 55, Percentile: 69, StandardScore: &standardScore,
			}},
		}},
	}
	def := &definition.Definition{
		Calibration: definition.Calibration{NormRefs: []norm.Ref{{FactorCode: "gec", NormTableVersion: table.TableVersion}}},
		Conclusions: []conclusion.Conclusion{conclusion.NormConclusion{FactorCode: "gec", ScoreBasis: conclusion.ScoreBasisTScore, Primary: true}},
	}

	snapshot, err := behavioral.SnapshotFromDefinition(
		behavioral.DefinitionEnvelope{Code: "BRIEF2", Version: "1", Status: "published"},
		def,
		map[string]*norm.Norm{table.TableVersion: table},
	)
	if err != nil {
		t.Fatalf("SnapshotFromDefinition: %v", err)
	}
	if snapshot.Norming == nil || snapshot.Norming.NormTables == nil || len(snapshot.Norming.NormTables.Factors) != 1 {
		t.Fatalf("norming = %#v", snapshot.Norming)
	}
	if len(snapshot.Norming.RequiredFactorCodes) != 1 || snapshot.Norming.RequiredFactorCodes[0] != "gec" {
		t.Fatalf("required factors = %#v", snapshot.Norming.RequiredFactorCodes)
	}
	rows := snapshot.Norming.NormTables.Factors[0].Lookup
	if len(rows) != 1 {
		t.Fatalf("lookup rows = %#v", rows)
	}
	got := rows[0]
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
