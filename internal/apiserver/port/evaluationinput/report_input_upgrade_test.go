package evaluationinput_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	scalesnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/scale"
)

func TestTryUpgradeReportInputToV3FromV2(t *testing.T) {
	t.Parallel()
	assets := &interpretationassets.Assets{Outcomes: []interpretationassets.OutcomePresentation{{
		OutcomeCode: "low", Summary: "偏低",
	}}}
	v2, err := evaluationinput.MarshalReportInput(evaluationinput.ReportInputFreezeOptions{
		Payload: evaluationinput.ScaleModelPayload{Scale: &scalesnapshot.ScaleSnapshot{
			Code: "PHQ9", Factors: []scalesnapshot.FactorSnapshot{{Code: "TOTAL", IsTotalScore: true}},
		}},
		Assets: assets,
	})
	if err != nil {
		t.Fatal(err)
	}
	modelRef := evaluationinput.ModelRef{Kind: evaluationinput.EvaluationModelKindScale, Code: "PHQ9", Version: "v1"}
	upgrade, err := evaluationinput.TryUpgradeReportInputToV3(v2, modelRef, modelcatalog.AlgorithmFamilyFactorScoring)
	if err != nil {
		t.Fatal(err)
	}
	if upgrade.Skipped != "" || upgrade.ToSchema != evaluationinput.ReportInputSchemaV3 {
		t.Fatalf("upgrade = %#v", upgrade)
	}
	if evaluationinput.ReportInputSchema(upgrade.Data) != evaluationinput.ReportInputSchemaV3 {
		t.Fatalf("schema = %d", evaluationinput.ReportInputSchema(upgrade.Data))
	}
}

func TestTryUpgradeReportInputToV3SkipsLegacyWithoutAssets(t *testing.T) {
	t.Parallel()
	raw, err := evaluationinput.MarshalReportInput(evaluationinput.ReportInputFreezeOptions{
		Payload: evaluationinput.ScaleModelPayload{Scale: &scalesnapshot.ScaleSnapshot{Code: "PHQ9"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	upgrade, err := evaluationinput.TryUpgradeReportInputToV3(raw, evaluationinput.ModelRef{Kind: evaluationinput.EvaluationModelKindScale, Code: "PHQ9"}, modelcatalog.AlgorithmFamilyFactorScoring)
	if err != nil {
		t.Fatal(err)
	}
	if upgrade.Skipped != "insufficient_minimal_snapshot" {
		t.Fatalf("upgrade = %#v", upgrade)
	}
}

func TestAuditReportInputDetectsLegacyPayloadOnly(t *testing.T) {
	t.Parallel()
	raw, err := evaluationinput.MarshalReportInput(evaluationinput.ReportInputFreezeOptions{
		Payload: evaluationinput.ScaleModelPayload{Scale: &scalesnapshot.ScaleSnapshot{Code: "PHQ9"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	issues := evaluationinput.AuditReportInput(raw, evaluationinput.ModelRef{Kind: evaluationinput.EvaluationModelKindScale, Code: "PHQ9"})
	if len(issues) == 0 || issues[0].Code != "report_input.legacy_payload_only" {
		t.Fatalf("issues = %#v", issues)
	}
}
