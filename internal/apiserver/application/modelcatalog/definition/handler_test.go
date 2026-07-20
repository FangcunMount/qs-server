package definition

import (
	"context"
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type stubHandler struct {
	kind domain.Kind
}

func (h stubHandler) Supports(identity domain.Identity) bool {
	return identity.Kind == h.kind
}

func (h stubHandler) ValidateForPublish(context.Context, *domain.AssessmentModel) []domain.DomainValidationIssue {
	return nil
}

func (h stubHandler) BuildSnapshotPayload(context.Context, *domain.AssessmentModel) (SnapshotBuildResult, error) {
	return SnapshotBuildResult{}, nil
}

func TestRegistryResolveByIdentity(t *testing.T) {
	t.Parallel()

	registry := NewRegistry(stubHandler{kind: domain.KindTypology})
	handler, ok := registry.Resolve(domain.Identity{Kind: domain.KindTypology, Algorithm: domain.AlgorithmPersonalityTypology})
	if !ok || handler == nil {
		t.Fatal("Resolve() did not return typology handler")
	}
	if _, ok := registry.Resolve(domain.Identity{Kind: domain.KindCognitive}); ok {
		t.Fatal("Resolve() should reject unsupported identity")
	}
}

func TestRegistryResolveBindingRejectsCrossKindAlgorithm(t *testing.T) {
	t.Parallel()

	registry := NewRegistry(
		ScaleDefinitionHandler{},
		BehavioralRatingDefinitionHandler{},
		CognitiveDefinitionHandler{},
		TypologyDefinitionHandler{},
	)
	cases := []domain.Identity{
		{Kind: domain.KindScale, Algorithm: domain.AlgorithmBrief2},
		{Kind: domain.KindBehavioralRating, Algorithm: domain.AlgorithmSPM},
		{Kind: domain.KindCognitive, Algorithm: domain.AlgorithmBrief2},
		{Kind: domain.KindTypology, SubKind: domain.SubKindTypology, Algorithm: domain.AlgorithmSPM},
	}
	for _, identity := range cases {
		if _, ok := registry.ResolveBinding(AlgorithmBindingFromIdentity(identity)); ok {
			t.Fatalf("ResolveBinding(%s/%s/%s) should reject incompatible binding", identity.Kind, identity.SubKind, identity.Algorithm)
		}
	}
}

func TestRegistryResolvesAllCanonicalDefinitionStrategies(t *testing.T) {
	t.Parallel()

	registry := NewRegistry(
		ScaleDefinitionHandler{},
		BehavioralRatingDefinitionHandler{},
		CognitiveDefinitionHandler{},
		TypologyDefinitionHandler{},
	)
	for _, identity := range []domain.Identity{
		{Kind: domain.KindScale, Algorithm: domain.AlgorithmScaleDefault},
		{Kind: domain.KindBehavioralRating, Algorithm: domain.AlgorithmBrief2},
		{Kind: domain.KindCognitive, Algorithm: domain.AlgorithmSPM},
		{Kind: domain.KindTypology, SubKind: domain.SubKindTypology, Algorithm: domain.AlgorithmPersonalityTypology},
	} {
		binding := AlgorithmBindingFromIdentity(identity)
		handler, ok := registry.ResolveBinding(binding)
		if !ok || handler == nil {
			t.Fatalf("ResolveBinding(%s/%s/%s) = no handler", identity.Kind, identity.SubKind, identity.Algorithm)
		}
		if binding.Family == "" {
			t.Fatalf("ResolveBinding should derive AlgorithmFamily for %s/%s/%s", identity.Kind, identity.SubKind, identity.Algorithm)
		}
	}
}

func TestValidationErrorPreservesIssues(t *testing.T) {
	t.Parallel()

	err := NewValidationError([]domain.DomainValidationIssue{{
		Field: "definition.payload", Message: "payload invalid", Code: "definition.payload.invalid", Level: domain.ValidationLevelError,
	}})
	if err == nil {
		t.Fatal("NewValidationError() returned nil")
	}
	validationErr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("error type = %T, want *ValidationError", err)
	}
	if validationErr.Error() != "payload invalid" {
		t.Fatalf("Error() = %q", validationErr.Error())
	}
	if validationErr.Issues[0].Code != "definition.payload.invalid" {
		t.Fatalf("issue = %#v", validationErr.Issues[0])
	}
}
