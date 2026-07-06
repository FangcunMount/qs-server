package result

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evaluationtypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/typology"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestModelIdentityFromOutcomeMapsLegacyMBTIToPersonalityTypology(t *testing.T) {
	modelRef := assessment.NewEvaluationModelRefWithIdentity(
		assessment.EvaluationModelKindPersonality,
		modelcatalog.SubKindTypology,
		modelcatalog.AlgorithmMBTI,
		meta.ID(0),
		meta.NewCode("MBTI_TEST"),
		"1.0.0",
		"MBTI",
	)
	outcome := NewOutcomeFromLegacyResult(nil, nil, &assessment.EvaluationResult{
		ModelRef: modelRef,
		Summary:  assessment.ResultSummary{PrimaryLabel: "INTJ"},
		Detail: assessment.EvaluationDetail{
			Kind: assessment.EvaluationModelKindPersonality,
			Payload: evaluationtypology.MBTIResultDetail{
				TypeCode:     "INTJ",
				MatchPercent: 40,
			},
		},
	})
	outcome.Execution.Primary = &assessment.OutcomeScoreValue{
		Kind:  assessment.OutcomeScoreKindMatchPercent,
		Value: 40,
		Label: "INTJ",
	}
	outcome.Execution.Level = &assessment.OutcomeResultLevel{
		Code:     "INTJ",
		Label:    "INTJ",
		Severity: "none",
	}
	identity := modelIdentityFromOutcome(outcome)
	if identity.Kind != "personality" || identity.SubKind != "typology" || identity.Algorithm != "mbti" {
		t.Fatalf("identity = %#v", identity)
	}
	primary := primaryScoreFromOutcome(outcome)
	if primary == nil || primary.Kind != domainreport.ScoreKindMatchPercent || primary.Value != 40 {
		t.Fatalf("primary = %#v", primary)
	}
	level := levelFromOutcome(outcome)
	if level == nil || level.Code != "INTJ" {
		t.Fatalf("level = %#v", level)
	}
}
