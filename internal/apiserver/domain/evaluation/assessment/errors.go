package assessment

import (
	"errors"
	"fmt"
)

// ==================== 领域错误定义 ====================
// 领域层只定义错误变量和错误工厂方法
// API 错误码映射统一在 application 层处理。

// 预定义领域错误变量
var (
	// ErrInvalidStatus 无效状态错误
	ErrInvalidStatus = errors.New("invalid assessment status for this operation")

	// ErrInvalidArgument 无效参数错误
	ErrInvalidArgument = errors.New("invalid argument")

	// ErrNoEvaluationModel 未绑定评估模型。
	ErrNoEvaluationModel = errors.New("assessment has no evaluation model bound")

	// ErrEvaluationModelMismatch 评分结果与测评的评估模型不匹配。
	ErrEvaluationModelMismatch = errors.New("evaluation result model does not match assessment model")

	// ErrNotFound 未找到错误
	ErrNotFound = errors.New("assessment not found")

	// ErrEvaluationModelNotPublished 评估模型未发布或不存在。
	ErrEvaluationModelNotPublished = errors.New("evaluation model is not published")

	// ErrEvaluationModelQuestionnaireMismatch 评估模型与问卷不匹配。
	ErrEvaluationModelQuestionnaireMismatch = errors.New("evaluation model is not linked to questionnaire")
)

// ==================== 错误工厂方法 ====================

// NewInvalidStatusError 创建无效状态错误
func NewInvalidStatusError(operation string, currentStatus Status) error {
	return fmt.Errorf("%w: cannot %s in status %s", ErrInvalidStatus, operation, currentStatus)
}

// ==================== 错误判断方法 ====================

// IsNotFoundError 判断是否为未找到错误
func IsNotFoundError(err error) bool {
	return errors.Is(err, ErrNotFound)
}
