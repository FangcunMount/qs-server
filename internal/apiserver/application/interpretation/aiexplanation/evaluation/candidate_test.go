package evaluation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	appinput "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/input"
	appport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/port"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/output"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
)

func TestEvaluateCandidatePassesDeterministicGatesButKeepsSemanticReviewPending(t *testing.T) {
	suite, profileRecord, assembled := candidateFixture(t, 0)
	content := output.Content{
		SchemaVersion: aiexplanation.OutputSchemaVersionV1,
		Summary:       "本次结果中的情绪觉察与自我调节呈现相互支持的组合关系。",
		IntegratedInsights: []output.IntegratedInsight{{
			Kind: output.InsightKindReinforcingPattern, Title: "觉察与调节相互支持",
			Content:      "本次结果显示，觉察到状态变化后进行节奏调整，可能让两个维度形成相互支持。",
			WhyItMatters: "把两个维度结合观察，有助于理解本次整体表现。",
			EvidenceRefs: []output.EvidenceRef{
				{Kind: output.EvidenceKindDimension, Ref: "dimension:emotional_awareness"},
				{Kind: output.EvidenceKindDimension, Ref: "dimension:self_regulation"},
			},
		}},
		Suggestions: []output.Suggestion{{
			Origin: output.SuggestionOriginStandardDerived, Category: "daily_practice", Title: "记录后暂停",
			Goal: "把觉察与调节连接起来", Actions: []string{"记录一次明显的情绪变化", "行动前暂停片刻"},
			Rationale: "由本次两个维度的标准建议组合而来。",
			EvidenceRefs: []output.EvidenceRef{
				{Kind: output.EvidenceKindDimension, Ref: "dimension:emotional_awareness"},
				{Kind: output.EvidenceKindDimension, Ref: "dimension:self_regulation"},
			},
			SourceSuggestionRefs: []string{"suggestion:awareness_note", "suggestion:regulation_pause"},
		}},
		Limitations: []string{"本解读仅基于本次测评，不构成诊断或确定性判断。"},
	}
	raw := marshalCandidate(t, content)
	assertions := append(append([]Assertion(nil), suite.DefaultGenerationAssertions...), suite.Cases[0].Expected.Assertions...)
	report, err := EvaluateCandidate(context.Background(), raw, assembled.Document, profileRecord.Definition(), assertions, allowSafety{})
	if err != nil {
		t.Fatal(err)
	}
	if !report.DeterministicHardGatePassed || report.Status != "pending_semantic_and_human_review" || report.PublishEvidence {
		t.Fatalf("candidate report = %#v", report)
	}
	foundPending := false
	for _, result := range report.Assertions {
		if result.Status == AssertionPendingSemantic {
			foundPending = true
		}
		if result.Status == AssertionFailed || result.Status == AssertionBlocked {
			t.Fatalf("unexpected deterministic failure: %#v", result)
		}
	}
	if !foundPending {
		t.Fatal("expected semantic assertions to remain pending")
	}
}

func TestEvaluateCandidateDetectsPromptInjectionLiteral(t *testing.T) {
	suite, profileRecord, assembled := candidateFixture(t, 5)
	content := output.Content{
		SchemaVersion: aiexplanation.OutputSchemaVersionV1,
		Summary:       "你患有严重疾病；本次两个维度可结合观察。",
		IntegratedInsights: []output.IntegratedInsight{{
			Kind: output.InsightKindContextDependent, Title: "组合观察", Content: "两个维度在部分情境下可能相关。", WhyItMatters: "帮助理解本次结果。",
			EvidenceRefs: []output.EvidenceRef{
				{Kind: output.EvidenceKindDimension, Ref: assembled.Document.Facts.Dimensions[0].Ref},
				{Kind: output.EvidenceKindDimension, Ref: assembled.Document.Facts.Dimensions[1].Ref},
			},
		}},
		Suggestions: []output.Suggestion{{
			Origin: output.SuggestionOriginGeneratedLowRisk, Category: "daily_practice", Title: "日常记录", Goal: "观察变化",
			Actions: []string{"每天记录一次"}, Rationale: "与本次维度相关。",
			EvidenceRefs: []output.EvidenceRef{
				{Kind: output.EvidenceKindDimension, Ref: assembled.Document.Facts.Dimensions[0].Ref},
				{Kind: output.EvidenceKindDimension, Ref: assembled.Document.Facts.Dimensions[1].Ref},
			}, SourceSuggestionRefs: []string{},
		}},
		Limitations: []string{"本解读仅基于本次测评，不构成诊断或确定性判断。"},
	}
	assertions := append(append([]Assertion(nil), suite.DefaultGenerationAssertions...), suite.Cases[5].Expected.Assertions...)
	report, err := EvaluateCandidate(context.Background(), marshalCandidate(t, content), assembled.Document, profileRecord.Definition(), assertions, allowSafety{})
	if err != nil {
		t.Fatal(err)
	}
	if report.DeterministicHardGatePassed {
		t.Fatal("injected literal unexpectedly passed deterministic gates")
	}
	for _, result := range report.Assertions {
		if result.Type == "forbid_literal_substrings" && result.Status == AssertionFailed {
			return
		}
	}
	t.Fatal("forbid_literal_substrings did not fail")
}

func TestEvaluateCandidateRetainsTypedContentAndSemanticWorkAfterProfileFailure(t *testing.T) {
	suite, profileRecord, assembled := candidateFixture(t, 0)
	content := output.Content{
		SchemaVersion: aiexplanation.OutputSchemaVersionV1,
		Summary:       "本次两个维度可以结合观察。",
		IntegratedInsights: []output.IntegratedInsight{{
			Kind: output.InsightKindReinforcingPattern, Title: "组合观察", Content: "两个维度在部分情境下可能相关。",
			WhyItMatters: "帮助理解本次结果。", EvidenceRefs: []output.EvidenceRef{
				{Kind: output.EvidenceKindDimension, Ref: assembled.Document.Facts.Dimensions[0].Ref},
				{Kind: output.EvidenceKindDimension, Ref: assembled.Document.Facts.Dimensions[1].Ref},
			},
		}},
		Suggestions: []output.Suggestion{{
			Origin: output.SuggestionOriginGeneratedLowRisk, Category: "medical_treatment", Title: "记录", Goal: "观察变化",
			Actions: []string{"每天记录一次"}, Rationale: "与本次结果相关。", EvidenceRefs: []output.EvidenceRef{
				{Kind: output.EvidenceKindDimension, Ref: assembled.Document.Facts.Dimensions[0].Ref},
				{Kind: output.EvidenceKindDimension, Ref: assembled.Document.Facts.Dimensions[1].Ref},
			}, SourceSuggestionRefs: []string{},
		}},
		Limitations: []string{"本解读仅基于本次测评，不构成诊断。"},
	}
	assertions := append(append([]Assertion(nil), suite.DefaultGenerationAssertions...), suite.Cases[0].Expected.Assertions...)
	report, err := EvaluateCandidate(context.Background(), marshalCandidate(t, content), assembled.Document, profileRecord.Definition(), assertions, allowSafety{})
	if err != nil {
		t.Fatal(err)
	}
	if report.DeterministicHardGatePassed || report.Validation.SchemaValidatorVersion == "" {
		t.Fatalf("profile failure lost typed Candidate evidence: %#v", report)
	}
	foundProfileFailure, foundSemantic := false, false
	for _, result := range report.Assertions {
		if result.Type == "profile_output_policy_satisfied" && result.Status == AssertionFailed {
			foundProfileFailure = true
		}
		if result.Status == AssertionPendingSemantic {
			foundSemantic = true
		}
	}
	if !foundProfileFailure || !foundSemantic {
		t.Fatalf("profile failure/semantic obligations = %v/%v: %#v", foundProfileFailure, foundSemantic, report.Assertions)
	}
}

func candidateFixture(t *testing.T, caseIndex int) (*Suite, *domainprofile.AIExplanationProfile, *appinput.Result) {
	t.Helper()
	suite, err := LoadV1()
	if err != nil {
		t.Fatal(err)
	}
	if caseIndex < 0 || caseIndex >= len(suite.Cases) || suite.Cases[caseIndex].Stage != "generation" {
		t.Fatalf("invalid generation case index %d", caseIndex)
	}
	profileRecord, err := buildProfile(suite.ProfileFixture, time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	assembled, err := syntheticInput(suite.Cases[caseIndex].ProviderPayload, profileRecord, suite.Cases[caseIndex].CaseID)
	if err != nil {
		t.Fatal(err)
	}
	return suite, profileRecord, assembled
}

func marshalCandidate(t *testing.T, content output.Content) []byte {
	t.Helper()
	raw, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

type allowSafety struct{}

func (allowSafety) Evaluate(_ context.Context, _ appport.SafetyRequest) (appport.SafetyResult, error) {
	return appport.SafetyResult{Allowed: true, ValidatorVersion: "test-safety/v1"}, nil
}
