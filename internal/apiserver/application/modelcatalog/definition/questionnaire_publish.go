package definition

import (
	"context"
	"fmt"

	questionnaireapp "github.com/FangcunMount/qs-server/internal/apiserver/application/survey/questionnaire"
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/questionnaireref"
)

// loadPublishedQuestionnaire loads the bound published questionnaire for publish guards.
// Empty binding yields (nil, nil) so callers can rely on model.ValidateForPublish first.
func loadPublishedQuestionnaire(
	ctx context.Context,
	query questionnaireapp.QuestionnaireQueryService,
	codeValue, version string,
) (*questionnaireapp.QuestionnaireResult, []domain.DomainValidationIssue) {
	if codeValue == "" || version == "" {
		return nil, nil
	}
	if query == nil {
		return nil, []domain.DomainValidationIssue{{
			Field: "binding.questionnaire", Message: "问卷查询服务未配置",
			Code: "binding.questionnaire_query.unavailable", Level: domain.ValidationLevelError,
		}}
	}
	questionnaire, err := query.GetPublishedByCodeVersion(ctx, codeValue, version)
	if err != nil || questionnaire == nil {
		return nil, []domain.DomainValidationIssue{{
			Field: "binding.questionnaire", Message: "绑定问卷不存在或未发布",
			Code: "binding.questionnaire.not_found", Level: domain.ValidationLevelError,
		}}
	}
	if len(questionnaire.Questions) == 0 {
		return nil, []domain.DomainValidationIssue{{
			Field: "binding.questionnaire", Message: "绑定问卷题目不能为空",
			Code: "binding.questionnaire.questions.required", Level: domain.ValidationLevelError,
		}}
	}
	return questionnaire, nil
}

func questionIndexFromResult(questionnaire *questionnaireapp.QuestionnaireResult) questionnaireref.Index {
	if questionnaire == nil {
		return questionnaireref.Index{}
	}
	questions := make([]questionnaireref.Question, 0, len(questionnaire.Questions))
	for _, question := range questionnaire.Questions {
		item := questionnaireref.Question{Code: question.Code, Type: question.Type, OptionCodes: make([]string, 0, len(question.Options))}
		for _, option := range question.Options {
			item.OptionCodes = append(item.OptionCodes, option.Value)
		}
		questions = append(questions, item)
	}
	return questionnaireref.NewIndex(questions)
}

// validateDefinitionQuestionnaireRefs checks DefinitionV2 measure/SPM question and option refs
// against the bound published questionnaire version (MC-R007 batch 2).
func validateDefinitionQuestionnaireRefs(
	ctx context.Context,
	query questionnaireapp.QuestionnaireQueryService,
	model *domain.AssessmentModel,
) []domain.DomainValidationIssue {
	if model == nil || model.DefinitionV2 == nil {
		return nil
	}
	refs := collectDefinitionQuestionnaireRefs(model.DefinitionV2)
	if len(refs) == 0 {
		return nil
	}
	questionnaire, issues := loadPublishedQuestionnaire(ctx, query, model.Binding.QuestionnaireCode, model.Binding.QuestionnaireVersion)
	if len(issues) > 0 {
		return issues
	}
	if questionnaire == nil {
		return nil
	}
	return questionIndexFromResult(questionnaire).ValidateRefs(refs)
}

func collectDefinitionQuestionnaireRefs(def *modeldefinition.Definition) []questionnaireref.Ref {
	if def == nil {
		return nil
	}
	refs := make([]questionnaireref.Ref, 0)
	for _, rule := range def.Measure.Scoring {
		for _, source := range rule.Sources {
			if source.Kind != factor.ScoringSourceQuestion {
				continue
			}
			field := fmt.Sprintf("measure.scoring[%s].sources", rule.FactorCode)
			refs = append(refs, questionnaireref.Ref{Field: field, QuestionCode: source.Code})
			for optionCode := range source.OptionScores {
				refs = append(refs, questionnaireref.Ref{Field: field + ".option_scores", QuestionCode: source.Code, OptionCode: optionCode})
			}
		}
	}
	if spm := def.Execution.SPM; spm != nil {
		for setIndex, set := range spm.ItemSets {
			for itemIndex, item := range set.Items {
				field := fmt.Sprintf("execution.spm.item_sets[%d].items[%d]", setIndex, itemIndex)
				refs = append(refs, questionnaireref.Ref{Field: field + ".question_code", QuestionCode: item.QuestionCode})
				if item.CorrectOptionCode != "" {
					refs = append(refs, questionnaireref.Ref{
						Field: field + ".correct_option_code", QuestionCode: item.QuestionCode, OptionCode: item.CorrectOptionCode,
					})
				}
			}
		}
	}
	return refs
}
