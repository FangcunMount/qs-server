package questionnaire

import (
	"slices"
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/validation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/surveyvalidation"
)

// RawSubmissionAnswer 是提交规格消费的原始答案输入。
type RawSubmissionAnswer struct {
	QuestionCode string
	QuestionType string
	Value        any
}

// PreparedSubmissionAnswer 是基于问卷规格归一化后的答案草稿。
type PreparedSubmissionAnswer struct {
	questionCode    meta.Code
	questionType    QuestionType
	value           any
	validationRules []validation.ValidationRule
}

func (a PreparedSubmissionAnswer) QuestionCode() meta.Code {
	return a.questionCode
}

func (a PreparedSubmissionAnswer) QuestionType() QuestionType {
	return a.questionType
}

func (a PreparedSubmissionAnswer) Value() any {
	return a.value
}

func (a PreparedSubmissionAnswer) ValidationRules() []validation.ValidationRule {
	return slices.Clone(a.validationRules)
}

type submissionQuestionSpec struct {
	code            meta.Code
	typ             QuestionType
	validationRules []validation.ValidationRule
	optionCodes     map[string]struct{}
	showController  *ShowController
}

// SubmissionSpec 描述一个已发布问卷版本可接受的提交规格。
type SubmissionSpec struct {
	code      meta.Code
	version   Version
	title     string
	questions map[string]submissionQuestionSpec
}

func (s SubmissionSpec) QuestionnaireCode() meta.Code {
	return s.code
}

func (s SubmissionSpec) QuestionnaireVersion() Version {
	return s.version
}

func (s SubmissionSpec) QuestionnaireTitle() string {
	return s.title
}

// PrepareAnswers delegates executable submission policy to the shared package
// so collection-server preflight and apiserver final validation cannot drift.
func (s SubmissionSpec) PrepareAnswers(rawAnswers []RawSubmissionAnswer) ([]PreparedSubmissionAnswer, error) {
	raw := make([]surveyvalidation.Answer, 0, len(rawAnswers))
	for _, answer := range rawAnswers {
		raw = append(raw, surveyvalidation.Answer{QuestionCode: answer.QuestionCode, QuestionType: answer.QuestionType, Value: answer.Value})
	}
	accepted, err := s.sharedSpec().Validate(raw)
	if err != nil {
		return nil, newError(ErrorKindInvalidAnswer, "%s", err)
	}
	prepared := make([]PreparedSubmissionAnswer, 0, len(accepted))
	for _, answer := range accepted {
		question := s.questions[answer.QuestionCode]
		prepared = append(prepared, PreparedSubmissionAnswer{
			questionCode: question.code, questionType: question.typ, value: answer.Value,
			validationRules: slices.Clone(question.validationRules),
		})
	}
	return prepared, nil
}

func (s SubmissionSpec) sharedSpec() surveyvalidation.Spec {
	questions := make([]surveyvalidation.Question, 0, len(s.questions))
	for _, question := range s.questions {
		optionCodes := make([]string, 0, len(question.optionCodes))
		for code := range question.optionCodes {
			optionCodes = append(optionCodes, code)
		}
		rules := make([]surveyvalidation.Rule, 0, len(question.validationRules))
		for _, rule := range question.validationRules {
			rules = append(rules, surveyvalidation.Rule{Type: string(rule.GetRuleType()), TargetValue: rule.GetTargetValue()})
		}
		questions = append(questions, surveyvalidation.Question{
			Code: question.code.Value(), Type: question.typ.Value(), OptionCodes: optionCodes, Rules: rules,
			ShowController: sharedShowController(question.showController),
		})
	}
	return surveyvalidation.Spec{QuestionnaireCode: s.code.Value(), QuestionnaireVersion: s.version.Value(), Questions: questions}
}

func sharedShowController(controller *ShowController) *surveyvalidation.ShowController {
	if controller == nil || controller.IsEmpty() {
		return nil
	}
	conditions := make([]surveyvalidation.ShowCondition, 0, len(controller.GetQuestions()))
	for _, condition := range controller.GetQuestions() {
		codes := make([]string, 0, len(condition.SelectOptionCodes))
		for _, code := range condition.SelectOptionCodes {
			codes = append(codes, code.Value())
		}
		conditions = append(conditions, surveyvalidation.ShowCondition{QuestionCode: condition.Code.Value(), OptionCodes: codes})
	}
	return &surveyvalidation.ShowController{Rule: controller.GetRule(), Conditions: conditions}
}

func optionCodesFromQuestion(question Question) map[string]struct{} {
	withOptions, ok := question.(HasOptions)
	if !ok {
		return nil
	}
	codes := make(map[string]struct{}, len(withOptions.GetOptions()))
	for _, option := range withOptions.GetOptions() {
		code := strings.TrimSpace(option.GetCode().Value())
		if code != "" {
			codes[code] = struct{}{}
		}
	}
	return codes
}

func showControllerFromQuestion(question Question) *ShowController {
	if question == nil {
		return nil
	}
	return question.GetShowController()
}

// EnsureSubmittable 校验问卷是否可提交。
func (q *Questionnaire) EnsureSubmittable() error {
	if q == nil {
		return newError(ErrorKindInvalidInput, "questionnaire is required")
	}
	if !q.IsPublished() {
		return newError(ErrorKindInvalidStatus, "only published questionnaire can be submitted")
	}
	if q.GetCode().IsEmpty() {
		return newError(ErrorKindInvalidCode, "questionnaire code cannot be empty")
	}
	if q.GetVersion().IsEmpty() {
		return newError(ErrorKindInvalidInput, "questionnaire version cannot be empty")
	}
	return nil
}

// BuildSubmissionSpec 为已发布问卷构造提交规格。
func (q *Questionnaire) BuildSubmissionSpec() (SubmissionSpec, error) {
	if err := q.EnsureSubmittable(); err != nil {
		return SubmissionSpec{}, err
	}
	questions := make(map[string]submissionQuestionSpec, len(q.GetQuestions()))
	for _, question := range q.GetQuestions() {
		if question == nil {
			continue
		}
		code := question.GetCode()
		questions[code.Value()] = submissionQuestionSpec{
			code:            code,
			typ:             question.GetType(),
			validationRules: slices.Clone(question.GetValidationRules()),
			optionCodes:     optionCodesFromQuestion(question),
			showController:  showControllerFromQuestion(question),
		}
	}
	return SubmissionSpec{
		code:      q.GetCode(),
		version:   q.GetVersion(),
		title:     q.GetTitle(),
		questions: questions,
	}, nil
}
