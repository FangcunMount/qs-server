package profile

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/output"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestProfileLifecycleDoesNotChangeFingerprint(t *testing.T) {
	createdAt := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	profile, err := NewDraft(meta.FromUint64(1), validDefinition(nil, nil), createdAt)
	if err != nil {
		t.Fatal(err)
	}
	want := profile.Fingerprint()
	if err := profile.Publish(meta.ID(101), "operator:1", "approved evaluation", createdAt.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := profile.Disable("operator:2", "retired release", createdAt.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if profile.Fingerprint() != want || profile.Status() != StatusDisabled {
		t.Fatal("profile lifecycle changed immutable policy identity")
	}
}

func TestProfileDefinitionClonePreservesEmptyArrayFingerprint(t *testing.T) {
	definition := validDefinition(nil, nil)
	definition.Eligibility.EligibleDimensionCodes = []string{}
	definition.Eligibility.ExcludedDimensionCodes = []string{}
	profile, err := NewDraft(meta.FromUint64(1), definition, time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}

	clonedFingerprint, err := profile.Definition().Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if clonedFingerprint != profile.Fingerprint() {
		t.Fatalf("cloned Definition fingerprint = %s, want %s", clonedFingerprint, profile.Fingerprint())
	}
}

func TestReleaseDraftRetainsTrustedCreationAudit(t *testing.T) {
	createdAt := time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC)
	profile, err := NewDraftForRelease(meta.FromUint64(1), validDefinition(nil, nil), "user:42", "initial release candidate", createdAt)
	if err != nil {
		t.Fatal(err)
	}
	if profile.CreatedBy() != "user:42" || profile.CreatedReason() != "initial release candidate" {
		t.Fatalf("creation audit = %q/%q", profile.CreatedBy(), profile.CreatedReason())
	}
	if _, err := NewDraftForRelease(meta.FromUint64(2), validDefinition(nil, nil), "", "missing actor", createdAt); err == nil {
		t.Fatal("expected missing creation actor rejection")
	}
}

func TestProfileRejectsRelaxedSafetyPolicy(t *testing.T) {
	definition := validDefinition(nil, nil)
	definition.SafetyPolicy.ForbiddenClaims = definition.SafetyPolicy.ForbiddenClaims[:6]
	if _, err := NewDraft(meta.FromUint64(1), definition, time.Now()); err == nil {
		t.Fatal("expected relaxed safety policy rejection")
	}
}

func TestProfileV1RejectsSelectorsOutsideParticipantScaleScoreRange(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Definition)
	}{
		{
			name: "clinician audience",
			mutate: func(definition *Definition) {
				definition.Selector.Audience = policy.AudienceClinician
			},
		},
		{
			name: "typology model",
			mutate: func(definition *Definition) {
				definition.Selector.ModelKind = modelcatalog.KindTypology
			},
		},
		{
			name: "pole composition decision",
			mutate: func(definition *Definition) {
				definition.Selector.DecisionKind = modelcatalog.DecisionKindPoleComposition
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			definition := validDefinition(nil, nil)
			tt.mutate(&definition)
			if _, err := NewDraft(meta.FromUint64(1), definition, time.Now()); err == nil {
				t.Fatal("expected unsupported v1 selector rejection")
			}
		})
	}
}

func TestResolverUsesExactThenCodeThenKindPrecedence(t *testing.T) {
	code := "scale-a"
	version := "v1"
	generic := publishedProfile(t, 1, validDefinition(nil, nil))
	codeDefault := publishedProfile(t, 2, validDefinition(&code, nil))
	exact := publishedProfile(t, 3, validDefinition(&code, &version))

	resolved, err := Resolve(context.Background(), repositoryStub{items: []*AIExplanationProfile{generic, codeDefault, exact}}, ResolveQuery{
		Audience: policy.AudienceParticipant, ModelKind: modelcatalog.KindScale,
		DecisionKind: modelcatalog.DecisionKindScoreRange, ModelCode: code, ModelVersion: version,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ID() != exact.ID() {
		t.Fatalf("resolved profile = %s, want exact %s", resolved.ID(), exact.ID())
	}
}

func TestResolverRejectsSameSpecificityAmbiguity(t *testing.T) {
	code := "scale-a"
	one := publishedProfile(t, 1, validDefinition(&code, nil))
	twoDefinition := validDefinition(&code, nil)
	twoDefinition.ProfileID = "participant-scale-code-second"
	two := publishedProfile(t, 2, twoDefinition)

	_, err := Resolve(context.Background(), repositoryStub{items: []*AIExplanationProfile{one, two}}, ResolveQuery{
		Audience: policy.AudienceParticipant, ModelKind: modelcatalog.KindScale,
		DecisionKind: modelcatalog.DecisionKindScoreRange, ModelCode: code, ModelVersion: "v1",
	})
	if !errors.Is(err, ErrAmbiguousSelector) {
		t.Fatalf("resolve error = %v, want ambiguous selector", err)
	}
}

func validDefinition(modelCode, modelVersion *string) Definition {
	return Definition{
		SchemaVersion: aiexplanation.ProfileSchemaVersionV1,
		ProfileID:     "participant-scale-default",
		Version:       "v1",
		Selector: Selector{
			Audience: policy.AudienceParticipant, ModelKind: modelcatalog.KindScale,
			DecisionKind: modelcatalog.DecisionKindScoreRange, ModelCode: modelCode, ModelVersion: modelVersion,
		},
		Eligibility: EligibilityPolicy{
			MinEligibleDimensions: 2, MaxInputDimensions: 12, OnDimensionOverflow: "reject",
		},
		InputPolicy: InputPolicy{ContextScope: "current_assessment_only", AllowedFocusAreas: []string{"sleep_routine"}},
		InsightPolicy: InsightPolicy{
			AllowedKinds: []output.InsightKind{output.InsightKindReinforcingPattern},
			MinItems:     1, MaxItems: 3, MinDimensionRefsPerItem: 2, MaxDimensionRefsPerItem: 3,
		},
		SuggestionPolicy: SuggestionPolicy{
			AllowedOrigins:    []output.SuggestionOrigin{output.SuggestionOriginStandardDerived, output.SuggestionOriginGeneratedLowRisk},
			AllowedCategories: []string{"daily_practice"}, MinItems: 1, MaxItems: 3, MaxActionsPerItem: 3,
			RequireEvidenceRefs: true, RequireStandardRefsForStandardDerived: true,
		},
		SafetyPolicy: SafetyPolicy{
			PolicyVersion: "v1", DisclaimerVersion: "v1",
			ForbiddenClaims: []string{"diagnosis", "causality", "medication", "treatment_plan", "risk_reclassification", "identity_inference", "deterministic_future_prediction"},
		},
		GenerationPolicy: GenerationPolicy{
			PromptTemplateID: "cross-dimension-participant-scale", PromptVersion: "v1", ProviderRoute: "balanced_text_v1",
			InputSchemaVersion: aiexplanation.InputSchemaVersionV1, OutputSchemaVersion: aiexplanation.OutputSchemaVersionV1,
			MaxOutputCharacters: 8000,
		},
	}
}

func publishedProfile(t *testing.T, id uint64, definition Definition) *AIExplanationProfile {
	t.Helper()
	profile, err := NewDraft(meta.FromUint64(id), definition, time.Date(2026, 8, 26, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if err := profile.Publish(meta.ID(101), "operator:1", "approved evaluation", time.Date(2026, 8, 26, 10, 1, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	return profile
}

type repositoryStub struct {
	items []*AIExplanationProfile
	err   error
}

func (r repositoryStub) Save(context.Context, *AIExplanationProfile) error { return nil }
func (r repositoryStub) FindByKey(context.Context, string, string) (*AIExplanationProfile, error) {
	return nil, ErrNotFound
}
func (r repositoryStub) ListPublishedByBaseSelector(context.Context, policy.Audience, modelcatalog.Kind, modelcatalog.DecisionKind) ([]*AIExplanationProfile, error) {
	return r.items, r.err
}
