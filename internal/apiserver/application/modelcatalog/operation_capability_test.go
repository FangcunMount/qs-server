package modelcatalog

import (
	"context"
	"encoding/json"
	"testing"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

func TestUpdateDefinitionCapabilityPolicy(t *testing.T) {
	t.Parallel()

	for _, cap := range domain.DefaultCapabilities() {
		cap := cap
		apiKind := DomainKindToAPIKind(cap.Kind)
		t.Run(apiKind, func(t *testing.T) {
			t.Parallel()

			svc := NewService(Dependencies{
				BehaviorCommand:         &behaviorCommandStub{},
				PersonalityCommand:      &personalityCommandStub{},
				CognitiveCommand:        &cognitiveCommandStub{},
				BehavioralRatingCommand: &behavioralRatingCommandStub{},
			})

			_, err := svc.UpdateDefinition(context.Background(), "capability_"+apiKind, DefinitionDTO{
				Kind:    apiKind,
				Payload: json.RawMessage(`{"items":[]}`),
			})

			if cap.DefinitionUpdateSupported {
				if err != nil {
					t.Fatalf("UpdateDefinition(%q) error = %v, want success", apiKind, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("UpdateDefinition(%q) error = nil, want rejection", apiKind)
			}
			if !cberrors.IsCode(err, code.ErrInvalidArgument) {
				t.Fatalf("UpdateDefinition(%q) code = %v, want ErrInvalidArgument", apiKind, cberrors.ParseCoder(err))
			}
		})
	}
}

func TestUpdateDefinitionDoesNotDefaultToBehaviorAbility(t *testing.T) {
	t.Parallel()

	svc := NewService(Dependencies{
		BehaviorCommand:    &behaviorCommandStub{},
		PersonalityCommand: &personalityCommandStub{},
		CognitiveCommand:   &cognitiveCommandStub{},
	})

	_, err := svc.UpdateDefinition(context.Background(), "missing_model", DefinitionDTO{
		Payload: json.RawMessage(`{}`),
	})
	if err == nil {
		t.Fatal("expected unknown model update to be rejected")
	}
	if !cberrors.IsCode(err, code.ErrInvalidArgument) {
		t.Fatalf("code = %v, want ErrInvalidArgument", cberrors.ParseCoder(err))
	}
}

func TestPublishCapabilityPolicy(t *testing.T) {
	t.Parallel()

	for _, cap := range domain.DefaultCapabilities() {
		cap := cap
		apiKind := DomainKindToAPIKind(cap.Kind)
		t.Run(apiKind, func(t *testing.T) {
			t.Parallel()

			svc := NewService(Dependencies{
				BehaviorCommand:         &behaviorCommandStub{},
				PersonalityCommand:      &personalityCommandStub{},
				CognitiveCommand:        &cognitiveCommandStub{},
				BehavioralRatingCommand: &behavioralRatingCommandStub{},
			})

			_, err := svc.Publish(context.Background(), previewModelCode(apiKind))
			if cap.PublishSupported {
				if err != nil {
					t.Fatalf("Publish(%q) error = %v, want success path", apiKind, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Publish(%q) error = nil, want rejection", apiKind)
			}
			if !cberrors.IsCode(err, code.ErrInvalidArgument) {
				t.Fatalf("Publish(%q) code = %v, want ErrInvalidArgument", apiKind, cberrors.ParseCoder(err))
			}
		})
	}
}

func TestBindQuestionnaireCapabilityPolicy(t *testing.T) {
	t.Parallel()

	for _, cap := range domain.DefaultCapabilities() {
		cap := cap
		apiKind := DomainKindToAPIKind(cap.Kind)
		t.Run(apiKind, func(t *testing.T) {
			t.Parallel()

			svc := NewService(Dependencies{
				BehaviorCommand:         &behaviorCommandStub{},
				PersonalityCommand:      &personalityCommandStub{},
				CognitiveCommand:        &cognitiveCommandStub{},
				BehavioralRatingCommand: &behavioralRatingCommandStub{},
			})

			_, err := svc.BindQuestionnaire(context.Background(), BindQuestionnaireDTO{
				Code:                 previewModelCode(apiKind),
				QuestionnaireCode:    "QNR-1",
				QuestionnaireVersion: "1.0.0",
			})
			if cap.BindQuestionnaire {
				if err != nil {
					t.Fatalf("BindQuestionnaire(%q) error = %v, want success path", apiKind, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("BindQuestionnaire(%q) error = nil, want rejection", apiKind)
			}
			if !cberrors.IsCode(err, code.ErrInvalidArgument) {
				t.Fatalf("BindQuestionnaire(%q) code = %v, want ErrInvalidArgument", apiKind, cberrors.ParseCoder(err))
			}
		})
	}
}

func TestRequireCatalogOperationRejectsUnknownKind(t *testing.T) {
	t.Parallel()

	if err := requireCatalogOperation("cognitive", domain.CatalogOpCreate); err != nil {
		t.Fatal("expected cognitive create to be allowed")
	}
	if err := requireCatalogOperation("custom", domain.CatalogOpCreate); err == nil {
		t.Fatal("expected custom create to be rejected")
	}
	if err := requireCatalogOperation(KindMedicalScale, domain.CatalogOpUpdateDefinition); err == nil {
		t.Fatal("expected medical_scale definition update to be rejected")
	}
}
