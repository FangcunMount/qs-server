package questionnaire

import (
	"slices"
	"strings"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/validation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
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

// PrepareAnswers 按提交规格归一化答案，并拒绝未知问题或客户端题型不一致。
func (s SubmissionSpec) PrepareAnswers(rawAnswers []RawSubmissionAnswer) ([]PreparedSubmissionAnswer, error) {
	prepared := make([]PreparedSubmissionAnswer, 0, len(rawAnswers))
	for _, raw := range rawAnswers {
		questionCode := strings.TrimSpace(raw.QuestionCode)
		if questionCode == "" {
			return nil, newError(ErrorKindInvalidQuestion, "question code cannot be empty")
		}
		question, ok := s.questions[questionCode]
		if !ok {
			return nil, newError(ErrorKindQuestionNotFound, "question %s is not in questionnaire", questionCode)
		}
		if strings.TrimSpace(raw.QuestionType) == "" {
			return nil, newError(ErrorKindInvalidQuestion, "question %s type cannot be empty", questionCode)
		}
		if raw.QuestionType != question.typ.Value() {
			return nil, newError(ErrorKindInvalidQuestion, "question %s type mismatch: got %s, want %s", questionCode, raw.QuestionType, question.typ.Value())
		}
		if err := validateOptionSelection(question, raw.Value); err != nil {
			return nil, err
		}
		prepared = append(prepared, PreparedSubmissionAnswer{
			questionCode:    question.code,
			questionType:    question.typ,
			value:           raw.Value,
			validationRules: slices.Clone(question.validationRules),
		})
	}
	if err := ensureVisibleRequiredQuestionsAnswered(s.questions, rawAnswers); err != nil {
		return nil, err
	}
	return prepared, nil
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
