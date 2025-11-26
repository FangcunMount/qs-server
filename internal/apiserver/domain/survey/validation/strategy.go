package validation

// ValidatableValue 可校验的值接口
// 这是 validation 领域对"被校验值"的抽象
type ValidatableValue interface {
	// IsEmpty 值是否为空
	IsEmpty() bool

	// AsString 获取字符串表示（用于长度、正则校验等）
	AsString() string

	// AsNumber 获取数值表示（用于范围校验等）
	AsNumber() (float64, error)

	// AsArray 获取数组表示（用于多选校验等）
	AsArray() []string
}

// ValidationStrategy 校验策略接口
// 每种 ValidationRule 类型对应一个策略实现
//
// 设计原则：validation 领域只关心"规则"和"值"，不依赖具体的业务对象
type ValidationStrategy interface {
	// Validate 执行校验
	// value: 被校验的值（抽象接口）
	// rule: 校验规则（值对象）
	Validate(value ValidatableValue, r ValidationRule) error

	// SupportRuleType 返回支持的规则类型
	SupportRuleType() RuleType
}

// strategyRegistry 策略注册表
var strategyRegistry = make(map[RuleType]ValidationStrategy)

// RegisterStrategy 注册校验策略
func RegisterStrategy(strategy ValidationStrategy) {
	strategyRegistry[strategy.SupportRuleType()] = strategy
}

// GetStrategy 获取校验策略
func GetStrategy(ruleType RuleType) ValidationStrategy {
	return strategyRegistry[ruleType]
}
