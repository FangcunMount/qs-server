package definition

import (
	"context"
	"fmt"

	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/capability"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	domainfactor "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
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
// MC-R018: new publishes must use canonical algorithms; retained-read aliases fail here.
func ValidateAlgorithmBinding(model *domain.AssessmentModel) []domain.DomainValidationIssue {
	if model == nil {
		return nil
	}
	issues := make([]domain.DomainValidationIssue, 0)
	issues = append(issues, validatePublishAlgorithmPolicy(model)...)
	if model.DefinitionV2 == nil {
		return issues
	}
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
	}
	return issues
}

func validatePublishAlgorithmPolicy(model *domain.AssessmentModel) []domain.DomainValidationIssue {
	if model == nil {
		return nil
	}
	policy := domain.ClassifyAlgorithmWritePolicy(model.Kind, model.Algorithm)
	switch policy {
	case domain.AlgorithmWriteCanonical:
		return nil
	case domain.AlgorithmWriteDraftOK:
		return []domain.DomainValidationIssue{{
			Field: "algorithm", Code: "algorithm.publish.required",
			Message: publishAlgorithmRequiredMessage(model.Kind), Level: domain.ValidationLevelError,
		}}
	default:
		if domain.IsRetainedReadAliasAlgorithm(model.Algorithm) {
			code := "algorithm.publish.legacy_alias"
			if model.Kind == domain.KindBehavioralRating {
				code = "behavioral_rating.algorithm.required"
			}
			return []domain.DomainValidationIssue{{
				Field: "algorithm", Code: code,
				Message: publishLegacyAlgorithmMessage(model.Kind, model.Algorithm), Level: domain.ValidationLevelError,
			}}
		}
		return []domain.DomainValidationIssue{{
			Field: "algorithm", Code: "algorithm.publish.unsupported",
			Message: fmt.Sprintf("algorithm %q is not supported for publish on kind %s", model.Algorithm, model.Kind),
			Level:   domain.ValidationLevelError,
		}}
	}
}

func publishAlgorithmRequiredMessage(kind domain.Kind) string {
	switch kind {
	case domain.KindTypology:
		return "typology 发布必须使用 personality_typology"
	case domain.KindBehavioralRating:
		return "behavioral_rating 发布必须指定真实 Algorithm（brief2 或 spm_sensory）"
	case domain.KindCognitive:
		return "cognitive 发布必须指定真实 Algorithm（spm）"
	case domain.KindScale:
		return "scale 发布必须指定 Algorithm（scale_default）"
	default:
		return "algorithm is required for publish"
	}
}

func publishLegacyAlgorithmMessage(kind domain.Kind, algorithm domain.Algorithm) string {
	switch kind {
	case domain.KindTypology:
		return fmt.Sprintf("新发布不允许 legacy typology algorithm %q；请使用 personality_typology", algorithm)
	case domain.KindBehavioralRating:
		if algorithm == domain.AlgorithmBehavioralRatingDefault {
			return "新发布不允许 behavioral_rating_default；请使用 brief2 或 spm_sensory"
		}
		return fmt.Sprintf("behavioral_rating algorithm %q 不受支持；请使用 brief2 或 spm_sensory", algorithm)
	default:
		return fmt.Sprintf("新发布不允许 retained-read algorithm %q", algorithm)
	}
}

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

// ValidateStrategyCapability checks Measure Scoring strategies and executable
// Scoring requirements against the Calculation capability catalog (MC-R014).
func ValidateStrategyCapability(model *domain.AssessmentModel, path capability.Path) []domain.DomainValidationIssue {
	if model == nil || model.DefinitionV2 == nil || path == "" {
		return nil
	}
	measure := model.DefinitionV2.Measure
	hierarchy := domainfactor.ValidateScoringStrategyCapability(path, measure.Scoring)
	hierarchy = append(hierarchy, domainfactor.ValidateExecutableScoringCapability(path, measure.Factors, measure.Scoring)...)
	if len(hierarchy) == 0 {
		return nil
	}
	out := make([]domain.DomainValidationIssue, 0, len(hierarchy))
	for _, issue := range hierarchy {
		out = append(out, domain.DomainValidationIssue{
			Field: issue.Field, Code: issue.Code, Message: issue.Message, Level: domain.ValidationLevelError,
		})
	}
	return out
}

