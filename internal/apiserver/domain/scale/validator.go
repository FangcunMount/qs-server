package scale

import (
	"fmt"

	"github.com/FangcunMount/component-base/pkg/errors"
	errorCode "github.com/FangcunMount/qs-server/internal/pkg/code"
)

// ValidationError 验证错误
type ValidationError struct {
	Field   string // 字段名
	Message string // 错误信息
}

// Error 实现 error 接口
func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// Validator 量表验证器
// 用于验证量表是否满足特定条件（如发布前验证）
type Validator struct{}

// ValidateForPublish 发布前验证
// 返回所有验证错误，如果返回空切片表示验证通过
func (Validator) ValidateForPublish(m *MedicalScale) []ValidationError {
	var errs []ValidationError

	// 1. 基本信息验证
	if m.GetTitle() == "" {
		errs = append(errs, ValidationError{
			Field:   "title",
			Message: "量表标题不能为空",
		})
	}

	if m.GetCode().IsEmpty() {
		errs = append(errs, ValidationError{
			Field:   "code",
			Message: "量表编码不能为空",
		})
	}

	// 2. 因子验证
	if m.FactorCount() == 0 {
		errs = append(errs, ValidationError{
			Field:   "factors",
			Message: "量表必须至少包含一个因子",
		})
	}

	// 3. 总分因子验证
	if _, ok := m.GetTotalScoreFactor(); !ok {
		errs = append(errs, ValidationError{
			Field:   "factors",
			Message: "量表必须包含一个总分因子",
		})
	}

	// 4. 每个因子的验证
	for _, factor := range m.GetFactors() {
		factorErrs := validateFactor(factor)
		errs = append(errs, factorErrs...)
	}

	// 5. 关联问卷验证（量表必须关联问卷）
	if m.GetQuestionnaireCode().IsEmpty() {
		errs = append(errs, ValidationError{
			Field:   "questionnaireCode",
			Message: "量表必须关联一个问卷",
		})
	}

	// 6. 问卷版本验证（量表必须指定问卷版本）
	if m.GetQuestionnaireVersion() == "" {
		errs = append(errs, ValidationError{
			Field:   "questionnaireVersion",
			Message: "量表必须指定关联问卷的版本",
		})
	}

	return errs
}

// validateFactor 验证单个因子
func validateFactor(f *Factor) []ValidationError {
	var errs []ValidationError
	factorCode := f.GetCode().Value()

	// 因子标题验证
	if f.GetTitle() == "" {
		errs = append(errs, ValidationError{
			Field:   fmt.Sprintf("factor[%s].title", factorCode),
			Message: "因子标题不能为空",
		})
	}

	// 因子必须包含题目（除非是总分因子，总分因子可以不直接包含题目）
	if !f.IsTotalScore() && f.QuestionCount() == 0 {
		errs = append(errs, ValidationError{
			Field:   fmt.Sprintf("factor[%s].questionCodes", factorCode),
			Message: "非总分因子必须包含至少一个题目",
		})
	}

	// 因子必须包含解读规则
	if len(f.GetInterpretRules()) == 0 {
		errs = append(errs, ValidationError{
			Field:   fmt.Sprintf("factor[%s].interpretRules", factorCode),
			Message: "因子必须包含至少一个解读规则",
		})
	}

	// 验证解读规则的有效性
	for i, rule := range f.GetInterpretRules() {
		if !rule.IsValid() {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("factor[%s].interpretRules[%d]", factorCode, i),
				Message: "解读规则无效（分数区间或风险等级不正确）",
			})
		}
	}

	return errs
}

// ToError 将验证错误列表转换为单个 error
// 如果没有错误返回 nil
// 返回的错误实现了 errors.Coder 接口，会返回 400 状态码
func ToError(errs []ValidationError) error {
	if len(errs) == 0 {
		return nil
	}

	// 将第一个验证错误包装为带错误码的错误，确保返回 400 状态码
	firstErr := errs[0]
	return errors.WithCode(errorCode.ErrInvalidArgument, "%s: %s", firstErr.Field, firstErr.Message)
}

// ToErrors 将验证错误列表转换为 error 切片
func ToErrors(errs []ValidationError) []error {
	if len(errs) == 0 {
		return nil
	}

	result := make([]error, len(errs))
	for i := range errs {
		result[i] = &errs[i]
	}
	return result
}
