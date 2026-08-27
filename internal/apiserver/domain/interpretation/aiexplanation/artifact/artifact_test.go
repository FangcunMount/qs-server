package artifact

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/output"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestArtifactStoresOnlyValidatedImmutableContent(t *testing.T) {
	input := validArtifactInput()
	artifact, err := New(input)
	if err != nil {
		t.Fatal(err)
	}
	input.Content.Suggestions[0].Actions[0] = "mutated"
	content := artifact.Content()
	content.Suggestions[0].Actions[0] = "also-mutated"
	if artifact.Content().Suggestions[0].Actions[0] != "记录一次相关情境。" {
		t.Fatal("artifact content was mutated after construction")
	}
}

func TestArtifactRejectsProviderReceiptOutsideExecutionSpec(t *testing.T) {
	input := validArtifactInput()
	input.ProviderReceipt.Model = "different-model"
	if _, err := New(input); err == nil {
		t.Fatal("expected provider execution mismatch")
	}
}

func validArtifactInput() NewInput {
	validatedAt := time.Date(2026, 8, 26, 10, 0, 2, 0, time.UTC)
	profileFingerprint := aiexplanation.NewFingerprint([]byte("profile"))
	executionFingerprint := aiexplanation.NewFingerprint([]byte("route"))
	return NewInput{
		ID: meta.FromUint64(401), GenerationID: meta.FromUint64(201), RunID: meta.FromUint64(301),
		Source: SourceRef{
			ReportID: meta.FromUint64(101), OutcomeID: meta.FromUint64(102), ReportType: "standard",
			Association:     aiexplanation.Association{OrgID: 9, AssessmentID: meta.FromUint64(501), TesteeID: 601},
			TemplateVersion: "v1", ContentSchemaVersion: "interpret-report-content/v1", BuilderIdentity: "factor-scoring/v1",
			ReportGeneratedAt: validatedAt.Add(-time.Hour),
		},
		Audience: policy.AudienceParticipant,
		Profile:  aiexplanation.ProfileRef{ID: "participant-scale", Version: "v1", Fingerprint: profileFingerprint},
		Prompt: aiexplanation.PromptRef{
			TemplateID: "cross-dimension-participant-scale", Version: "v1",
			Fingerprint: aiexplanation.NewFingerprint([]byte("prompt")), GitBlobSHA: "abc123",
		},
		ExecutionSpec: aiexplanation.ProviderExecutionSpec{
			Route: "participant_scale_v1", RouteRevision: "v1", ResolvedProvider: "provider-a", ResolvedModel: "model-a", Fingerprint: executionFingerprint,
		},
		InputSchema: aiexplanation.InputSchemaVersionV1, InputFingerprint: aiexplanation.NewFingerprint([]byte("input")),
		OutputSchema: aiexplanation.OutputSchemaVersionV1, SafetyPolicy: "ai-safety/v1",
		ProviderReceipt: aiexplanation.ProviderReceipt{
			InvocationID: "generation-201/attempt-1", RequestID: "provider-request-1", Provider: "provider-a", Model: "model-a",
			InputTokens: 100, OutputTokens: 200, Latency: time.Second,
		},
		Validation: ValidationReceipt{
			SchemaValidatorVersion: "schema/v1", ReferenceValidatorVersion: "reference/v1",
			ProfileValidatorVersion: "profile/v1", SafetyValidatorVersion: "safety/v1", ValidatedAt: validatedAt,
		},
		Content: output.Content{
			SchemaVersion: aiexplanation.OutputSchemaVersionV1, Summary: "本次结果呈现出两个维度之间值得关注的组合关系。",
			IntegratedInsights: []output.IntegratedInsight{{
				Kind: output.InsightKindReinforcingPattern, Title: "组合表现", Content: "两个维度在部分情境中可能相互增强。",
				WhyItMatters: "有助于理解本次结果的整体表现。",
				EvidenceRefs: []output.EvidenceRef{{Kind: output.EvidenceKindDimension, Ref: "dimension:a"}, {Kind: output.EvidenceKindDimension, Ref: "dimension:b"}},
			}},
			Suggestions: []output.Suggestion{{
				Origin: output.SuggestionOriginStandardDerived, Category: "daily_observation", Title: "观察情境",
				Goal: "了解组合表现出现的情境。", Actions: []string{"记录一次相关情境。"}, Rationale: "与本次结果相关。",
				EvidenceRefs:         []output.EvidenceRef{{Kind: output.EvidenceKindDimension, Ref: "dimension:a"}},
				SourceSuggestionRefs: []string{"suggestion:a"}, Caution: "可按实际情况选择。",
			}},
			Limitations: []string{"仅基于本次测评，不构成诊断或确定性判断。"},
		},
		GeneratedAt: validatedAt.Add(time.Second),
	}
}
