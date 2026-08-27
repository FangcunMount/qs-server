package input

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	appsource "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/source"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/output"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestAssembleBuildsFrozenInputAndMinimalProviderPayload(t *testing.T) {
	source := testCurrentSource(t, modelcatalog.KindScale, modelcatalog.DecisionKindScoreRange, []string{"sleep", "stress"})
	profile := testPublishedProfile(t, source, func(definition *domainprofile.Definition) {
		definition.InputPolicy.AllowedFocusAreas = []string{"daily_routine"}
	})
	result, err := Assemble(Request{
		Source: source, Profile: profile, Audience: policy.AudienceParticipant, Locale: "zh-CN", FocusAreas: []string{"daily_routine"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Snapshot.Fingerprint() == "" || result.Snapshot.SchemaVersion() != aiexplanation.InputSchemaVersionV1 {
		t.Fatalf("snapshot = %#v", result.Snapshot)
	}
	if len(result.Document.Facts.Dimensions) != 2 {
		t.Fatalf("dimensions = %#v", result.Document.Facts.Dimensions)
	}
	if result.Document.Facts.Runtime.DecisionKind != string(modelcatalog.DecisionKindScoreRange) {
		t.Fatalf("decision kind = %q", result.Document.Facts.Runtime.DecisionKind)
	}
	if result.Document.Context.PersonalizationScope != "assessment_result_and_focus_areas" {
		t.Fatalf("personalization scope = %q", result.Document.Context.PersonalizationScope)
	}
	for _, dimension := range result.Document.Facts.Dimensions {
		if !strings.HasPrefix(dimension.Ref, "dimension:") {
			t.Fatalf("dimension ref = %q", dimension.Ref)
		}
	}
	var provider map[string]json.RawMessage
	if err := json.Unmarshal(result.ProviderPayload, &provider); err != nil {
		t.Fatal(err)
	}
	if len(provider) != 2 || provider["context"] == nil || provider["facts"] == nil {
		t.Fatalf("provider keys = %v", provider)
	}
	for _, forbidden := range []string{"source", "profile", source.Report.ID().String(), source.Report.OutcomeID().String()} {
		if strings.Contains(string(result.ProviderPayload), forbidden) {
			t.Fatalf("provider payload contains forbidden value %q: %s", forbidden, result.ProviderPayload)
		}
	}
	again, err := Assemble(Request{Source: source, Profile: profile, Audience: policy.AudienceParticipant, Locale: "zh-CN", FocusAreas: []string{"daily_routine"}})
	if err != nil {
		t.Fatal(err)
	}
	if again.Snapshot.Fingerprint() != result.Snapshot.Fingerprint() || string(again.Snapshot.CanonicalJSON()) != string(result.Snapshot.CanonicalJSON()) {
		t.Fatal("identical immutable facts must produce identical snapshot identity")
	}
}

func TestRestoreRebuildsOnlyProviderProjectionFromFrozenSnapshot(t *testing.T) {
	source := testCurrentSource(t, modelcatalog.KindScale, modelcatalog.DecisionKindScoreRange, []string{"sleep", "stress"})
	profile := testPublishedProfile(t, source, nil)
	assembled, err := Assemble(Request{Source: source, Profile: profile, Audience: policy.AudienceParticipant, Locale: "zh-CN"})
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Restore(assembled.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Snapshot.Fingerprint() != assembled.Snapshot.Fingerprint() || string(restored.ProviderPayload) != string(assembled.ProviderPayload) {
		t.Fatalf("restored snapshot/payload drifted: %s/%s", restored.Snapshot.Fingerprint(), restored.ProviderPayload)
	}
	for _, forbidden := range []string{"source", "profile", source.Report.ID().String(), source.Report.OutcomeID().String()} {
		if strings.Contains(string(restored.ProviderPayload), forbidden) {
			t.Fatalf("restored provider payload contains %q", forbidden)
		}
	}
}

func TestAssembleUsesParticipantVisibleAndProfileEligibleDimensionsOnly(t *testing.T) {
	source := testCurrentSource(t, modelcatalog.KindScale, modelcatalog.DecisionKindScoreRange, []string{"sleep", "stress"})
	profile := testPublishedProfile(t, source, func(definition *domainprofile.Definition) {
		definition.Eligibility.EligibleDimensionCodes = []string{"sleep", "stress", "hidden"}
	})
	result, err := Assemble(Request{Source: source, Profile: profile, Audience: policy.AudienceParticipant, Locale: "zh-CN"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Document.Facts.Dimensions) != 2 {
		t.Fatalf("visible dimensions = %#v", result.Document.Facts.Dimensions)
	}
	for _, dimension := range result.Document.Facts.Dimensions {
		if dimension.Code == "hidden" {
			t.Fatal("participant-hidden dimension leaked into provider input")
		}
	}
}

func TestAssembleRejectsUnsupportedScopeAndInvalidPersonalization(t *testing.T) {
	scale := testCurrentSource(t, modelcatalog.KindScale, modelcatalog.DecisionKindScoreRange, []string{"sleep", "stress"})
	profile := testPublishedProfile(t, scale, func(definition *domainprofile.Definition) {
		definition.InputPolicy.AllowedFocusAreas = []string{"daily_routine"}
	})
	tests := []struct {
		name    string
		request Request
		want    error
	}{
		{name: "clinician", request: Request{Source: scale, Profile: profile, Audience: policy.AudienceClinician, Locale: "zh-CN"}, want: ErrNotApplicable},
		{name: "unknown focus", request: Request{Source: scale, Profile: profile, Audience: policy.AudienceParticipant, Locale: "zh-CN", FocusAreas: []string{"medical_history"}}, want: ErrInvalidInput},
		{name: "invalid locale", request: Request{Source: scale, Profile: profile, Audience: policy.AudienceParticipant, Locale: "zh_CN"}, want: ErrInvalidInput},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Assemble(tt.request)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}

	typology := testCurrentSource(t, modelcatalog.KindTypology, modelcatalog.DecisionKindPoleComposition, []string{"sleep", "stress"})
	_, err := Assemble(Request{Source: typology, Profile: profile, Audience: policy.AudienceParticipant, Locale: "zh-CN"})
	if !errors.Is(err, ErrNotApplicable) {
		t.Fatalf("typology error = %v", err)
	}
}

func TestAssembleRejectsInsufficientEligibleDimensionsBeforeProviderCall(t *testing.T) {
	source := testCurrentSource(t, modelcatalog.KindScale, modelcatalog.DecisionKindScoreRange, []string{"sleep", "stress"})
	profile := testPublishedProfile(t, source, func(definition *domainprofile.Definition) {
		definition.Eligibility.EligibleDimensionCodes = []string{"sleep"}
	})
	_, err := Assemble(Request{Source: source, Profile: profile, Audience: policy.AudienceParticipant, Locale: "zh-CN"})
	if !errors.Is(err, ErrNotApplicable) {
		t.Fatalf("error = %v", err)
	}
}

func testCurrentSource(t *testing.T, kind modelcatalog.Kind, decision modelcatalog.DecisionKind, visible []string) *appsource.Current {
	t.Helper()
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	algorithm := modelcatalog.AlgorithmScaleDefault
	if kind == modelcatalog.KindTypology {
		algorithm = modelcatalog.AlgorithmPersonalityTypology
	}
	model := domainreport.ModelIdentity{Kind: string(kind), Algorithm: string(algorithm), Code: "model-a", Version: "v1", Title: "Model A"}
	max := 10.0
	sleep := domainreport.NewDimensionInterpret(domainreport.NewFactorCode("sleep"), "睡眠", 6, &max, domainreport.RiskLevelLow, "睡眠表现需要关注", "记录睡眠节律").WithHierarchy("factor", "", 1, 1)
	stress := domainreport.NewDimensionInterpret(domainreport.NewFactorCode("stress"), "压力", 7, &max, domainreport.RiskLevelMedium, "压力水平偏高", "安排短暂休息").WithHierarchy("factor", "", 1, 2)
	hidden := domainreport.NewDimensionInterpret(domainreport.NewFactorCode("hidden"), "隐藏维度", 9, &max, domainreport.RiskLevelHigh, "不应展示", "不应发送").WithHierarchy("factor", "", 1, 3)
	factor := domainreport.NewFactorCode("sleep")
	report, err := domainreport.NewInterpretReport(domainreport.InterpretReportInput{
		ID: meta.FromUint64(101), GenerationID: meta.FromUint64(201), OutcomeID: meta.FromUint64(301), InterpretationRunID: meta.FromUint64(401),
		Association: domainreport.Association{OrgID: 1, AssessmentID: meta.FromUint64(7), TesteeID: 9}, ReportType: policy.ReportTypeStandard,
		TemplateVersion: policy.TemplateVersionCurrent, BuilderIdentity: domainreport.BuilderIdentityFactorScoring, ContentSchemaVersion: domainreport.ContentSchemaVersionV1,
		Content: domainreport.Content{
			Model: model, PrimaryScore: &domainreport.ScoreValue{Kind: domainreport.ScoreKindRawTotal, Value: 13, Max: &max},
			Level: &domainreport.ResultLevel{Code: "medium", Label: "中等", Severity: "medium"}, Conclusion: "本次结果显示需要关注压力与睡眠的组合。",
			Dimensions:  []domainreport.DimensionInterpret{sleep, stress, hidden},
			Suggestions: []domainreport.Suggestion{{Category: domainreport.SuggestionCategoryDimension, Content: "记录一周睡眠", FactorCode: &factor}},
			PresentationProfile: func() *domainreport.PresentationProfile {
				value := domainreport.NewFrozenPresentationProfile(visible)
				return &value
			}(),
		},
		GeneratedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	association := report.Association()
	outcome := evaluationfact.NewRecord(evaluationfact.NewRecordInput{
		ID: report.OutcomeID(), OrgID: association.OrgID, AssessmentID: association.AssessmentID, TesteeID: association.TesteeID,
		Model:   evaluationfact.ModelIdentity{Kind: kind, Algorithm: algorithm, Code: model.Code, Version: model.Version, Title: model.Title},
		Runtime: evaluationfact.RuntimeIdentity{DecisionKind: decision}, EvaluatedAt: now.Add(-time.Second),
	})
	return &appsource.Current{Report: report, Outcome: outcome}
}

func testPublishedProfile(t *testing.T, source *appsource.Current, mutate func(*domainprofile.Definition)) *domainprofile.AIExplanationProfile {
	t.Helper()
	model := source.Outcome.Model()
	definition := domainprofile.Definition{
		SchemaVersion: aiexplanation.ProfileSchemaVersionV1,
		ProfileID:     "participant-scale",
		Version:       "v1",
		Selector: domainprofile.Selector{
			Audience: policy.AudienceParticipant, ModelKind: model.Kind, DecisionKind: source.Outcome.Runtime().DecisionKind,
		},
		Eligibility: domainprofile.EligibilityPolicy{MinEligibleDimensions: 2, MaxInputDimensions: 50, OnDimensionOverflow: "reject"},
		InputPolicy: domainprofile.InputPolicy{ContextScope: "current_assessment_only", IncludeNormContext: true, IncludeModelResult: false},
		InsightPolicy: domainprofile.InsightPolicy{
			AllowedKinds: []output.InsightKind{output.InsightKindReinforcingPattern, output.InsightKindContrastingPattern},
			MinItems:     1, MaxItems: 3, MinDimensionRefsPerItem: 2, MaxDimensionRefsPerItem: 4, AllowCausalClaims: false,
		},
		SuggestionPolicy: domainprofile.SuggestionPolicy{
			AllowedOrigins:    []output.SuggestionOrigin{output.SuggestionOriginStandardDerived, output.SuggestionOriginGeneratedLowRisk},
			AllowedCategories: []string{"daily_observation", "routine"}, MinItems: 1, MaxItems: 3, MaxActionsPerItem: 3,
			RequireEvidenceRefs: true, RequireStandardRefsForStandardDerived: true,
		},
		SafetyPolicy: domainprofile.SafetyPolicy{
			PolicyVersion: "v1", DisclaimerVersion: "v1",
			ForbiddenClaims: []string{"diagnosis", "causality", "medication", "treatment_plan", "risk_reclassification", "identity_inference", "deterministic_future_prediction"},
		},
		GenerationPolicy: domainprofile.GenerationPolicy{
			PromptTemplateID: "cross-dimension-participant-scale", PromptVersion: "v1", ProviderRoute: "participant_default",
			InputSchemaVersion: aiexplanation.InputSchemaVersionV1, OutputSchemaVersion: aiexplanation.OutputSchemaVersionV1, MaxOutputCharacters: 4000,
		},
	}
	if mutate != nil {
		mutate(&definition)
	}
	profile, err := domainprofile.NewDraft(meta.FromUint64(501), definition, time.Date(2026, 8, 26, 11, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if err := profile.Publish(meta.ID(101), "tester", "approved evaluation", time.Date(2026, 8, 26, 11, 1, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	return profile
}
