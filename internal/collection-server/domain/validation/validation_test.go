package validation

import (
	"testing"

	"github.com/yshujie/questionnaire-scale/internal/collection-server/domain/validation/rules"
	"github.com/yshujie/questionnaire-scale/internal/collection-server/domain/validation/strategies"
)

func TestValidator_Validate(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name     string
		value    interface{}
		rule     *rules.BaseRule
		expected bool // true if should pass validation
	}{
		{
			name:     "required field with value",
			value:    "test",
			rule:     rules.NewBaseRule("required", nil, "此字段为必填项"),
			expected: true,
		},
		{
			name:     "required field with empty string",
			value:    "",
			rule:     rules.NewBaseRule("required", nil, "此字段为必填项"),
			expected: false,
		},
		{
			name:     "required field with nil",
			value:    nil,
			rule:     rules.NewBaseRule("required", nil, "此字段为必填项"),
			expected: false,
		},
		{
			name:     "min_length with valid string",
			value:    "hello",
			rule:     rules.NewBaseRule("min_length", 3, "长度不能少于3个字符"),
			expected: true,
		},
		{
			name:     "min_length with short string",
			value:    "hi",
			rule:     rules.NewBaseRule("min_length", 3, "长度不能少于3个字符"),
			expected: false,
		},
		{
			name:     "max_length with valid string",
			value:    "hello",
			rule:     rules.NewBaseRule("max_length", 10, "长度不能超过10个字符"),
			expected: true,
		},
		{
			name:     "max_length with long string",
			value:    "hello world",
			rule:     rules.NewBaseRule("max_length", 5, "长度不能超过5个字符"),
			expected: false,
		},
		{
			name:     "min_value with valid number",
			value:    10,
			rule:     rules.NewBaseRule("min_value", 5, "值不能小于5"),
			expected: true,
		},
		{
			name:     "min_value with small number",
			value:    3,
			rule:     rules.NewBaseRule("min_value", 5, "值不能小于5"),
			expected: false,
		},
		{
			name:     "max_value with valid number",
			value:    5,
			rule:     rules.NewBaseRule("max_value", 10, "值不能大于10"),
			expected: true,
		},
		{
			name:     "max_value with large number",
			value:    15,
			rule:     rules.NewBaseRule("max_value", 10, "值不能大于10"),
			expected: false,
		},
		{
			name:     "email with valid email",
			value:    "test@example.com",
			rule:     rules.NewBaseRule("email", nil, "邮箱格式不正确"),
			expected: true,
		},
		{
			name:     "email with invalid email",
			value:    "invalid-email",
			rule:     rules.NewBaseRule("email", nil, "邮箱格式不正确"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.value, tt.rule)
			if tt.expected && err != nil {
				t.Errorf("期望验证通过，但得到错误: %v", err)
			}
			if !tt.expected && err == nil {
				t.Errorf("期望验证失败，但没有得到错误")
			}
		})
	}
}

func TestValidator_ValidateMultiple(t *testing.T) {
	validator := NewValidator()

	rules := []*rules.BaseRule{
		rules.NewBaseRule("required", nil, "此字段为必填项"),
		rules.NewBaseRule("min_length", 3, "长度不能少于3个字符"),
		rules.NewBaseRule("max_length", 10, "长度不能超过10个字符"),
	}

	// 测试有效值
	errors := validator.ValidateMultiple("hello", rules)
	if len(errors) > 0 {
		t.Errorf("期望验证通过，但得到错误: %v", errors)
	}

	// 测试无效值
	errors = validator.ValidateMultiple("", rules)
	if len(errors) == 0 {
		t.Errorf("期望验证失败，但没有得到错误")
	}
}

func TestValidator_ValidateStruct(t *testing.T) {
	validator := NewValidator()

	type TestStruct struct {
		Name  string  `json:"name"`
		Age   int     `json:"age"`
		Email string  `json:"email"`
		Score float64 `json:"score"`
	}

	data := TestStruct{
		Name:  "John",
		Age:   25,
		Email: "john@example.com",
		Score: 85.5,
	}

	rules := map[string][]*rules.BaseRule{
		"name": {
			rules.NewBaseRule("required", nil, "姓名不能为空"),
			rules.NewBaseRule("min_length", 2, "姓名长度不能少于2个字符"),
		},
		"age": {
			rules.NewBaseRule("min_value", 18, "年龄不能小于18岁"),
			rules.NewBaseRule("max_value", 100, "年龄不能大于100岁"),
		},
		"email": {
			rules.NewBaseRule("required", nil, "邮箱不能为空"),
			rules.NewBaseRule("email", nil, "邮箱格式不正确"),
		},
		"score": {
			rules.NewBaseRule("min_value", 0.0, "分数不能小于0"),
			rules.NewBaseRule("max_value", 100.0, "分数不能大于100"),
		},
	}

	errors := validator.ValidateStruct(data, rules)
	if len(errors) > 0 {
		t.Errorf("期望验证通过，但得到错误: %v", errors)
	}
}

func TestStrategyFactory(t *testing.T) {
	factory := strategies.NewStrategyFactory()

	// 测试获取策略
	strategy, err := factory.GetStrategy("required")
	if err != nil {
		t.Errorf("获取required策略失败: %v", err)
	}
	if strategy == nil {
		t.Error("策略不能为空")
	}

	// 测试获取不存在的策略
	_, err = factory.GetStrategy("non_existent")
	if err == nil {
		t.Error("期望获取不存在的策略时返回错误")
	}
}

func TestValidationRuleBuilder(t *testing.T) {
	// 测试构建器模式
	rule := NewRule("required").
		WithMessage("此字段为必填项").
		WithParam("custom", "value").
		Build()

	if rule.Name != "required" {
		t.Errorf("期望规则名称为required，但得到: %s", rule.Name)
	}

	if rule.Message != "此字段为必填项" {
		t.Errorf("期望错误消息为'此字段为必填项'，但得到: %s", rule.Message)
	}

	if rule.Params["custom"] != "value" {
		t.Errorf("期望参数custom为value，但得到: %v", rule.Params["custom"])
	}
}

func TestStringRules(t *testing.T) {
	// 测试字符串规则组合
	stringRules := NewStringRules().
		SetRequired(true).
		SetMinLength(3).
		SetMaxLength(10).
		SetPattern(`^[a-zA-Z]+$`).
		SetEmail(false)

	rules := stringRules.Build()
	if len(rules) != 4 { // required + min_length + max_length + pattern
		t.Errorf("期望4个规则，但得到: %d", len(rules))
	}
}

func TestNumberRules(t *testing.T) {
	// 测试数值规则组合
	numberRules := NewNumberRules().
		SetRequired(true).
		SetMinValue(1). // 改为非零值
		SetMaxValue(100)

	rules := numberRules.Build()
	if len(rules) != 3 { // required + min_value + max_value
		t.Errorf("期望3个规则，但得到: %d", len(rules))
	}
}
