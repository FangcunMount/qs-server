package definition

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestCompatibilityPayloadProjectorRejectsTypologyWrongSubKind(t *testing.T) {
	model := &domain.AssessmentModel{
		Kind:         domain.KindTypology,
		SubKind:      domain.SubKindEmpty,
		Algorithm:    domain.AlgorithmMBTI,
		DefinitionV2: &domain.Definition{},
	}
	_, err := (CompatibilityPayloadProjector{}).ProjectTypology(model)
	if err == nil {
		t.Fatal("expected sub_kind rejection")
	}
}

func TestCompatibilityPayloadProjectorScaleDefaultsAlgorithm(t *testing.T) {
	model := publishableScaleShell()
	model.Algorithm = ""
	model.DefinitionV2 = completeScaleDefinition()
	result, err := (CompatibilityPayloadProjector{}).ProjectScale(model)
	if err != nil {
		t.Fatalf("ProjectScale: %v", err)
	}
	if result.Algorithm != domain.AlgorithmScaleDefault {
		t.Fatalf("algorithm = %s, want %s", result.Algorithm, domain.AlgorithmScaleDefault)
	}
	if result.PayloadFormat != domain.PayloadFormatAssessmentScaleV1 {
		t.Fatalf("payload format = %s", result.PayloadFormat)
	}
}
