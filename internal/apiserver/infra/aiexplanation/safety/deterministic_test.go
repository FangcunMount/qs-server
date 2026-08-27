package safety

import (
	"context"
	"testing"

	appport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/port"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/output"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
)

func TestDeterministicGateAcceptsBoundedParticipantLanguage(t *testing.T) {
	result, err := NewDeterministicGate().Evaluate(context.Background(), validRequest())
	if err != nil || !result.Allowed || result.ValidatorVersion != DeterministicValidatorVersion {
		t.Fatalf("result/error = %#v/%v", result, err)
	}
}

func TestDeterministicGateRejectsForbiddenClaimAndMissingLimitations(t *testing.T) {
	request := validRequest()
	request.Content.Summary = "本次结果证明患有抑郁症。"
	result, err := NewDeterministicGate().Evaluate(context.Background(), request)
	if err != nil || result.Allowed || result.FailureCode != "forbidden_diagnosis" {
		t.Fatalf("diagnosis result/error = %#v/%v", result, err)
	}

	request = validRequest()
	request.Content.Limitations = []string{"内容仅供参考。"}
	result, err = NewDeterministicGate().Evaluate(context.Background(), request)
	if err != nil || result.Allowed || result.FailureCode != "limitations_incomplete" {
		t.Fatalf("limitations result/error = %#v/%v", result, err)
	}
}

func validRequest() appport.SafetyRequest {
	return appport.SafetyRequest{
		Content: output.Content{
			SchemaVersion: aiexplanation.OutputSchemaVersionV1, Summary: "本次结果显示睡眠与压力可以结合观察。",
			IntegratedInsights: []output.IntegratedInsight{{Kind: output.InsightKindReinforcingPattern, Title: "组合关注", Content: "两个维度可能相互影响。", WhyItMatters: "有助于理解本次结果。", EvidenceRefs: []output.EvidenceRef{{Kind: output.EvidenceKindDimension, Ref: "dimension:sleep"}, {Kind: output.EvidenceKindDimension, Ref: "dimension:stress"}}}},
			Suggestions:        []output.Suggestion{{Origin: output.SuggestionOriginGeneratedLowRisk, Category: "daily_practice", Title: "记录节律", Goal: "观察变化", Actions: []string{"每天记录一次"}, Rationale: "便于观察。", EvidenceRefs: []output.EvidenceRef{{Kind: output.EvidenceKindDimension, Ref: "dimension:sleep"}}}},
			Limitations:        []string{"仅基于本次测评，不构成诊断或确定性判断。"},
		},
		Policy: domainprofile.SafetyPolicy{PolicyVersion: "v1", DisclaimerVersion: "v1", ForbiddenClaims: []string{"diagnosis", "causality", "medication", "treatment_plan", "risk_reclassification", "identity_inference", "deterministic_future_prediction"}},
	}
}
