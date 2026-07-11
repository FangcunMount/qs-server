package reporting_test

import (
	"testing"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	typologylegacy "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology/legacy"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestAttachReportOutcomeSummaryMapsLegacyMBTIToPersonalityTypology(t *testing.T) {
	modelRef := assessment.NewEvaluationModelRefWithIdentity(
		assessment.EvaluationModelKindPersonality,
		modelcatalog.SubKindTypology,
		modelcatalog.AlgorithmMBTI,
		meta.ID(0),
		meta.NewCode("MBTI_TEST"),
		"1.0.0",
		"MBTI",
	)
	o := evaloutcome.Outcome{Execution: domainoutcome.NewExecution(
		evaloutcome.ModelRefFromAssessment(modelRef),
		domainoutcome.Summary{PrimaryLabel: "INTJ"},
		domainoutcome.Detail{Kind: modelcatalog.KindTypology, Payload: typologylegacy.MBTIResultDetail{
			TypeCode:     "INTJ",
			MatchPercent: 40,
		}},
	)}
	o.Execution.Primary = &domainoutcome.ScoreValue{
		Kind:  domainoutcome.ScoreKindMatchPercent,
		Value: 40,
		Label: "INTJ",
	}
	o.Execution.Level = &domainoutcome.ResultLevel{
		Code:     "INTJ",
		Label:    "INTJ",
		Severity: "none",
	}
	rpt := reporting.AttachReportOutcomeSummary(o, domainreport.NewInterpretReport(
		domainreport.ID(1),
		"MBTI",
		"MBTI_TEST",
		40,
		domainreport.RiskLevelNone,
		"INTJ",
		nil,
		nil,
		nil,
	))
	model := rpt.Model()
	if model.Kind != "typology" || model.SubKind != "typology" || model.Algorithm != "mbti" {
		t.Fatalf("model = %#v", model)
	}
	primary := rpt.PrimaryScore()
	if primary == nil || primary.Kind != domainreport.ScoreKindMatchPercent || primary.Value != 40 {
		t.Fatalf("primary = %#v", primary)
	}
	level := rpt.Level()
	if level == nil || level.Code != "INTJ" {
		t.Fatalf("level = %#v", level)
	}
}
