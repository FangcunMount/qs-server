package pipeline

// 处理器相关错误定义

// HandlerError 处理器错误
type HandlerError struct {
	message string
}

func (e *HandlerError) Error() string {
	return e.message
}

// NewHandlerError 创建处理器错误
func NewHandlerError(message string) *HandlerError {
	return &HandlerError{message: message}
}

// ==================== 通用错误定义 ====================

var (
	// ErrAssessmentRequired 测评不能为空
	ErrAssessmentRequired = NewHandlerError("assessment is required")

	// ErrMedicalScaleRequired 量表不能为空
	ErrMedicalScaleRequired = NewHandlerError("medical scale is required")

	// ErrFactorScoresRequired 因子得分不能为空
	ErrFactorScoresRequired = NewHandlerError("factor scores are required")

	// ErrEvaluationResultRequired 评估结果不能为空
	ErrEvaluationResultRequired = NewHandlerError("evaluation result is required")

	// ErrAssessmentNotSubmitted 测评未提交
	ErrAssessmentNotSubmitted = NewHandlerError("assessment is not submitted")

	// ErrMedicalScaleNoFactors 量表无因子
	ErrMedicalScaleNoFactors = NewHandlerError("medical scale has no factors")

	// ErrAnswerSheetRefRequired 答卷引用不能为空
	ErrAnswerSheetRefRequired = NewHandlerError("answer sheet reference is required")
)
