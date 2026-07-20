package definition

import (
	"fmt"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
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
	issues = append(issues, checkScoreBasisProducible(model, table, ref)...)
	return issues
}

func checkScoreBasisProducible(model *domain.AssessmentModel, table *norm.Norm, ref norm.Ref) []domain.DomainValidationIssue {
	if model == nil || model.DefinitionV2 == nil || table == nil || ref.FactorCode == "" {
		return nil
	}
	factorTable, ok := normFactorTable(table, ref.FactorCode)
	if !ok {
		return nil
	}
	issues := make([]domain.DomainValidationIssue, 0)
	for _, item := range model.DefinitionV2.Conclusions {
		basis, factorCode, kind := conclusionScoreBasis(item)
		if factorCode == "" || factorCode != ref.FactorCode || basis == "" || basis == conclusion.ScoreBasisRaw {
			continue
		}
		if errMsg := scoreBasisUnsupported(model, factorTable, basis); errMsg != "" {
			issues = append(issues, domain.DomainValidationIssue{
				Field: "definition_v2.conclusions", Code: "norm.score_basis.unsupported",
				Message: fmt.Sprintf("%s conclusion factor %s: %s", kind, factorCode, errMsg),
				Level:   domain.ValidationLevelError,
			})
		}
	}
	return issues
}

func conclusionScoreBasis(item conclusion.Conclusion) (conclusion.ScoreBasis, string, string) {
	switch typed := item.(type) {
	case conclusion.NormConclusion:
		return typed.ScoreBasis, typed.FactorCode, "norm"
	case conclusion.AbilityConclusion:
		return typed.ScoreBasis, typed.FactorCode, "ability"
	default:
		return "", "", ""
	}
}

func scoreBasisUnsupported(model *domain.AssessmentModel, factorTable norm.FactorTable, basis conclusion.ScoreBasis) string {
	spmRuntime := effectiveNormAlgorithm(model) == domain.AlgorithmSPM
	switch basis {
	case conclusion.ScoreBasisTScore:
		if spmRuntime {
			return "cognitive+spm 运行时不产生 T 分，不能使用 score_basis=t_score"
		}
		if len(factorTable.Lookup) == 0 && len(factorTable.Bands) == 0 {
			return "常模表无法产生 T 分"
		}
		return ""
	case conclusion.ScoreBasisPercentile:
		if len(factorTable.Lookup) == 0 && len(factorTable.Bands) == 0 {
			return "常模表无法产生百分位"
		}
		return ""
	case conclusion.ScoreBasisStandardScore:
		if factorHasStandardScore(factorTable) {
			return ""
		}
		return "常模表未提供 standard_score，不能使用 score_basis=standard_score"
	default:
		return ""
	}
}

func factorHasStandardScore(factorTable norm.FactorTable) bool {
	for _, row := range factorTable.Lookup {
		if row.StandardScore != nil {
			return true
		}
	}
	return false
}

func effectiveNormAlgorithm(model *domain.AssessmentModel) domain.Algorithm {
	if model == nil {
		return ""
	}
	if model.Algorithm != "" {
		return model.Algorithm
	}
	family, ok := domain.AlgorithmFamilyFromIdentity(model.Kind, model.SubKind, model.Algorithm)
	if ok && family == domain.AlgorithmFamilyTaskPerformance {
		return domain.AlgorithmSPM
	}
	return ""
}

func definitionFormVariant(model *domain.AssessmentModel) string {
	if model == nil || model.DefinitionV2 == nil || model.DefinitionV2.Execution.Brief2 == nil {
		return ""
	}
	return model.DefinitionV2.Execution.Brief2.FormVariant
}

func normTableHasFactor(table *norm.Norm, factorCode string) bool {
	_, ok := normFactorTable(table, factorCode)
	return ok
}

func normFactorTable(table *norm.Norm, factorCode string) (norm.FactorTable, bool) {
	if table == nil || factorCode == "" {
		return norm.FactorTable{}, false
	}
	for _, factor := range table.Factors {
		if factor.FactorCode == factorCode {
			return factor, true
		}
	}
	return norm.FactorTable{}, false
}
