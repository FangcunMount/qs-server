package evaluationinput_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
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

func TestTryUpgradeReportInputToV3WithDefinitionFromLegacyPayloadOnly(t *testing.T) {
	t.Parallel()
	raw, err := evaluationinput.MarshalReportInput(evaluationinput.ReportInputFreezeOptions{
		Payload: evaluationinput.ScaleModelPayload{Scale: &scalesnapshot.ScaleSnapshot{
			Code: "PHQ9", Factors: []scalesnapshot.FactorSnapshot{{Code: "TOTAL", IsTotalScore: true}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	def := &modeldefinition.Definition{
		Measure: modeldefinition.MeasureSpec{
			Factors: []factor.Factor{{Code: "TOTAL", Title: "总分", Role: factor.FactorRoleTotal}},
		},
		Outcomes: []conclusion.Outcome{{Code: "low", Summary: "偏低"}},
	}
	modeldefinition.MaterializeLayers(def)
	modelRef := evaluationinput.ModelRef{Kind: evaluationinput.EvaluationModelKindScale, Code: "PHQ9", Version: "v1"}
	upgrade, err := evaluationinput.TryUpgradeReportInputToV3WithDefinition(raw, modelRef, modelcatalog.AlgorithmFamilyFactorScoring, def)
	if err != nil {
		t.Fatal(err)
	}
	if upgrade.Skipped != "" || upgrade.ToSchema != evaluationinput.ReportInputSchemaV3 {
		t.Fatalf("upgrade = %#v", upgrade)
	}
}

func TestAuditRuntimeInputSourceDetectsCompatFallback(t *testing.T) {
	t.Parallel()
	issues := evaluationinput.AuditRuntimeInputSource(&evaluationinput.InputSnapshot{
		ModelPayload: evaluationinput.ScaleModelPayload{Scale: &scalesnapshot.ScaleSnapshot{Code: "PHQ9"}},
	})
	if len(issues) == 0 || issues[0].Code != "runtime.compat_payload_only" {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestAuditLegacyIdentityFlagsRetainedAlgorithm(t *testing.T) {
	t.Parallel()
	issues := evaluationinput.AuditLegacyIdentity(&evaluationinput.InputSnapshot{
		Model: &evaluationinput.ModelSnapshot{
			Kind: evaluationinput.EvaluationModelKindTypology, Algorithm: string(modelcatalog.AlgorithmMBTI),
		},
	})
	if len(issues) == 0 || issues[0].Code != "identity.algorithm.unknown" {
		t.Fatalf("issues = %#v", issues)
	}
	_ = interpretationassets.Assets{}
}
