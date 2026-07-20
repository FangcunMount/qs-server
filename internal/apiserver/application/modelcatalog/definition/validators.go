package definition

import (
	"context"
	"fmt"

	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// modelRequiredIssue is the shared nil-model publish error.
func modelRequiredIssue() domain.DomainValidationIssue {
	return domain.DomainValidationIssue{
		Field: "model", Message: "模型不能为空", Code: "model.required", Level: domain.ValidationLevelError,
	}
}

// AppendDecisionKindIssues records DecisionKind derivation failures.
func AppendDecisionKindIssues(model *domain.AssessmentModel, issues []domain.DomainValidationIssue) []domain.DomainValidationIssue {
	if model == nil {
		return issues
	}
	if _, err := model.DecisionKindForDefinition(); err != nil {
		return append(issues, domain.DomainValidationIssue{
			Field: "definition_v2.conclusions", Code: "definition_v2.decision.invalid",
			Message: err.Error(), Level: domain.ValidationLevelError,
		})
	}
	return issues
}

// ValidateAlgorithmBinding checks Algorithm / ExecutionSpec publish rules by
// AlgorithmBinding (Algorithm + derived Family), not by Kind switch.
func ValidateAlgorithmBinding(model *domain.AssessmentModel) []domain.DomainValidationIssue {
	if model == nil || model.DefinitionV2 == nil {
		return nil
	}
	issues := make([]domain.DomainValidationIssue, 0)
	switch model.Algorithm {
	case domain.AlgorithmBrief2:
		if model.DefinitionV2.Execution.Brief2 == nil {
			issues = append(issues, domain.DomainValidationIssue{
				Field: "execution.brief2", Code: "brief2.execution.required",
				Message: "BRIEF-2 execution spec is required", Level: domain.ValidationLevelError,
			})
		}
	case domain.AlgorithmSPM:
		if model.DefinitionV2.Execution.SPM == nil {
			issues = append(issues, domain.DomainValidationIssue{
				Field: "execution.spm", Code: "spm.execution.required",
				Message: "SPM execution spec is required", Level: domain.ValidationLevelError,
			})
		}
	case domain.AlgorithmSPMSensory:
		// publishable factor_norm algorithm; no extra ExecutionSpec gate here
	case domain.AlgorithmBehavioralRatingDefault, "":
		if isFactorNormFamily(model) {
			if err := requireBehavioralPublishAlgorithm(model.Algorithm); err != nil {
				issues = append(issues, domain.DomainValidationIssue{
					Field: "algorithm", Code: "behavioral_rating.algorithm.required",
					Message: err.Error(), Level: domain.ValidationLevelError,
				})
			}
		}
	default:
		if isFactorNormFamily(model) {
			if err := requireBehavioralPublishAlgorithm(model.Algorithm); err != nil {
				issues = append(issues, domain.DomainValidationIssue{
					Field: "algorithm", Code: "behavioral_rating.algorithm.required",
					Message: err.Error(), Level: domain.ValidationLevelError,
				})
			}
		}
	}
	return issues
}

func isFactorNormFamily(model *domain.AssessmentModel) bool {
	if model == nil {
		return false
	}
	family, ok := domain.AlgorithmFamilyFromIdentity(model.Kind, model.SubKind, model.Algorithm)
	return ok && family == domain.AlgorithmFamilyFactorNorm
}

// ValidateBehavioralSemantic checks behavioral NormRef / conclusion contracts
// without loading Norm tables (existence is handled by NormCompatibility).
func ValidateBehavioralSemantic(model *domain.AssessmentModel) []domain.DomainValidationIssue {
	if model == nil || model.DefinitionV2 == nil {
		return nil
	}
	issues := make([]domain.DomainValidationIssue, 0)
	def := model.DefinitionV2
	if len(def.Calibration.NormRefs) == 0 {
		issues = append(issues, domain.DomainValidationIssue{
			Field: "calibration.norm_refs", Code: "behavioral_rating.norm_refs.required",
			Message: "behavioral_rating 必须配置至少一条 NormRef；原始分区间模型应使用 scale",
			Level:   domain.ValidationLevelError,
		})
	}
	normRefFactors := map[string]struct{}{}
	for _, ref := range def.Calibration.NormRefs {
		if ref.FactorCode != "" {
			normRefFactors[ref.FactorCode] = struct{}{}
		}
	}
	for _, item := range def.Conclusions {
		normConclusion, ok := item.(domain.NormConclusion)
		if !ok {
			continue
		}
		if _, ok := normRefFactors[normConclusion.FactorCode]; normConclusion.FactorCode != "" && !ok {
			issues = append(issues, domain.DomainValidationIssue{
				Field: "definition_v2.conclusions", Code: "behavioral_rating.conclusion.norm_ref.missing",
				Message: fmt.Sprintf("NormConclusion factor %s 缺少对应 NormRef", normConclusion.FactorCode),
				Level:   domain.ValidationLevelError,
			})
		}
	}
	return issues
}

// ValidateQuestionnaireMeasure checks Definition question/option refs against the
// bound published questionnaire version.
func ValidateQuestionnaireMeasure(
	ctx context.Context,
	query questionnaireapp.QuestionnaireQueryService,
	model *domain.AssessmentModel,
) []domain.DomainValidationIssue {
	return validateDefinitionQuestionnaireRefs(ctx, query, model)
}

// ValidateDefinitionForPublish is the shared DefinitionV2 + optional Norm path.
func ValidateDefinitionForPublish(
	ctx context.Context,
	model *domain.AssessmentModel,
	norms port.NormRepository,
) []domain.DomainValidationIssue {
	if model == nil {
		return nil
	}
	if norms == nil {
		return ValidateDefinitionV2ForPublish(ctx, model.DefinitionV2, nil)
	}
	return ValidateDefinitionV2ForPublishWithModel(ctx, model, model.DefinitionV2, norms)
}

func requireBehavioralPublishAlgorithm(algorithm domain.Algorithm) error {
	switch algorithm {
	case domain.AlgorithmBrief2, domain.AlgorithmSPMSensory:
		return nil
	case "":
		return fmt.Errorf("behavioral_rating 发布必须指定真实 Algorithm（brief2 或 spm_sensory）")
	case domain.AlgorithmBehavioralRatingDefault:
		return fmt.Errorf("新发布不允许 behavioral_rating_default；请使用 brief2 或 spm_sensory")
	default:
		return fmt.Errorf("behavioral_rating algorithm %q 不受支持；请使用 brief2 或 spm_sensory", algorithm)
	}
}
