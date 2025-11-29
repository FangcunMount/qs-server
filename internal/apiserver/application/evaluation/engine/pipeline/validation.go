package pipeline

import "context"

// ValidationHandler 前置校验处理器
// 职责：校验评估所需的输入数据完整性
// 位置：链首，在所有处理器之前执行
type ValidationHandler struct {
	*BaseHandler
}

// NewValidationHandler 创建前置校验处理器
func NewValidationHandler() *ValidationHandler {
	return &ValidationHandler{
		BaseHandler: NewBaseHandler("ValidationHandler"),
	}
}

// Handle 执行前置校验
func (h *ValidationHandler) Handle(ctx context.Context, evalCtx *Context) error {
	// 1. 校验 Assessment 不为空
	if evalCtx.Assessment == nil {
		evalCtx.SetError(ErrAssessmentRequired)
		return evalCtx.Error
	}

	// 2. 校验 Assessment 状态
	if !evalCtx.Assessment.Status().IsSubmitted() {
		evalCtx.SetError(ErrAssessmentNotSubmitted)
		return evalCtx.Error
	}

	// 3. 校验 MedicalScale 不为空（量表模式必须）
	if evalCtx.MedicalScale == nil {
		evalCtx.SetError(ErrMedicalScaleRequired)
		return evalCtx.Error
	}

	// 4. 校验量表有效性
	if err := h.validateMedicalScale(evalCtx); err != nil {
		evalCtx.SetError(err)
		return err
	}

	// 5. 校验答卷引用
	if err := h.validateAnswerSheetRef(evalCtx); err != nil {
		evalCtx.SetError(err)
		return err
	}

	// 校验通过，继续下一个处理器
	return h.Next(ctx, evalCtx)
}

// validateMedicalScale 校验量表有效性
func (h *ValidationHandler) validateMedicalScale(evalCtx *Context) error {
	// 检查量表是否有因子
	factors := evalCtx.MedicalScale.GetFactors()
	if len(factors) == 0 {
		return ErrMedicalScaleNoFactors
	}

	// TODO: 可以添加更多校验
	// - 检查量表状态是否为已发布
	// - 检查量表与问卷的匹配性

	return nil
}

// validateAnswerSheetRef 校验答卷引用
func (h *ValidationHandler) validateAnswerSheetRef(evalCtx *Context) error {
	// 检查答卷引用是否存在
	answerSheetRef := evalCtx.Assessment.AnswerSheetRef()
	if answerSheetRef.IsEmpty() {
		return ErrAnswerSheetRefRequired
	}

	// TODO: 后续集成 survey 域后，可以校验答卷是否存在、是否已完成

	return nil
}
