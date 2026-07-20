package definition_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
)

func TestCanonicalContentHashIgnoresDerivedLayers(t *testing.T) {
	t.Parallel()
	base := &definition.Definition{
		Outcomes: []conclusion.Outcome{{Code: "low", Title: "低", Summary: "摘要"}},
		Conclusions: []conclusion.Conclusion{conclusion.RiskConclusion{
			FactorCode: "total",
			Rules:      []conclusion.ScoreRangeOutcome{{MinScore: 0, MaxScore: 10, MaxInclusive: true, OutcomeCode: "low"}},
		}},
	}
	definition.MaterializeLayers(base)
	hashA, err := definition.CanonicalContentHash(base)
	if err != nil || hashA == "" {
		t.Fatalf("hashA = %q err=%v", hashA, err)
	}
	base.InterpretationAssets.Outcomes[0].Summary = "改写"
	hashB, err := definition.CanonicalContentHash(base)
	if err != nil || hashA != hashB {
		t.Fatalf("derived layer rewrite changed hash: %q vs %q", hashA, hashB)
	}
	base.Conclusions[0] = conclusion.RiskConclusion{
		FactorCode: "total",
		Rules:      []conclusion.ScoreRangeOutcome{{MinScore: 0, MaxScore: 20, MaxInclusive: true, OutcomeCode: "low"}},
	}
	hashC, err := definition.CanonicalContentHash(base)
	if err != nil || hashA == hashC {
		t.Fatalf("authoring change must change hash: %q vs %q", hashA, hashC)
	}
}

func TestPayloadProjectionHash(t *testing.T) {
	t.Parallel()
	a := definition.PayloadProjectionHash([]byte(`{"code":"A"}`))
	b := definition.PayloadProjectionHash([]byte(`{"code":"B"}`))
	if a == "" || a == b {
		t.Fatalf("hashes = %q %q", a, b)
	}
}
