package validation

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	appinput "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/output"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestValidateAcceptsResolvableProfileConstrainedOutput(t *testing.T) {
	result, err := Validate(marshal(t, validContent()), validInput(), validDefinition())
	if err != nil {
		t.Fatal(err)
	}
	if result.Content.SchemaVersion != aiexplanation.OutputSchemaVersionV1 || result.ReferenceValidatorVersion == "" {
		t.Fatalf("result = %#v", result)
	}
}

func TestValidateRejectsUnknownJSONFieldAndTrailingText(t *testing.T) {
	base := marshal(t, validContent())
	withUnknown := append(append([]byte(nil), base[:len(base)-1]...), []byte(`,"unknown":true}`)...)
	tests := []struct {
		raw  []byte
		want SchemaViolation
	}{
		{raw: withUnknown, want: SchemaViolationUnknownField},
		{raw: append(append([]byte(nil), base...), []byte(" trailing")...), want: SchemaViolationTrailingContent},
	}
	for _, test := range tests {
		_, err := Validate(test.raw, validInput(), validDefinition())
		if !errors.Is(err, ErrSchema) {
			t.Fatalf("schema error = %v", err)
		}
		if got := SchemaViolationOf(err); got != test.want {
			t.Fatalf("schema violation = %q, want %q: %v", got, test.want, err)
		}
	}
}

func TestValidateClassifiesSchemaViolationsWithoutUnboundedDetails(t *testing.T) {
	contentInvalid := validContent()
	contentInvalid.SchemaVersion = ""
	tests := []struct {
		name string
		raw  []byte
		want SchemaViolation
	}{
		{name: "object", raw: []byte(`[]`), want: SchemaViolationObjectRequired},
		{name: "syntax", raw: []byte(`{"schema_version":`), want: SchemaViolationJSONSyntax},
		{name: "field type", raw: []byte(`{"schema_version":"ai-explanation-output/v1","summary":1}`), want: SchemaViolationFieldType},
		{name: "content contract", raw: marshal(t, contentInvalid), want: SchemaViolationContentContract},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Validate(test.raw, validInput(), validDefinition())
			if !errors.Is(err, ErrSchema) || SchemaViolationOf(err) != test.want {
				t.Fatalf("schema violation = %q/%v, want %q", SchemaViolationOf(err), err, test.want)
			}
		})
	}
}

func TestValidateRejectsUnresolvedAndWrongKindReferences(t *testing.T) {
	content := validContent()
	content.IntegratedInsights[0].EvidenceRefs[1].Ref = "dimension:invented"
	raw := marshal(t, content)
	_, err := Validate(raw, validInput(), validDefinition())
	if !errors.Is(err, ErrReference) {
		t.Fatalf("reference error = %v", err)
	}
	typed, err := ParseTypedContent(raw)
	if err != nil || typed.IntegratedInsights[0].EvidenceRefs[1].Ref != "dimension:invented" {
		t.Fatalf("typed content must survive a quality-only reference failure: %#v / %v", typed, err)
	}

	content = validContent()
	content.Suggestions[0].SourceSuggestionRefs = []string{"suggestion:invented"}
	_, err = Validate(marshal(t, content), validInput(), validDefinition())
	if !errors.Is(err, ErrReference) {
		t.Fatalf("source suggestion error = %v", err)
	}
}

func TestValidateRejectsProfileKindCategoryAndHierarchyViolations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*output.Content, *appinput.Document)
	}{
		{name: "kind", mutate: func(content *output.Content, _ *appinput.Document) {
			content.IntegratedInsights[0].Kind = output.InsightKindContrastingPattern
		}},
		{name: "category", mutate: func(content *output.Content, _ *appinput.Document) {
			content.Suggestions[0].Category = "medical_treatment"
		}},
		{name: "parent child", mutate: func(_ *output.Content, input *appinput.Document) {
			parent := "dimension:sleep"
			input.Facts.Dimensions[1].ParentRef = &parent
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := validContent()
			input := validInput()
			tt.mutate(&content, &input)
			_, err := Validate(marshal(t, content), input, validDefinition())
			if !errors.Is(err, ErrProfile) {
				t.Fatalf("profile error = %v", err)
			}
		})
	}
}

func validContent() output.Content {
	return output.Content{
		SchemaVersion: aiexplanation.OutputSchemaVersionV1,
		Summary:       "睡眠与压力可以结合观察。",
		IntegratedInsights: []output.IntegratedInsight{{
			Kind: output.InsightKindReinforcingPattern, Title: "组合关注", Content: "两个维度可能相互增强。", WhyItMatters: "有助于理解本次结果。",
			EvidenceRefs: []output.EvidenceRef{{Kind: output.EvidenceKindDimension, Ref: "dimension:sleep"}, {Kind: output.EvidenceKindDimension, Ref: "dimension:stress"}},
		}},
		Suggestions: []output.Suggestion{{
			Origin: output.SuggestionOriginStandardDerived, Category: "daily_practice", Title: "记录节律", Goal: "观察变化", Actions: []string{"每天记录一次"},
			Rationale: "来自本次标准建议。", EvidenceRefs: []output.EvidenceRef{{Kind: output.EvidenceKindStandardSuggestion, Ref: "suggestion:sleep-note"}},
			SourceSuggestionRefs: []string{"suggestion:sleep-note"},
		}},
		Limitations: []string{"仅基于本次测评，不构成诊断或确定性判断。"},
	}
}

func validInput() appinput.Document {
	return appinput.Document{
		SchemaVersion: aiexplanation.InputSchemaVersionV1,
		Context:       appinput.Context{Scope: "current_assessment_only", Audience: "participant", Locale: "zh-CN", PersonalizationScope: "assessment_result_only", FocusAreas: []string{}},
		Facts: appinput.Facts{
			Runtime: appinput.Runtime{DecisionKind: "score_range"}, Model: appinput.Model{Kind: "scale", Algorithm: "scale_default", Code: "model-a", Version: "v1", Title: "示例量表"},
			Dimensions:          []appinput.Dimension{{Ref: "dimension:sleep", Code: "sleep"}, {Ref: "dimension:stress", Code: "stress"}},
			StandardSuggestions: []appinput.StandardSuggestion{{Ref: "suggestion:sleep-note", Category: "dimension", Content: "记录睡眠", DimensionRefs: []string{"dimension:sleep"}}},
		},
	}
}

func validDefinition() domainprofile.Definition {
	return domainprofile.Definition{
		SchemaVersion: aiexplanation.ProfileSchemaVersionV1, ProfileID: "participant-scale", Version: "v1",
		Selector:         domainprofile.Selector{Audience: policy.AudienceParticipant, ModelKind: modelcatalog.KindScale, DecisionKind: modelcatalog.DecisionKindScoreRange},
		Eligibility:      domainprofile.EligibilityPolicy{MinEligibleDimensions: 2, MaxInputDimensions: 12, OnDimensionOverflow: "reject"},
		InputPolicy:      domainprofile.InputPolicy{ContextScope: "current_assessment_only"},
		InsightPolicy:    domainprofile.InsightPolicy{AllowedKinds: []output.InsightKind{output.InsightKindReinforcingPattern}, MinItems: 1, MaxItems: 2, MinDimensionRefsPerItem: 2, MaxDimensionRefsPerItem: 3},
		SuggestionPolicy: domainprofile.SuggestionPolicy{AllowedOrigins: []output.SuggestionOrigin{output.SuggestionOriginStandardDerived}, AllowedCategories: []string{"daily_practice"}, MinItems: 1, MaxItems: 2, MaxActionsPerItem: 2, RequireEvidenceRefs: true, RequireStandardRefsForStandardDerived: true},
		SafetyPolicy:     domainprofile.SafetyPolicy{PolicyVersion: "v1", DisclaimerVersion: "v1", ForbiddenClaims: []string{"diagnosis", "causality", "medication", "treatment_plan", "risk_reclassification", "identity_inference", "deterministic_future_prediction"}},
		GenerationPolicy: domainprofile.GenerationPolicy{PromptTemplateID: "cross-dimension-participant-scale", PromptVersion: "v1", ProviderRoute: "balanced_text_v1", InputSchemaVersion: aiexplanation.InputSchemaVersionV1, OutputSchemaVersion: aiexplanation.OutputSchemaVersionV1, MaxOutputCharacters: 8000},
	}
}

func marshal(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// The v4 injection-case failures preserved valid JSON with no integrated
// insights. This isolates the observed contract breach without replaying a model.
func TestParseTypedContentRejectsEmptyInsightsAfterIgnoringUntrustedDescription(t *testing.T) {
	content := validContent()
	content.IntegratedInsights = []output.IntegratedInsight{}
	_, err := ParseTypedContent(marshal(t, content))
	if !errors.Is(err, ErrSchema) || SchemaViolationOf(err) != SchemaViolationContentContract || !strings.Contains(err.Error(), "output item counts are invalid") {
		t.Fatalf("empty insight contract = %v", err)
	}
	content.IntegratedInsights = validContent().IntegratedInsights
	if _, err := ParseTypedContent(marshal(t, content)); err != nil {
		t.Fatalf("minimum populated insight rejected: %v", err)
	}
}
