package validation

import "context"

// Service 验证服务统一接口
type Service interface {
	// ValidateAnswersheet 验证答卷
	ValidateAnswersheet(ctx context.Context, req *ValidationRequest) error

	// ValidateQuestionnaireCode 验证问卷代码
	ValidateQuestionnaireCode(ctx context.Context, code string) error
}

// ValidationRequest 验证请求
type ValidationRequest struct {
	QuestionnaireCode string                  `json:"questionnaire_code" validate:"required"`
	Title             string                  `json:"title" validate:"required"`
	TesteeInfo        *TesteeInfo             `json:"testee_info" validate:"required"`
	Answers           []*AnswerValidationItem `json:"answers" validate:"required"`
}

// TesteeInfo 测试者信息
type TesteeInfo struct {
	Name  string `json:"name" validate:"required"`
	Email string `json:"email,omitempty"`
	Phone string `json:"phone,omitempty"`
}

// AnswerValidationItem 答案验证项
type AnswerValidationItem struct {
	QuestionCode string      `json:"question_code" validate:"required"`
	QuestionType string      `json:"question_type" validate:"required"`
	Value        interface{} `json:"value" validate:"required"`
}

// ValidationStrategy 验证策略
type ValidationStrategy string

const (
	SequentialStrategy ValidationStrategy = "sequential"
	ConcurrentStrategy ValidationStrategy = "concurrent"
)

// ValidationConfig 验证配置
type ValidationConfig struct {
	Strategy           ValidationStrategy `json:"strategy"`
	MaxConcurrency     int                `json:"max_concurrency"`
	ValidationTimeout  int                `json:"validation_timeout"`
	EnableDetailedLogs bool               `json:"enable_detailed_logs"`
}

// DefaultValidationConfig 默认验证配置
func DefaultValidationConfig() *ValidationConfig {
	return &ValidationConfig{
		Strategy:           SequentialStrategy,
		MaxConcurrency:     10,
		ValidationTimeout:  30,
		EnableDetailedLogs: false,
	}
}
