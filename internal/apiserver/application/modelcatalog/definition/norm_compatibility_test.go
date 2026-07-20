package definition_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/definition"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
)

func TestCheckNormCompatibilityRejectsKindMismatch(t *testing.T) {
	t.Parallel()
	model := &domain.AssessmentModel{Kind: domain.KindBehavioralRating, Algorithm: domain.AlgorithmBrief2}
	table := &norm.Norm{Kind: domain.KindCognitive, Algorithm: domain.AlgorithmSPM, FormVariant: "standard", Factors: []norm.FactorTable{{FactorCode: "total"}}}
	issues := definition.CheckNormCompatibility(model, table, norm.Ref{FactorCode: "total", NormTableVersion: "spm-1"})
	if !hasIssueCode(issues, "norm.kind.mismatch") {
		t.Fatalf("issues = %#v, want norm.kind.mismatch", issues)
	}
}

func TestCheckNormCompatibilityRejectsFormVariantMismatch(t *testing.T) {
	t.Parallel()
	model := &domain.AssessmentModel{
		Kind: domain.KindBehavioralRating, Algorithm: domain.AlgorithmBrief2,
		DefinitionV2: &modeldefinition.Definition{Execution: modeldefinition.ExecutionSpec{Brief2: &modeldefinition.Brief2Spec{FormVariant: "parent"}}},
	}
	table := &norm.Norm{Kind: domain.KindBehavioralRating, Algorithm: domain.AlgorithmBrief2, FormVariant: "teacher", Factors: []norm.FactorTable{{FactorCode: "gec"}}}
	issues := definition.CheckNormCompatibility(model, table, norm.Ref{FactorCode: "gec", NormTableVersion: "brief2-1"})
	if !hasIssueCode(issues, "norm.form_variant.mismatch") {
		t.Fatalf("issues = %#v, want norm.form_variant.mismatch", issues)
	}
}

func TestCheckNormCompatibilityAcceptsCognitiveSPM(t *testing.T) {
	t.Parallel()
	model := &domain.AssessmentModel{Kind: domain.KindCognitive, Algorithm: domain.AlgorithmSPM}
	table := &norm.Norm{Kind: domain.KindCognitive, Algorithm: domain.AlgorithmSPM, FormVariant: "standard", Factors: []norm.FactorTable{{FactorCode: "total"}}}
	issues := definition.CheckNormCompatibility(model, table, norm.Ref{FactorCode: "total", NormTableVersion: "spm-1"})
	if len(issues) != 0 {
		t.Fatalf("issues = %#v, want none", issues)
	}
}
