package definition_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
)

func TestDefinitionJSONRoundTripPreservesTaggedConclusions(t *testing.T) {
	t.Parallel()

	want := definition.Definition{Conclusions: []conclusion.Conclusion{
		conclusion.RiskConclusion{FactorCode: "risk", Rules: []conclusion.ScoreRangeOutcome{{MinScore: 0, MaxScore: 1}}},
		conclusion.NormConclusion{FactorCode: "norm", ScoreBasis: conclusion.ScoreBasisTScore, Primary: true},
		conclusion.AbilityConclusion{FactorCode: "ability", ScoreBasis: conclusion.ScoreBasisRaw},
		conclusion.TypeConclusion{FactorCodes: []string{"type"}, Decision: conclusion.TypeDecision{Kind: binding.DecisionKindPoleComposition}},
	}}
	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"Kind":"risk"`) || !strings.Contains(string(encoded), `"Kind":"type"`) {
		t.Fatalf("canonical DefinitionV2 JSON must tag conclusions: %s", encoded)
	}
	var got definition.Definition
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Definition JSON round trip = %#v, want %#v", got, want)
	}
}

func TestDefinitionJSONRejectsUntaggedConclusion(t *testing.T) {
	t.Parallel()

	var value definition.Definition
	err := json.Unmarshal([]byte(`{"Conclusions":[{"FactorCode":"total"}]}`), &value)
	if err == nil || !strings.Contains(err.Error(), "conclusions[0].Kind") {
		t.Fatalf("Unmarshal error = %v, want conclusion kind error", err)
	}
}
