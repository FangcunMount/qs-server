package modelcatalog

import (
	"context"
	"encoding/json"
	"testing"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/option"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

func modelFamilyRegistryOptions() []option.RegisteredOption {
	out := make([]option.RegisteredOption, 0)
	for _, entry := range option.DefaultRegistry().RegisteredOptions() {
		if entry.IsProductChannel() {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func TestListBehaviorAbilityChannelReturnsInvalidArgument(t *testing.T) {
	t.Parallel()

	svc := NewService(Dependencies{})
	_, err := svc.List(context.Background(), ListModelsDTO{
		Kind:     KindBehaviorAbility,
		Page:     1,
		PageSize: 20,
	})
	if err == nil {
		t.Fatal("List(behavior_ability) error = nil, want rejection")
	}
	if !cberrors.IsCode(err, code.ErrInvalidArgument) {
		t.Fatalf("List(behavior_ability) code = %v, want ErrInvalidArgument", cberrors.ParseCoder(err))
	}
}

func TestOptionsExposeBehaviorAbilityModelFamilies(t *testing.T) {
	t.Parallel()

	svc := NewService(Dependencies{})
	result, err := svc.Options(context.Background(), KindBehaviorAbility)
	if err != nil {
		t.Fatalf("Options: %v", err)
	}
	if len(result.ModelFamilies) != 2 {
		t.Fatalf("model families = %#v", result.ModelFamilies)
	}
}

func TestCreateRejectsBehaviorAbilityChannelKind(t *testing.T) {
	t.Parallel()

	svc := NewService(Dependencies{})
	_, err := svc.Create(context.Background(), CreateModelDTO{
		Kind:  KindBehaviorAbility,
		Code:  "BA-001",
		Title: "invalid",
	})
	if err == nil {
		t.Fatal("Create(behavior_ability) error = nil, want rejection")
	}
	if !cberrors.IsCode(err, code.ErrInvalidArgument) {
		t.Fatalf("Create(behavior_ability) code = %v, want ErrInvalidArgument", cberrors.ParseCoder(err))
	}
}

func TestAPICatalogCapabilityMatrix(t *testing.T) {
	t.Parallel()

	for _, entry := range modelFamilyRegistryOptions() {
		entry := entry
		apiKind := entry.APIKind
		cap, ok := domain.FamilyCapabilityByKind(entry.Kind)
		if !ok {
			t.Fatalf("missing family capability for %q", entry.Kind)
		}
		t.Run(apiKind, func(t *testing.T) {
			t.Parallel()

			mapped, ok := APIKindToDomainKind(apiKind)
			if !ok {
				t.Fatalf("APIKindToDomainKind(%q) = false, want true", apiKind)
			}
			if mapped != entry.Kind {
				t.Fatalf("APIKindToDomainKind(%q) = %q, want %q", apiKind, mapped, entry.Kind)
			}
			if got := DomainKindToAPIKind(entry.Kind); got != apiKind {
				t.Fatalf("DomainKindToAPIKind(%q) = %q, want %q", entry.Kind, got, apiKind)
			}

			got, ok := registeredOptionForAPIKind(apiKind)
			if !ok {
				t.Fatalf("registeredOptionForAPIKind(%q) = false, want true", apiKind)
			}
			if got.OptionsEnabled != entry.OptionsEnabled {
				t.Fatalf("OptionsEnabled = %v, want %v", got.OptionsEnabled, entry.OptionsEnabled)
			}
			if got.Operations.CreateSupported != cap.CreateSupported {
				t.Fatalf("CreateSupported = %v, want %v", got.Operations.CreateSupported, cap.CreateSupported)
			}
			if got.Operations.PreviewSupported != entry.Operations.PreviewSupported {
				t.Fatalf("PreviewSupported = %v, want %v", got.Operations.PreviewSupported, entry.Operations.PreviewSupported)
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

	for _, entry := range modelFamilyRegistryOptions() {
		apiKind := entry.APIKind
		item, ok := optionByValue[apiKind]
		if !ok {
			t.Fatalf("Options().Kinds missing %q", apiKind)
		}
		if item.Label != entry.DisplayName {
			t.Fatalf("Options().Kinds[%q].Label = %q, want %q", apiKind, item.Label, entry.DisplayName)
		}
		wantDisabled := !entry.OptionsEnabled
		if item.Disabled != wantDisabled {
			t.Fatalf("Options().Kinds[%q].Disabled = %v, want %v", apiKind, item.Disabled, wantDisabled)
		}
	}
}

func TestCreateCapabilityPolicy(t *testing.T) {
	t.Parallel()

	for _, entry := range modelFamilyRegistryOptions() {
		entry := entry
		apiKind := entry.APIKind
		cap, ok := domain.FamilyCapabilityByKind(entry.Kind)
		if !ok {
			t.Fatalf("missing family capability for %q", entry.Kind)
		}
		t.Run(apiKind, func(t *testing.T) {
			t.Parallel()

			personalityStub := &personalityCommandStub{}
			cognitiveStub := &cognitiveCommandStub{}
			behavioralRatingStub := &behavioralRatingCommandStub{}
			svc := NewService(Dependencies{
				TypologyCommand:        personalityStub,
				TaskPerformanceCommand: cognitiveStub,
				NormingCommand:         behavioralRatingStub,
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
				case KindPersonality:
					if !personalityStub.createCalled {
						t.Fatal("personality command Create was not called")
					}
				case KindCognitive:
					if !cognitiveStub.createCalled {
						t.Fatal("cognitive command Create was not called")
					}
				case KindBehavioralRating:
					if !behavioralRatingStub.createCalled {
						t.Fatal("behavioral_rating command Create was not called")
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

func previewModelCode(apiKind string) string {
	switch apiKind {
	case KindPersonality:
		return "personality_demo"
	case KindCognitive:
		return "cognitive_demo"
	case KindBehavioralRating:
		return "behavioral_demo"
	default:
		return apiKind + "_demo"
	}
}

func TestPreviewReportCapabilityPolicy(t *testing.T) {
	t.Parallel()

	svc := NewService(Dependencies{
		TypologyCommand: &personalityCommandStub{},
	})

	for _, entry := range modelFamilyRegistryOptions() {
		entry := entry
		apiKind := entry.APIKind
		t.Run(apiKind, func(t *testing.T) {
			t.Parallel()

			_, err := svc.PreviewReport(context.Background(), previewModelCode(apiKind), json.RawMessage(`{}`))
			if entry.Operations.PreviewSupported {
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
