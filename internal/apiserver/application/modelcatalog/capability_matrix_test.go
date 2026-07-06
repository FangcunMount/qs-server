package modelcatalog

import (
	"context"
	"encoding/json"
	"testing"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/behavior"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// catalogCapability documents the model-catalog API surface. Update this table
// when enabling or disabling a model family; Batch 2 will promote it to policy.
type catalogCapability struct {
	apiKind          string
	domainKind       domain.Kind
	optionsEnabled   bool
	createSupported  bool
	previewSupported bool
}

func expectedCatalogCapabilities() []catalogCapability {
	return []catalogCapability{
		{
			apiKind:          KindPersonality,
			domainKind:       domain.KindPersonality,
			optionsEnabled:   true,
			createSupported:  true,
			previewSupported: true,
		},
		{
			apiKind:          KindBehaviorAbility,
			domainKind:       domain.KindBehavioralRating,
			optionsEnabled:   true,
			createSupported:  true,
			previewSupported: false,
		},
		{
			apiKind:          KindMedicalScale,
			domainKind:       domain.KindScale,
			optionsEnabled:   true,
			createSupported:  false,
			previewSupported: false,
		},
		{
			apiKind:          KindCognitive,
			domainKind:       domain.KindCognitive,
			optionsEnabled:   false,
			createSupported:  false,
			previewSupported: false,
		},
		{
			apiKind:          KindCustom,
			domainKind:       domain.KindCustom,
			optionsEnabled:   false,
			createSupported:  false,
			previewSupported: false,
		},
	}
}

func TestAPICatalogCapabilityMatrix(t *testing.T) {
	t.Parallel()

	for _, tc := range expectedCatalogCapabilities() {
		tc := tc
		t.Run(tc.apiKind, func(t *testing.T) {
			t.Parallel()

			mapped, ok := APIKindToDomainKind(tc.apiKind)
			if !ok {
				t.Fatalf("APIKindToDomainKind(%q) = false, want true", tc.apiKind)
			}
			if mapped != tc.domainKind {
				t.Fatalf("APIKindToDomainKind(%q) = %q, want %q", tc.apiKind, mapped, tc.domainKind)
			}
			if got := DomainKindToAPIKind(tc.domainKind); got != tc.apiKind {
				t.Fatalf("DomainKindToAPIKind(%q) = %q, want %q", tc.domainKind, got, tc.apiKind)
			}
		})
	}
}

func TestOptionsReflectsCapabilityMatrix(t *testing.T) {
	t.Parallel()

	svc := NewService(Dependencies{})
	result, err := svc.Options(context.Background(), "")
	if err != nil {
		t.Fatalf("Options() error = %v", err)
	}

	optionByValue := make(map[string]Option, len(result.Kinds))
	for _, item := range result.Kinds {
		optionByValue[item.Value] = item
	}

	for _, tc := range expectedCatalogCapabilities() {
		item, ok := optionByValue[tc.apiKind]
		if !ok {
			t.Fatalf("Options().Kinds missing %q", tc.apiKind)
		}
		wantDisabled := !tc.optionsEnabled
		if item.Disabled != wantDisabled {
			t.Fatalf("Options().Kinds[%q].Disabled = %v, want %v", tc.apiKind, item.Disabled, wantDisabled)
		}
	}
}

func TestCreateCapabilityMatrix(t *testing.T) {
	t.Parallel()

	behaviorStub := &behaviorCommandStub{}
	personalityStub := &personalityCommandStub{}
	svc := NewService(Dependencies{
		BehaviorCommand:    behaviorStub,
		PersonalityCommand: personalityStub,
	})

	for _, tc := range expectedCatalogCapabilities() {
		tc := tc
		t.Run(tc.apiKind, func(t *testing.T) {
			t.Parallel()

			behaviorStub.createCalled = false
			personalityStub.createCalled = false

			_, err := svc.Create(context.Background(), CreateModelDTO{
				Kind:  tc.apiKind,
				Code:  "capability_" + tc.apiKind,
				Title: "Capability Matrix",
			})

			if tc.createSupported {
				if err != nil {
					t.Fatalf("Create(%q) error = %v, want success", tc.apiKind, err)
				}
				switch tc.apiKind {
				case KindBehaviorAbility:
					if !behaviorStub.createCalled {
						t.Fatal("behavior command Create was not called")
					}
				case KindPersonality:
					if !personalityStub.createCalled {
						t.Fatal("personality command Create was not called")
					}
				}
				return
			}

			if err == nil {
				t.Fatalf("Create(%q) error = nil, want rejection", tc.apiKind)
			}
			if !cberrors.IsCode(err, code.ErrInvalidArgument) {
				t.Fatalf("Create(%q) code = %v, want ErrInvalidArgument", tc.apiKind, cberrors.ParseCoder(err))
			}
		})
	}
}

func TestBehaviorAbilityIsLegacyScaleAdapter(t *testing.T) {
	t.Parallel()

	mapped, ok := APIKindToDomainKind(KindBehaviorAbility)
	if !ok || mapped != domain.KindBehavioralRating {
		t.Fatalf("behavior_ability domain kind = %q, want behavioral_rating", mapped)
	}
	if behavior.PayloadFormatScale != PayloadFormatScaleV1 {
		t.Fatalf("behavior payload format drift: behavior=%q dto=%q", behavior.PayloadFormatScale, PayloadFormatScaleV1)
	}
	if behavior.PayloadFormatScale != "assessmentmodel.behavior_ability.scale.v1" {
		t.Fatalf("behavior payload format = %q, want legacy scale adapter envelope", behavior.PayloadFormatScale)
	}
	if domain.PayloadFormatBehavioralRatingDefaultV1 == behavior.PayloadFormatScale {
		t.Fatal("behavior_ability must not use canonical behavioral_rating.default payload format yet")
	}
}

func previewModelCode(apiKind string) string {
	switch apiKind {
	case KindPersonality:
		return "personality_demo"
	case KindBehaviorAbility:
		return "behavior_demo"
	default:
		return apiKind + "_demo"
	}
}

func TestPreviewReportCapabilityMatrix(t *testing.T) {
	t.Parallel()

	svc := NewService(Dependencies{
		BehaviorCommand:    &behaviorCommandStub{},
		PersonalityCommand: &personalityCommandStub{},
	})

	for _, tc := range expectedCatalogCapabilities() {
		tc := tc
		t.Run(tc.apiKind, func(t *testing.T) {
			t.Parallel()

			_, err := svc.PreviewReport(context.Background(), previewModelCode(tc.apiKind), json.RawMessage(`{}`))
			if tc.previewSupported {
				if err != nil {
					t.Fatalf("PreviewReport(%q) error = %v, want success path", tc.apiKind, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("PreviewReport(%q) error = nil, want rejection", tc.apiKind)
			}
		})
	}
}
