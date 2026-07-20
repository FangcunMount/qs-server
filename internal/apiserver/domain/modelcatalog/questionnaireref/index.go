package questionnaireref

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

// Question is the minimal questionnaire question surface needed for publish ref checks.
type Question struct {
	Code        string
	Type        string
	OptionCodes []string
}

// Index maps question codes to option codes for existence checks.
type Index struct {
	questions map[string]map[string]struct{}
	types     map[string]string
}

// NewIndex builds an index from published questionnaire questions.
func NewIndex(questions []Question) Index {
	idx := Index{
		questions: make(map[string]map[string]struct{}, len(questions)),
		types:     make(map[string]string, len(questions)),
	}
	for _, question := range questions {
		if question.Code == "" {
			continue
		}
		options := make(map[string]struct{}, len(question.OptionCodes))
		for _, option := range question.OptionCodes {
			if option != "" {
				options[option] = struct{}{}
			}
		}
		idx.questions[question.Code] = options
		idx.types[question.Code] = question.Type
	}
	return idx
}

// Len returns the number of indexed questions.
func (idx Index) Len() int {
	return len(idx.questions)
}

// QuestionType returns the stored question type when present.
func (idx Index) QuestionType(questionCode string) (string, bool) {
	value, ok := idx.types[questionCode]
	return value, ok
}

// Ref is one publish-time reference from DefinitionV2 into a questionnaire version.
// Empty OptionCode means only the question must exist.
type Ref struct {
	Field        string
	QuestionCode string
	OptionCode   string
}

// ValidateRefs checks question/option existence against the published questionnaire index.
func (idx Index) ValidateRefs(refs []Ref) []binding.DomainValidationIssue {
	issues := make([]binding.DomainValidationIssue, 0)
	for _, ref := range refs {
		field := ref.Field
		if field == "" {
			field = "questionnaire_ref"
		}
		if ref.QuestionCode == "" {
			issues = append(issues, binding.DomainValidationIssue{
				Field: field, Code: "question_mapping.question_code.required",
				Message: "question_code 不能为空", Level: binding.ValidationLevelError,
			})
			continue
		}
		options, ok := idx.questions[ref.QuestionCode]
		if !ok {
			issues = append(issues, binding.DomainValidationIssue{
				Field: field, Code: "question_mapping.question_not_found",
				Message: fmt.Sprintf("题目 %s 不存在", ref.QuestionCode), Level: binding.ValidationLevelError,
			})
			continue
		}
		if ref.OptionCode == "" {
			continue
		}
		if _, exists := options[ref.OptionCode]; !exists {
			issues = append(issues, binding.DomainValidationIssue{
				Field: field, Code: "question_mapping.option_not_found",
				Message: fmt.Sprintf("题目 %s 的选项 %s 不存在", ref.QuestionCode, ref.OptionCode),
				Level:   binding.ValidationLevelError,
			})
		}
	}
	return issues
}
