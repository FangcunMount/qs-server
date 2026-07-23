package definition

import (
	"testing"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

func TestRuntimeMaterializerRejectsTypologyWrongSubKind(t *testing.T) {
	model := &domain.AssessmentModel{Kind: domain.KindTypology, Algorithm: domain.AlgorithmPersonalityTypology, DefinitionV2: &domain.Definition{}}
	if _, err := (RuntimeMaterializer{}).MaterializeTypology(model); err == nil {
		t.Fatal("expected sub_kind rejection")
	}
}

func TestRuntimeMaterializerScaleDefaultsAlgorithm(t *testing.T) {
	model := publishableScaleShell()
	model.Algorithm = ""
	model.DefinitionV2 = completeScaleDefinition()
	result, err := (RuntimeMaterializer{}).MaterializeScale(model)
	if err != nil {
		t.Fatalf("MaterializeScale: %v", err)
	}
	if result.Algorithm != domain.AlgorithmScaleDefault || result.AlgorithmFamily != domain.AlgorithmFamilyFactorScoring {
		t.Fatalf("materialization = %#v", result)
	}
}
