package modelcatalog

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestDraftMapperRoundTrip(t *testing.T) {
	original, err := domain.NewAssessmentModel(domain.NewAssessmentModelInput{
		Code: "personality_demo", Kind: domain.KindPersonality,
		SubKind: domain.SubKindTypology, Algorithm: domain.AlgorithmMBTI, Title: "Demo",
	})
	if err != nil {
		t.Fatalf("NewAssessmentModel: %v", err)
	}
	_ = original.UpdateDefinition(domain.DefinitionPayload{
		Format: domain.PayloadFormatPersonalityTypologyV1,
		Data:   []byte(`{"decision":{"kind":"pole_composition"}}`),
	}, original.CreatedAt)

	mapper := NewDraftMapper()
	po := mapper.ToPO(original)
	got := mapper.ToDomain(po)
	if got.Code != original.Code || got.Algorithm != original.Algorithm {
		t.Fatalf("round trip = %#v", got)
	}
}
