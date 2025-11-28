package assessment

import (
	"errors"
	"fmt"
)

// ==================== 领域错误定义 ====================
// 领域层只定义错误变量和错误工厂方法
// 错误码定义和映射统一在 internal/pkg/code/assessment.go 中

// 预定义领域错误变量
var (
	// ErrInvalidStatus 无效状态错误
	ErrInvalidStatus = errors.New("invalid assessment status for this operation")

	// ErrInvalidArgument 无效参数错误
	ErrInvalidArgument = errors.New("invalid argument")

	// ErrNoScale 无量表错误
	ErrNoScale = errors.New("assessment has no medical scale bound")

	// ErrNotFound 未找到错误
	ErrNotFound = errors.New("assessment not found")

	// ErrDuplicate 重复错误
	ErrDuplicate = errors.New("assessment already exists")

	// ErrTesteeNotFound 受试者未找到
	ErrTesteeNotFound = errors.New("testee not found")

	// ErrQuestionnaireNotFound 问卷未找到
	ErrQuestionnaireNotFound = errors.New("questionnaire not found")

	// ErrQuestionnaireNotPublished 问卷未发布
	ErrQuestionnaireNotPublished = errors.New("questionnaire is not published")

	// ErrAnswerSheetNotFound 答卷未找到
	ErrAnswerSheetNotFound = errors.New("answer sheet not found")

	// ErrAnswerSheetMismatch 答卷不匹配
	ErrAnswerSheetMismatch = errors.New("answer sheet does not belong to questionnaire")

	// ErrScaleNotFound 量表未找到
	ErrScaleNotFound = errors.New("medical scale not found")

	// ErrScaleNotLinked 量表未关联
	ErrScaleNotLinked = errors.New("medical scale is not linked to questionnaire")

	// ErrReportNotFound 报告未找到
	ErrReportNotFound = errors.New("interpret report not found")

	// ErrScoreNotFound 得分未找到
	ErrScoreNotFound = errors.New("assessment score not found")
)

// ==================== 错误工厂方法 ====================

// NewInvalidStatusError 创建无效状态错误
func NewInvalidStatusError(operation string, currentStatus Status) error {
	return fmt.Errorf("%w: cannot %s in status %s", ErrInvalidStatus, operation, currentStatus)
}

// NewNotFoundError 创建未找到错误
func NewNotFoundError(entityType string, id interface{}) error {
	return fmt.Errorf("%w: %s with id %v", ErrNotFound, entityType, id)
}

// NewDuplicateError 创建重复错误
func NewDuplicateError(entityType string, field string, value interface{}) error {
	return fmt.Errorf("%w: %s with %s=%v", ErrDuplicate, entityType, field, value)
}

// ==================== 错误判断方法 ====================

// IsInvalidStatusError 判断是否为无效状态错误
func IsInvalidStatusError(err error) bool {
	return errors.Is(err, ErrInvalidStatus)
}

// IsNotFoundError 判断是否为未找到错误
func IsNotFoundError(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsDuplicateError 判断是否为重复错误
func IsDuplicateError(err error) bool {
	return errors.Is(err, ErrDuplicate)
}

// IsTesteeNotFoundError 判断是否为受试者未找到错误
func IsTesteeNotFoundError(err error) bool {
	return errors.Is(err, ErrTesteeNotFound)
}

// IsQuestionnaireNotFoundError 判断是否为问卷未找到错误
func IsQuestionnaireNotFoundError(err error) bool {
	return errors.Is(err, ErrQuestionnaireNotFound)
}

// IsScaleNotFoundError 判断是否为量表未找到错误
func IsScaleNotFoundError(err error) bool {
	return errors.Is(err, ErrScaleNotFound)
}
