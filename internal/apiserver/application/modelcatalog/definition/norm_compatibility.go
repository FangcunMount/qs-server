package definition

import (
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
)

// CheckNormCompatibility validates Model Kind/Algorithm/FormVariant/Factor against a Norm table.
// Raven SPM is a controlled cognitive extension, not a removal of identity checks.
func CheckNormCompatibility(model *domain.AssessmentModel, table *norm.Norm, ref norm.Ref) []domain.DomainValidationIssue {
	if model == nil || table == nil {
		return nil
	}
	issues := make([]domain.DomainValidationIssue, 0, 4)
	if table.Kind != "" && table.Kind != model.Kind {
		issues = append(issues, domain.DomainValidationIssue{
			Field: "calibration.norm_refs", Code: "norm.kind.mismatch",
			Message: fmt.Sprintf("常模 Kind %s 与模型 Kind %s 不兼容", table.Kind, model.Kind),
			Level:   domain.ValidationLevelError,
		})
	}
	modelAlgorithm := effectiveNormAlgorithm(model)
	if table.Algorithm != "" && modelAlgorithm != "" && table.Algorithm != modelAlgorithm {
		issues = append(issues, domain.DomainValidationIssue{
			Field: "calibration.norm_refs", Code: "norm.algorithm.mismatch",
			Message: fmt.Sprintf("常模 Algorithm %s 与模型 Algorithm %s 不兼容", table.Algorithm, modelAlgorithm),
			Level:   domain.ValidationLevelError,
		})
	}
	if formVariant := definitionFormVariant(model); formVariant != "" && table.FormVariant != "" && formVariant != table.FormVariant {
		issues = append(issues, domain.DomainValidationIssue{
			Field: "calibration.norm_refs", Code: "norm.form_variant.mismatch",
			Message: fmt.Sprintf("常模 FormVariant %s 与模型 FormVariant %s 不兼容", table.FormVariant, formVariant),
			Level:   domain.ValidationLevelError,
		})
	}
	if ref.FactorCode != "" && !normTableHasFactor(table, ref.FactorCode) {
		issues = append(issues, domain.DomainValidationIssue{
			Field: "calibration.norm_refs", Code: "norm.factor.missing",
			Message: fmt.Sprintf("NormRef factor %s 不在常模表 %s 中", ref.FactorCode, ref.NormTableVersion),
			Level:   domain.ValidationLevelError,
		})
	}
	return issues
}

func effectiveNormAlgorithm(model *domain.AssessmentModel) domain.Algorithm {
	if model == nil {
		return ""
	}
	if model.Algorithm != "" {
		return model.Algorithm
	}
	switch model.Kind {
	case domain.KindCognitive:
		return domain.AlgorithmSPM
	default:
		return ""
	}
}

func definitionFormVariant(model *domain.AssessmentModel) string {
	if model == nil || model.DefinitionV2 == nil || model.DefinitionV2.Execution.Brief2 == nil {
		return ""
	}
	return model.DefinitionV2.Execution.Brief2.FormVariant
}

func normTableHasFactor(table *norm.Norm, factorCode string) bool {
	if table == nil || factorCode == "" {
		return false
	}
	for _, factor := range table.Factors {
		if factor.FactorCode == factorCode {
			return true
		}
	}
	return false
}
