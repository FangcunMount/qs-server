package questionnaire

import (
	"fmt"
	"regexp"
)

// Validator 问卷验证器
type Validator struct{}

// NewValidator 创建问卷验证器
func NewValidator() *Validator {
	return &Validator{}
}

// ValidateCode 验证问卷代码
func (v *Validator) ValidateCode(code string) error {
	if code == "" {
		return fmt.Errorf("questionnaire code cannot be empty")
	}

	// 校验代码长度
	if len(code) < 3 || len(code) > 50 {
		return fmt.Errorf("questionnaire code length must be between 3 and 50 characters")
	}

	// 校验代码格式（只允许字母、数字、下划线、连字符）
	matched, err := regexp.MatchString(`^[a-zA-Z0-9_-]+$`, code)
	if err != nil {
		return fmt.Errorf("failed to validate questionnaire code format: %w", err)
	}
	if !matched {
		return fmt.Errorf("questionnaire code can only contain letters, numbers, underscores, and hyphens")
	}

	return nil
}

// ValidateQuestionnaire 验证问卷实体
func (v *Validator) ValidateQuestionnaire(questionnaire *Questionnaire) error {
	if questionnaire == nil {
		return fmt.Errorf("questionnaire cannot be nil")
	}

	// 验证问卷代码
	if err := v.ValidateCode(questionnaire.Code); err != nil {
		return fmt.Errorf("invalid questionnaire code: %w", err)
	}

	// 验证问卷标题
	if questionnaire.Title == "" {
		return fmt.Errorf("questionnaire title cannot be empty")
	}
	if len(questionnaire.Title) > 200 {
		return fmt.Errorf("questionnaire title cannot exceed 200 characters")
	}

	// 验证问卷状态
	if questionnaire.Status == "" {
		return fmt.Errorf("questionnaire status cannot be empty")
	}
	validStatuses := map[string]bool{
		"draft":     true,
		"published": true,
		"archived":  true,
	}
	if !validStatuses[questionnaire.Status] {
		return fmt.Errorf("invalid questionnaire status: %s", questionnaire.Status)
	}

	// 验证问题
	if len(questionnaire.Questions) == 0 {
		return fmt.Errorf("questionnaire must have at least one question")
	}

	// 验证问题代码唯一性
	questionCodes := make(map[string]bool)
	for i, question := range questionnaire.Questions {
		if question.Code == "" {
			return fmt.Errorf("question code cannot be empty at index %d", i)
		}
		if questionCodes[question.Code] {
			return fmt.Errorf("duplicate question code: %s", question.Code)
		}
		questionCodes[question.Code] = true
	}

	return nil
}
