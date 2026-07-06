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

func TestAPICatalogCapabilityMatrix(t *testing.T) {
	t.Parallel()

	for _, cap := range domain.DefaultCapabilities() {
		cap := cap
		apiKind := DomainKindToAPIKind(cap.Kind)
		t.Run(apiKind, func(t *testing.T) {
			t.Parallel()

			mapped, ok := APIKindToDomainKind(apiKind)
			if !ok {
				t.Fatalf("APIKindToDomainKind(%q) = false, want true", apiKind)
			}
			if mapped != cap.Kind {
				t.Fatalf("APIKindToDomainKind(%q) = %q, want %q", apiKind, mapped, cap.Kind)
			}
			if got := DomainKindToAPIKind(cap.Kind); got != apiKind {
				t.Fatalf("DomainKindToAPIKind(%q) = %q, want %q", cap.Kind, got, apiKind)
			}

			got, ok := capabilityForAPIKind(apiKind)
			if !ok {
				t.Fatalf("capabilityForAPIKind(%q) = false, want true", apiKind)
			}
			if got.OptionsEnabled != cap.OptionsEnabled {
				t.Fatalf("OptionsEnabled = %v, want %v", got.OptionsEnabled, cap.OptionsEnabled)
			}
			if got.CreateSupported != cap.CreateSupported {
				t.Fatalf("CreateSupported = %v, want %v", got.CreateSupported, cap.CreateSupported)
			}
			if got.PreviewSupported != cap.PreviewSupported {
				t.Fatalf("PreviewSupported = %v, want %v", got.PreviewSupported, cap.PreviewSupported)
			}
		})
	}
}

func TestOptionsReflectsCapabilityPolicy(t *testing.T) {
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

	for _, cap := range domain.DefaultCapabilities() {
		apiKind := DomainKindToAPIKind(cap.Kind)
		item, ok := optionByValue[apiKind]
		if !ok {
			t.Fatalf("Options().Kinds missing %q", apiKind)
		}
		if item.Label != cap.DisplayName {
			t.Fatalf("Options().Kinds[%q].Label = %q, want %q", apiKind, item.Label, cap.DisplayName)
		}
		wantDisabled := !cap.OptionsEnabled
		if item.Disabled != wantDisabled {
			t.Fatalf("Options().Kinds[%q].Disabled = %v, want %v", apiKind, item.Disabled, wantDisabled)
		}
	}
}

func TestCreateCapabilityPolicy(t *testing.T) {
	t.Parallel()

	for _, cap := range domain.DefaultCapabilities() {
		cap := cap
		apiKind := DomainKindToAPIKind(cap.Kind)
		t.Run(apiKind, func(t *testing.T) {
			t.Parallel()

			behaviorStub := &behaviorCommandStub{}
			personalityStub := &personalityCommandStub{}
			svc := NewService(Dependencies{
				BehaviorCommand:    behaviorStub,
				PersonalityCommand: personalityStub,
			})

			_, err := svc.Create(context.Background(), CreateModelDTO{
				Kind:  apiKind,
				Code:  "capability_" + apiKind,
				Title: "Capability Policy",
			})

			if cap.CreateSupported {
				if err != nil {
					t.Fatalf("Create(%q) error = %v, want success", apiKind, err)
				}
				switch apiKind {
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
				t.Fatalf("Create(%q) error = nil, want rejection", apiKind)
			}
			if !cberrors.IsCode(err, code.ErrInvalidArgument) {
				t.Fatalf("Create(%q) code = %v, want ErrInvalidArgument", apiKind, cberrors.ParseCoder(err))
			}
		})
	}
}

func TestBehaviorAbilityIsLegacyScaleAdapter(t *testing.T) {
	t.Parallel()

	cap, ok := domain.CapabilityByKind(domain.KindBehaviorAbility)
	if !ok || !cap.RuntimeViaScaleLegacy || cap.RuntimeExecutable {
		t.Fatalf("behavior_ability capability = %#v", cap)
	}
	if cap.APIKind != domain.APIKindBehaviorAbility {
		t.Fatalf("APIKind = %q, want %q", cap.APIKind, domain.APIKindBehaviorAbility)
	}
	if behavior.PayloadFormatScale != domain.PayloadFormatBehaviorAbilityScaleV1 {
		t.Fatalf("behavior payload format = %q, want %q", behavior.PayloadFormatScale, domain.PayloadFormatBehaviorAbilityScaleV1)
	}
	if domain.PayloadFormatBehavioralRatingDefaultV1 == domain.PayloadFormatBehaviorAbilityScaleV1 {
		t.Fatal("behavior_ability must not use canonical behavioral_rating.default payload format")
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

func TestPreviewReportCapabilityPolicy(t *testing.T) {
	t.Parallel()

	svc := NewService(Dependencies{
		BehaviorCommand:    &behaviorCommandStub{},
		PersonalityCommand: &personalityCommandStub{},
	})

	for _, cap := range domain.DefaultCapabilities() {
		cap := cap
		apiKind := DomainKindToAPIKind(cap.Kind)
		t.Run(apiKind, func(t *testing.T) {
			t.Parallel()

			_, err := svc.PreviewReport(context.Background(), previewModelCode(apiKind), json.RawMessage(`{}`))
			if cap.PreviewSupported {
				if err != nil {
					t.Fatalf("PreviewReport(%q) error = %v, want success path", apiKind, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("PreviewReport(%q) error = nil, want rejection", apiKind)
			}
		})
	}
}

func TestListRejectsNonListableKinds(t *testing.T) {
	t.Parallel()

	svc := NewService(Dependencies{})
	_, err := svc.List(context.Background(), ListModelsDTO{Kind: KindMedicalScale, Page: 1, PageSize: 10})
	if err == nil {
		t.Fatal("List(medical_scale) error = nil, want rejection")
	}
	if !cberrors.IsCode(err, code.ErrInvalidArgument) {
		t.Fatalf("List(medical_scale) code = %v, want ErrInvalidArgument", cberrors.ParseCoder(err))
	}
}
