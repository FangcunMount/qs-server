package main

import (
	"encoding/json"
	"reflect"
	"testing"

	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/interpretationassets"
	evaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func TestIdentityRefPatternAcceptsOnlyCanonicalV2(t *testing.T) {
	t.Parallel()

	valid := "isn:v2:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if !identityRefPattern.MatchString(valid) {
		t.Fatalf("canonical v2 ref rejected: %s", valid)
	}
	for _, invalid := range []string{
		"isn:v1:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"model:brief2",
		"answersheet:42",
		"isn:v2:ABCDEF",
		"isn:v2:0123456789abcdef",
	} {
		if identityRefPattern.MatchString(invalid) {
			t.Fatalf("non-canonical ref accepted: %s", invalid)
		}
	}
}

func TestAuditOutcomeContractRowAcceptsCurrentNormativeOutcome(t *testing.T) {
	t.Parallel()

	modelRef := evaluationinput.ModelRef{
		Kind: evaluationinput.EvaluationModelKindBehavioralRating, Algorithm: string(modelcatalog.AlgorithmBrief2),
		Code: "BRIEF2", Version: "v1",
	}
	reportInput, err := evaluationinput.MarshalReportInput(evaluationinput.ReportInputFreezeOptions{
		ModelRef: modelRef,
		Assets: &interpretationassets.Assets{Outcomes: []interpretationassets.OutcomePresentation{{
			OutcomeCode: "normal", Title: "Normal",
		}}},
		DecisionKind:  modelcatalog.DecisionKindNormLookup,
		FactorCatalog: []evaluationinput.FactorCatalogEntry{{Code: "total", IsTotalScore: true}},
		Norming:       &evaluationinput.NormingFreeze{NormTables: &calcnorm.NormTables{NormTableVersion: "norm-v1"}},
	})
	if err != nil {
		t.Fatalf("MarshalReportInput: %v", err)
	}
	payload, err := json.Marshal(domainoutcome.Execution{Dimensions: []domainoutcome.DimensionResult{{
		Code: "total", NormReference: &domainoutcome.NormReference{TableVersion: "norm-v1"},
	}}})
	if err != nil {
		t.Fatalf("Marshal outcome: %v", err)
	}
	got := auditOutcomeContractRow(outcomeContractRow{
		SchemaVersion: domainoutcome.CurrentSchemaVersion,
		ModelKind:     string(modelcatalog.KindBehavioralRating), ModelAlgorithm: string(modelcatalog.AlgorithmBrief2),
		ModelCode: "BRIEF2", ModelVersion: "v1", DecisionKind: string(modelcatalog.DecisionKindNormLookup),
		ReportInputJSON: string(reportInput), PayloadJSON: string(payload),
	})
	if len(got) != 0 {
		t.Fatalf("findings = %v, want none", got)
	}
}

func TestAuditOutcomeContractRowClassifiesCurrentOnlyViolations(t *testing.T) {
	t.Parallel()

	got := auditOutcomeContractRow(outcomeContractRow{
		SchemaVersion: 1,
		ModelKind:     string(modelcatalog.KindBehavioralRating), ModelAlgorithm: string(modelcatalog.AlgorithmBrief2),
		ModelCode: "BRIEF2", ModelVersion: "v1", DecisionKind: string(modelcatalog.DecisionKindNormLookup),
		ReportInputJSON: `{"schema_version":2}`, PayloadJSON: `{}`,
	})
	want := []string{"schema_version.not_2", "report_input.invalid", "norm_reference.missing"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("findings = %v, want %v", got, want)
	}
}

func TestAuditIndexNamesClassifiesMissingAndLegacyIndexes(t *testing.T) {
	t.Parallel()

	got := auditIndexNames(
		"assessment_models",
		map[string]struct{}{"_id_": {}, "required_a": {}, "legacy_a": {}},
		[]string{"required_a", "required_b"},
		[]string{"legacy_a", "legacy_b"},
	)
	want := []indexAuditRule{
		{rule: "required.missing", sample: "assessment_models/required_b"},
		{rule: "legacy.present", sample: "assessment_models/legacy_a"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("findings = %#v, want %#v", got, want)
	}
}
