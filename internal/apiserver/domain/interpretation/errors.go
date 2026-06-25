package interpretation

import "errors"

// ==================== 领域错误定义 ====================

var (
	// ErrNoInterpretRules 无解读规则
	ErrNoInterpretRules = errors.New("no interpret rules provided")

	// ErrNoMatchingRule 无匹配规则
	ErrNoMatchingRule = errors.New("no matching interpret rule found")

	// ErrInvalidConfig 无效配置
	ErrInvalidConfig = errors.New("invalid interpret config")

	// ErrInvalidCondition 无效条件
	ErrInvalidCondition = errors.New("invalid condition in composite rule")

	// ErrFactorNotFound 因子未找到
	ErrFactorNotFound = errors.New("factor not found in scores")
)
