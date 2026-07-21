package evaluationinput_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	cognitivepayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/cognitive"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

func TestAbilityConclusionsFromSnapshotPrefersDefinition(t *testing.T) {
	t.Parallel()
	def := &modeldefinition.Definition{
		Conclusions: []conclusion.Conclusion{conclusion.AbilityConclusion{
			FactorCode: "total", Primary: true,
			Rules: []conclusion.ScoreRangeOutcome{{MinScore: 10, MaxScore: 20, Level: "high"}},
		}},
	}
	snapshot := &evaluationinput.InputSnapshot{
		DefinitionV2: def,
		ModelPayload: evaluationinput.CognitiveModelPayload{Snapshot: &cognitivepayload.Snapshot{}},
	}
	rules := evaluationinput.AbilityConclusionsFromSnapshot(snapshot)
	if len(rules) != 1 || rules[0].FactorCode != "total" {
		t.Fatalf("rules = %#v", rules)
	}
}

func TestCognitiveExecutionSnapshotRejectsPayloadOnlyInput(t *testing.T) {
	t.Parallel()
	input := &evaluationinput.InputSnapshot{ModelPayload: evaluationinput.CognitiveModelPayload{Snapshot: &cognitivepayload.Snapshot{}}}
	if _, ok := evaluationinput.CognitiveExecutionSnapshot(input); ok {
		t.Fatal("payload-only cognitive input was accepted")
	}
}

func TestAuditRuntimeInputSourceDetectsCompatFallback(t *testing.T) {
	t.Parallel()
	issues := evaluationinput.AuditRuntimeInputSource(&evaluationinput.InputSnapshot{
		ModelPayload: evaluationinput.ScaleModelPayload{Scale: &scalesnapshot.ScaleSnapshot{Code: "PHQ9"}},
	})
	if len(issues) == 0 || issues[len(issues)-1].Code != "runtime.definition_v2_missing" {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAuditInputIdentityFlagsUnsupportedAlgorithm(t *testing.T) {
	t.Parallel()
	issues := evaluationinput.AuditInputIdentity(&evaluationinput.InputSnapshot{
		Model: &evaluationinput.ModelSnapshot{
			Kind: evaluationinput.EvaluationModelKindTypology, Algorithm: "unsupported",
		},
	})
	if len(issues) == 0 || issues[0].Code != "identity.algorithm.unknown" {
		t.Fatalf("issues = %#v", issues)
	}
}
