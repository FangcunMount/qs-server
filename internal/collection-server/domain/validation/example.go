package validation

import (
	"fmt"
	"strings"
)

// AnswerData 答卷答案数据示例
type AnswerData struct {
	QuestionCode string      `json:"question_code"`
	AnswerType   string      `json:"answer_type"`
	Value        interface{} `json:"value"`
}

// ExampleUsage 使用示例
func ExampleUsage() {
	// 创建验证器
	validator := NewValidator()

	// 定义不同题型的验证规则
	validationRules := map[string][]*ValidationRule{
		"radio": { // 单选题
			Required("请选择一个选项"),
			OptionCode([]string{"A", "B", "C", "D"}, "选择的选项不在允许范围内"),
		},
		"text": { // 文本题
			Required("请填写答案"),
			MinLength(5, "答案长度不能少于5个字符"),
			MaxLength(500, "答案长度不能超过500个字符"),
		},
		"number": { // 数字题
			Required("请填写数字"),
			Range(0, 100, "答案必须在0到100之间"),
		},
		"email": { // 邮箱题
			Required("请填写邮箱"),
			Email("邮箱格式不正确"),
		},
		"phone": { // 手机号题
			Required("请填写手机号"),
			Phone("手机号格式不正确"),
		},
	}

	// 测试数据
	answers := []AnswerData{
		{
			QuestionCode: "Q1",
			AnswerType:   "radio",
			Value:        "A",
		},
		{
			QuestionCode: "Q2",
			AnswerType:   "text",
			Value:        "这是一个很长的答案，用来测试长度验证",
		},
		{
			QuestionCode: "Q3",
			AnswerType:   "number",
			Value:        75,
		},
		{
			QuestionCode: "Q4",
			AnswerType:   "email",
			Value:        "test@example.com",
		},
		{
			QuestionCode: "Q5",
			AnswerType:   "phone",
			Value:        "13800138000",
		},
	}

	// 验证每个答案
	fmt.Println("=== 答案验证示例 ===")
	for _, answer := range answers {
		rules, exists := validationRules[answer.AnswerType]
		if !exists {
			fmt.Printf("问题 %s: 未知的答案类型 %s\n", answer.QuestionCode, answer.AnswerType)
			continue
		}

		errors := validator.ValidateMultiple(answer.Value, rules)
		if len(errors) > 0 {
			fmt.Printf("问题 %s (%s): 验证失败 - %s\n",
				answer.QuestionCode, answer.AnswerType, errors[0].Error())
		} else {
			fmt.Printf("问题 %s (%s): 验证通过\n",
				answer.QuestionCode, answer.AnswerType)
		}
	}
}

// ExampleStringRules 文本答案规则使用示例
func ExampleStringRules() {
	validator := NewValidator()

	// 使用 StringRules 构建器
	stringRules := NewStringRules().
		SetRequired(true).
		SetMinLength(10).
		SetMaxLength(200).
		SetPattern(`^[^<>]*$`) // 不允许HTML标签

	rules := stringRules.Build()

	// 测试验证
	testAnswers := []string{
		"",   // 空答案
		"太短", // 长度不够
		"这是一个正常的答案，长度符合要求",                     // 正常答案
		"这是一个很长的答案" + strings.Repeat("很长", 50), // 超长答案
		"包含<script>标签的答案",                      // 包含HTML标签
	}

	for _, answer := range testAnswers {
		errors := validator.ValidateMultiple(answer, rules)
		if len(errors) > 0 {
			fmt.Printf("'%s' 验证失败: %v\n", answer, errors[0].Error())
		} else {
			fmt.Printf("'%s' 验证通过\n", answer)
		}
	}
}

// ExampleNumberRules 数值答案规则使用示例
func ExampleNumberRules() {
	validator := NewValidator()

	// 使用 NumberRules 构建器
	numberRules := NewNumberRules().
		SetRequired(true).
		SetRange(0, 100)

	rules := numberRules.Build()

	// 测试验证
	testAnswers := []interface{}{
		nil,   // 空答案
		-5,    // 负数
		25,    // 正常范围
		75,    // 正常范围
		150,   // 超出范围
		"abc", // 非数字
		"50",  // 字符串数字
	}

	for _, answer := range testAnswers {
		errors := validator.ValidateMultiple(answer, rules)
		if len(errors) > 0 {
			fmt.Printf("%v 验证失败: %v\n", answer, errors[0].Error())
		} else {
			fmt.Printf("%v 验证通过\n", answer)
		}
	}
}

// ExampleCustomStrategy 自定义策略示例
func ExampleCustomStrategy() {
	validator := NewValidator()

	// 注册自定义策略
	validator.RegisterStrategy(&CustomStrategy{})

	// 使用自定义策略
	rule := NewRule("custom").
		WithValue([]string{"敏感词1", "敏感词2"}).
		WithMessage("答案包含敏感词汇").
		Build()

	// 测试验证
	testAnswers := []string{"这是一个正常的答案", "这个答案包含敏感词1", "另一个正常答案", ""}

	for _, answer := range testAnswers {
		err := validator.Validate(answer, rule)
		if err != nil {
			fmt.Printf("'%s' 验证失败: %s\n", answer, err.Error())
		} else {
			fmt.Printf("'%s' 验证通过\n", answer)
		}
	}
}

// CustomStrategy 自定义验证策略示例
type CustomStrategy struct{}

func (s *CustomStrategy) Validate(value interface{}, rule *ValidationRule) error {
	if value == nil {
		return nil // 空值跳过验证
	}

	strValue, ok := value.(string)
	if !ok {
		return NewValidationError("", "答案必须是字符串类型", value)
	}

	// 自定义验证逻辑：检查是否包含特定关键词
	keywords, ok := rule.Value.([]string)
	if !ok {
		return fmt.Errorf("自定义验证规则的值必须是字符串数组")
	}

	for _, keyword := range keywords {
		if strings.Contains(strValue, keyword) {
			return NewValidationError("", rule.Message, value)
		}
	}

	return nil
}

func (s *CustomStrategy) GetStrategyName() string {
	return "custom"
}

// RunExamples 运行所有示例
func RunExamples() {
	fmt.Println("=== 基本使用示例 ===")
	ExampleUsage()

	fmt.Println("\n=== 字符串规则示例 ===")
	ExampleStringRules()

	fmt.Println("\n=== 数值规则示例 ===")
	ExampleNumberRules()

	fmt.Println("\n=== 自定义策略示例 ===")
	ExampleCustomStrategy()
}
