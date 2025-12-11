package questionnaire

import (
	"fmt"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

// ValidationError 验证错误
type ValidationError struct {
	Field   string // 字段名
	Code    string // 问题编码（如果适用）
	Message string // 错误信息
}

// Error 实现 error 接口
func (e ValidationError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("[%s] %s: %s", e.Code, e.Field, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// Validator 问卷验证领域服务
// 负责问卷的业务规则验证，确保问卷符合发布和使用的要求
type Validator struct{}

// ValidateForPublish 验证问卷是否可以发布
// 返回所有验证错误列表，如果为空则表示验证通过
func (Validator) ValidateForPublish(q *Questionnaire) []ValidationError {
	var validationErrors []ValidationError

	// 1. 验证基本信息
	if q.title == "" {
		validationErrors = append(validationErrors, ValidationError{
			Field:   "title",
			Message: "问卷标题不能为空",
		})
	}

	if len(q.title) > 100 {
		validationErrors = append(validationErrors, ValidationError{
			Field:   "title",
			Message: "问卷标题长度不能超过100个字符",
		})
	}

	// 1.1 验证问卷分类
	if !q.GetType().IsValid() {
		validationErrors = append(validationErrors, ValidationError{
			Field:   "type",
			Message: "问卷分类无效",
		})
	}

	// 2. 验证版本
	if q.version.IsEmpty() {
		validationErrors = append(validationErrors, ValidationError{
			Field:   "version",
			Message: "问卷版本不能为空",
		})
	} else if err := q.version.Validate(); err != nil {
		validationErrors = append(validationErrors, ValidationError{
			Field:   "version",
			Message: fmt.Sprintf("问卷版本格式无效: %v", err),
		})
	}

	// 3. 验证问题列表
	if q.QuestionCount() == 0 {
		validationErrors = append(validationErrors, ValidationError{
			Field:   "questions",
			Message: "问卷必须包含至少一个问题",
		})
		// 没有问题就不需要继续验证了
		return validationErrors
	}

	// 4. 验证问题编码唯一性
	codeMap := make(map[string]bool)
	for i, question := range q.questions {
		// 检查问题对象是否为 nil
		if question == nil {
			validationErrors = append(validationErrors, ValidationError{
				Field:   "questions",
				Code:    fmt.Sprintf("第%d个问题", i+1),
				Message: "问题对象为nil",
			})
			continue
		}

		questionCode := question.GetCode().Value()
		if questionCode == "" {
			validationErrors = append(validationErrors, ValidationError{
				Field:   "questions",
				Code:    fmt.Sprintf("第%d个问题", i+1),
				Message: "问题编码不能为空",
			})
		} else if codeMap[questionCode] {
			validationErrors = append(validationErrors, ValidationError{
				Field:   "questions",
				Code:    questionCode,
				Message: "问题编码重复",
			})
		} else {
			codeMap[questionCode] = true
		}
	}

	// 5. 验证每个问题的有效性
	for _, question := range q.questions {
		questionErrors := validateQuestion(question)
		validationErrors = append(validationErrors, questionErrors...)
	}

	return validationErrors
}

// validateQuestion 验证单个问题的有效性
func validateQuestion(q Question) []ValidationError {
	var validationErrors []ValidationError

	// 检查问题对象是否为nil
	if q == nil {
		validationErrors = append(validationErrors, ValidationError{
			Field:   "question",
			Message: "问题对象为nil",
		})
		return validationErrors
	}

	questionCode := q.GetCode().Value()

	// 验证题干
	if q.GetStem() == "" {
		validationErrors = append(validationErrors, ValidationError{
			Field:   "stem",
			Code:    questionCode,
			Message: "问题题干不能为空",
		})
	}

	// 验证选择题的选项
	questionType := q.GetType()
	if questionType == TypeRadio || questionType == TypeCheckbox {
		options := q.GetOptions()

		if len(options) == 0 {
			validationErrors = append(validationErrors, ValidationError{
				Field:   "options",
				Code:    questionCode,
				Message: "选择题必须包含选项",
			})
		} else if len(options) < 2 {
			validationErrors = append(validationErrors, ValidationError{
				Field:   "options",
				Code:    questionCode,
				Message: fmt.Sprintf("选择题至少需要2个选项，当前只有%d个", len(options)),
			})
		}

		// 验证选项编码唯一性
		optionCodes := make(map[string]bool)
		for i, option := range options {
			optionCode := option.GetCode().Value()
			if optionCode == "" {
				validationErrors = append(validationErrors, ValidationError{
					Field:   "options",
					Code:    questionCode,
					Message: fmt.Sprintf("第%d个选项编码不能为空", i+1),
				})
			} else if optionCodes[optionCode] {
				validationErrors = append(validationErrors, ValidationError{
					Field:   "options",
					Code:    questionCode,
					Message: fmt.Sprintf("选项编码'%s'重复", optionCode),
				})
			} else {
				optionCodes[optionCode] = true
			}

			// 验证选项文本
			if option.GetContent() == "" {
				validationErrors = append(validationErrors, ValidationError{
					Field:   "options",
					Code:    questionCode,
					Message: fmt.Sprintf("选项'%s'的文本不能为空", optionCode),
				})
			}
		}
	}

	return validationErrors
}

// ValidateBasicInfo 验证基本信息
func (Validator) ValidateBasicInfo(q *Questionnaire) error {
	if q.title == "" {
		return errors.WithMessage(
			errors.WithCode(code.ErrQuestionnaireInvalidTitle, ""),
			"标题不能为空",
		)
	}

	if len(q.title) > 100 {
		return errors.WithMessage(
			errors.WithCode(code.ErrQuestionnaireInvalidTitle, ""),
			"标题长度不能超过100个字符",
		)
	}

	if len(q.desc) > 500 {
		return errors.WithMessage(
			errors.WithCode(code.ErrQuestionnaireInvalidInput, ""),
			"描述长度不能超过500个字符",
		)
	}

	return nil
}

// ValidateQuestion 验证问题是否有效
func (Validator) ValidateQuestion(q Question) error {
	if q == nil {
		return errors.WithMessage(
			errors.WithCode(code.ErrQuestionnaireInvalidQuestion, ""),
			"问题对象不能为空",
		)
	}

	if q.GetCode().Value() == "" {
		return errors.WithMessage(
			errors.WithCode(code.ErrQuestionnaireInvalidQuestion, ""),
			"问题编码不能为空",
		)
	}

	if q.GetStem() == "" {
		return errors.WithMessage(
			errors.WithCode(code.ErrQuestionnaireInvalidQuestion, ""),
			"问题题干不能为空",
		)
	}

	// 验证选择题的选项
	questionType := q.GetType()
	if questionType == TypeRadio || questionType == TypeCheckbox {
		options := q.GetOptions()
		if len(options) < 2 {
			return errors.WithMessage(
				errors.WithCode(code.ErrQuestionnaireInvalidQuestion, ""),
				"选择题至少需要2个选项",
			)
		}
	}

	return nil
}

// ValidateQuestions 批量验证问题列表
func (v Validator) ValidateQuestions(questions []Question) error {
	if len(questions) == 0 {
		return errors.WithMessage(
			errors.WithCode(code.ErrQuestionnaireInvalidQuestion, ""),
			"问题列表不能为空",
		)
	}

	// 验证编码唯一性
	codeMap := make(map[string]bool)
	for i, question := range questions {
		if question == nil {
			return fmt.Errorf("第%d个问题对象为空", i+1)
		}

		questionCode := question.GetCode().Value()
		if questionCode == "" {
			return fmt.Errorf("第%d个问题的编码不能为空", i+1)
		}

		if codeMap[questionCode] {
			return fmt.Errorf("问题编码'%s'重复", questionCode)
		}
		codeMap[questionCode] = true

		// 验证单个问题
		if err := v.ValidateQuestion(question); err != nil {
			return err
		}
	}

	return nil
}

// ToError 将验证错误列表转换为单个错误
// 如果没有错误则返回 nil
// 如果有错误则将所有错误信息合并
func ToError(validationErrors []ValidationError) error {
	if len(validationErrors) == 0 {
		return nil
	}

	if len(validationErrors) == 1 {
		return fmt.Errorf("%s", validationErrors[0].Error())
	}

	// 合并多个错误
	var messages []string
	for _, ve := range validationErrors {
		messages = append(messages, ve.Error())
	}

	return fmt.Errorf("问卷验证失败，共%d个错误:\n- %s",
		len(validationErrors), joinStrings(messages, "\n- "))
}

// joinStrings 连接字符串切片
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}
