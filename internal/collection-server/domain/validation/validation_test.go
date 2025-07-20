package validation

import (
	"testing"
)

func TestRequiredStrategy(t *testing.T) {
	strategy := &RequiredStrategy{}

	tests := []struct {
		name    string
		value   interface{}
		rule    *ValidationRule
		wantErr bool
	}{
		{
			name:    "空字符串",
			value:   "",
			rule:    NewValidationRule("required", nil, "不能为空"),
			wantErr: true,
		},
		{
			name:    "空格字符串",
			value:   "   ",
			rule:    NewValidationRule("required", nil, "不能为空"),
			wantErr: true,
		},
		{
			name:    "有效字符串",
			value:   "hello",
			rule:    NewValidationRule("required", nil, "不能为空"),
			wantErr: false,
		},
		{
			name:    "nil值",
			value:   nil,
			rule:    NewValidationRule("required", nil, "不能为空"),
			wantErr: true,
		},
		{
			name:    "空切片",
			value:   []string{},
			rule:    NewValidationRule("required", nil, "不能为空"),
			wantErr: true,
		},
		{
			name:    "非空切片",
			value:   []string{"item"},
			rule:    NewValidationRule("required", nil, "不能为空"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := strategy.Validate(tt.value, tt.rule)
			if (err != nil) != tt.wantErr {
				t.Errorf("RequiredStrategy.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMaxValueStrategy(t *testing.T) {
	strategy := &MaxValueStrategy{}

	tests := []struct {
		name    string
		value   interface{}
		rule    *ValidationRule
		wantErr bool
	}{
		{
			name:    "整数小于最大值",
			value:   5,
			rule:    NewValidationRule("max_value", float64(10), "不能大于10"),
			wantErr: false,
		},
		{
			name:    "整数等于最大值",
			value:   10,
			rule:    NewValidationRule("max_value", float64(10), "不能大于10"),
			wantErr: false,
		},
		{
			name:    "整数大于最大值",
			value:   15,
			rule:    NewValidationRule("max_value", float64(10), "不能大于10"),
			wantErr: true,
		},
		{
			name:    "浮点数小于最大值",
			value:   5.5,
			rule:    NewValidationRule("max_value", float64(10), "不能大于10"),
			wantErr: false,
		},
		{
			name:    "字符串数字小于最大值",
			value:   "5",
			rule:    NewValidationRule("max_value", float64(10), "不能大于10"),
			wantErr: false,
		},
		{
			name:    "字符串数字大于最大值",
			value:   "15",
			rule:    NewValidationRule("max_value", float64(10), "不能大于10"),
			wantErr: true,
		},
		{
			name:    "无效字符串",
			value:   "abc",
			rule:    NewValidationRule("max_value", float64(10), "不能大于10"),
			wantErr: true,
		},
		{
			name:    "nil值跳过验证",
			value:   nil,
			rule:    NewValidationRule("max_value", float64(10), "不能大于10"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := strategy.Validate(tt.value, tt.rule)
			if (err != nil) != tt.wantErr {
				t.Errorf("MaxValueStrategy.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMinValueStrategy(t *testing.T) {
	strategy := &MinValueStrategy{}

	tests := []struct {
		name    string
		value   interface{}
		rule    *ValidationRule
		wantErr bool
	}{
		{
			name:    "整数大于最小值",
			value:   15,
			rule:    NewValidationRule("min_value", float64(10), "不能小于10"),
			wantErr: false,
		},
		{
			name:    "整数等于最小值",
			value:   10,
			rule:    NewValidationRule("min_value", float64(10), "不能小于10"),
			wantErr: false,
		},
		{
			name:    "整数小于最小值",
			value:   5,
			rule:    NewValidationRule("min_value", float64(10), "不能小于10"),
			wantErr: true,
		},
		{
			name:    "nil值跳过验证",
			value:   nil,
			rule:    NewValidationRule("min_value", float64(10), "不能小于10"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := strategy.Validate(tt.value, tt.rule)
			if (err != nil) != tt.wantErr {
				t.Errorf("MinValueStrategy.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMaxLengthStrategy(t *testing.T) {
	strategy := &MaxLengthStrategy{}

	tests := []struct {
		name    string
		value   interface{}
		rule    *ValidationRule
		wantErr bool
	}{
		{
			name:    "字符串长度小于最大值",
			value:   "hello",
			rule:    NewValidationRule("max_length", 10, "长度不能超过10"),
			wantErr: false,
		},
		{
			name:    "字符串长度等于最大值",
			value:   "hello world",
			rule:    NewValidationRule("max_length", 11, "长度不能超过11"),
			wantErr: false,
		},
		{
			name:    "字符串长度大于最大值",
			value:   "hello world",
			rule:    NewValidationRule("max_length", 5, "长度不能超过5"),
			wantErr: true,
		},
		{
			name:    "切片长度小于最大值",
			value:   []string{"a", "b"},
			rule:    NewValidationRule("max_length", 5, "长度不能超过5"),
			wantErr: false,
		},
		{
			name:    "nil值跳过验证",
			value:   nil,
			rule:    NewValidationRule("max_length", 5, "长度不能超过5"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := strategy.Validate(tt.value, tt.rule)
			if (err != nil) != tt.wantErr {
				t.Errorf("MaxLengthStrategy.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMinLengthStrategy(t *testing.T) {
	strategy := &MinLengthStrategy{}

	tests := []struct {
		name    string
		value   interface{}
		rule    *ValidationRule
		wantErr bool
	}{
		{
			name:    "字符串长度大于最小值",
			value:   "hello world",
			rule:    NewValidationRule("min_length", 5, "长度不能少于5"),
			wantErr: false,
		},
		{
			name:    "字符串长度等于最小值",
			value:   "hello",
			rule:    NewValidationRule("min_length", 5, "长度不能少于5"),
			wantErr: false,
		},
		{
			name:    "字符串长度小于最小值",
			value:   "hi",
			rule:    NewValidationRule("min_length", 5, "长度不能少于5"),
			wantErr: true,
		},
		{
			name:    "nil值跳过验证",
			value:   nil,
			rule:    NewValidationRule("min_length", 5, "长度不能少于5"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := strategy.Validate(tt.value, tt.rule)
			if (err != nil) != tt.wantErr {
				t.Errorf("MinLengthStrategy.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidator(t *testing.T) {
	validator := NewValidator()

	// 测试单个值验证
	err := validator.Validate("hello", Required("不能为空"))
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	err = validator.Validate("", Required("不能为空"))
	if err == nil {
		t.Error("Expected error, got nil")
	}

	// 测试多个规则验证
	rules := []*ValidationRule{
		Required("不能为空"),
		MinLength(3, "长度不能少于3"),
		MaxLength(10, "长度不能超过10"),
	}

	errors := validator.ValidateMultiple("hi", rules)
	if len(errors) == 0 {
		t.Error("Expected validation errors, got none")
	}

	errors = validator.ValidateMultiple("hello", rules)
	if len(errors) > 0 {
		t.Errorf("Expected no errors, got %v", errors)
	}
}

func TestValidatorStruct(t *testing.T) {
	validator := NewValidator()

	type TestStruct struct {
		Name  string `json:"name"`
		Email string `json:"email"`
		Age   int    `json:"age"`
	}

	rules := map[string][]*ValidationRule{
		"name": {
			Required("姓名不能为空"),
			MinLength(2, "姓名长度不能少于2"),
		},
		"email": {
			Required("邮箱不能为空"),
			Email("邮箱格式不正确"),
		},
		"age": {
			MinValue(0, "年龄不能为负数"),
			MaxValue(150, "年龄不能超过150"),
		},
	}

	// 测试有效数据
	validData := &TestStruct{
		Name:  "张三",
		Email: "zhangsan@example.com",
		Age:   25,
	}

	errors := validator.ValidateStruct(validData, rules)
	if validator.HasErrors(errors) {
		t.Errorf("Expected no errors, got %v", errors)
	}

	// 测试无效数据
	invalidData := &TestStruct{
		Name:  "",
		Email: "invalid-email",
		Age:   -5,
	}

	errors = validator.ValidateStruct(invalidData, rules)
	if !validator.HasErrors(errors) {
		t.Error("Expected errors, got none")
	}

	// 检查错误数量
	allErrors := validator.GetAllErrors(errors)
	if len(allErrors) < 3 {
		t.Errorf("Expected at least 3 errors, got %d", len(allErrors))
	}
}

func TestValidationRuleBuilder(t *testing.T) {
	// 测试构建器模式
	rule := NewRule("max_length").
		WithValue(10).
		WithMessage("长度不能超过10").
		WithParam("trim", true).
		Build()

	if rule.Strategy != "max_length" {
		t.Errorf("Expected strategy 'max_length', got %s", rule.Strategy)
	}

	if rule.Value != 10 {
		t.Errorf("Expected value 10, got %v", rule.Value)
	}

	if rule.Message != "长度不能超过10" {
		t.Errorf("Expected message '长度不能超过10', got %s", rule.Message)
	}

	if rule.Params["trim"] != true {
		t.Errorf("Expected param 'trim' to be true, got %v", rule.Params["trim"])
	}
}

func TestStringRulesBuilder(t *testing.T) {
	validator := NewValidator()

	// 使用 StringRules 构建器
	stringRules := NewStringRules().
		SetRequired(true).
		SetMinLength(3).
		SetMaxLength(10).
		SetPattern(`^[a-zA-Z]+$`)

	rules := stringRules.Build()

	// 测试有效字符串
	err := validator.ValidateMultiple("hello", rules)
	if len(err) > 0 {
		t.Errorf("Expected no errors for 'hello', got %v", err)
	}

	// 测试无效字符串
	err = validator.ValidateMultiple("hi", rules)
	if len(err) == 0 {
		t.Error("Expected errors for 'hi', got none")
	}

	err = validator.ValidateMultiple("hello123", rules)
	if len(err) == 0 {
		t.Error("Expected errors for 'hello123', got none")
	}
}

func TestNumberRulesBuilder(t *testing.T) {
	validator := NewValidator()

	// 使用 NumberRules 构建器
	numberRules := NewNumberRules().
		SetRequired(true).
		SetRange(18, 65)

	rules := numberRules.Build()

	// 测试有效数值
	err := validator.ValidateMultiple(25, rules)
	if len(err) > 0 {
		t.Errorf("Expected no errors for 25, got %v", err)
	}

	// 测试无效数值
	err = validator.ValidateMultiple(15, rules)
	if len(err) == 0 {
		t.Error("Expected errors for 15, got none")
	}

	err = validator.ValidateMultiple(70, rules)
	if len(err) == 0 {
		t.Error("Expected errors for 70, got none")
	}
}
