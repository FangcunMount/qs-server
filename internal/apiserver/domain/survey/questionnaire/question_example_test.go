package questionnaire_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/validation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// TestCreateRadioQuestion 创建单选题示例
func TestCreateRadioQuestion(t *testing.T) {
	// 使用 Builder 模式创建单选题
	question, err := questionnaire.NewQuestion(
		questionnaire.WithCode(meta.NewCode("Q1")),
		questionnaire.WithStem("您的性别是？"),
		questionnaire.WithTips("请选择您的性别"),
		questionnaire.WithQuestionType(questionnaire.TypeRadio),
		questionnaire.WithOption("A", "男", 0),
		questionnaire.WithOption("B", "女", 0),
		questionnaire.WithRequired(),
		questionnaire.WithCalculationRule(calculation.FormulaTypeScore),
	)

	if err != nil {
		t.Fatalf("创建单选题失败: %v", err)
	}

	// 验证题型
	if question.GetType() != questionnaire.TypeRadio {
		t.Errorf("题型不匹配，期望 %s，实际 %s", questionnaire.TypeRadio, question.GetType())
	}

	// 验证选项
	options := question.GetOptions()
	if len(options) != 2 {
		t.Errorf("选项数量不匹配，期望 2，实际 %d", len(options))
	}

	// 验证校验规则
	rules := question.GetValidationRules()
	if len(rules) == 0 {
		t.Error("应该有必填校验规则")
	}

	t.Logf("成功创建单选题: %s - %s", question.GetCode().Value(), question.GetStem())
}

// TestCreateCheckboxQuestion 创建多选题示例
func TestCreateCheckboxQuestion(t *testing.T) {
	question, err := questionnaire.NewQuestion(
		questionnaire.WithCode(meta.NewCode("Q2")),
		questionnaire.WithStem("您的兴趣爱好有哪些？"),
		questionnaire.WithQuestionType(questionnaire.TypeCheckbox),
		questionnaire.WithOption("A", "运动", 1),
		questionnaire.WithOption("B", "阅读", 1),
		questionnaire.WithOption("C", "音乐", 1),
		questionnaire.WithOption("D", "旅游", 1),
		questionnaire.WithRequired(),
		questionnaire.WithValidationRule(validation.RuleTypeMinSelections, "1"),
		questionnaire.WithValidationRule(validation.RuleTypeMaxSelections, "3"),
	)

	if err != nil {
		t.Fatalf("创建多选题失败: %v", err)
	}

	if question.GetType() != questionnaire.TypeCheckbox {
		t.Errorf("题型不匹配，期望 %s，实际 %s", questionnaire.TypeCheckbox, question.GetType())
	}

	t.Logf("成功创建多选题: %s - %s", question.GetCode().Value(), question.GetStem())
}

// TestCreateTextQuestion 创建文本题示例
func TestCreateTextQuestion(t *testing.T) {
	question, err := questionnaire.NewQuestion(
		questionnaire.WithCode(meta.NewCode("Q3")),
		questionnaire.WithStem("请输入您的姓名"),
		questionnaire.WithQuestionType(questionnaire.TypeText),
		questionnaire.WithPlaceholder("请输入真实姓名"),
		questionnaire.WithRequired(),
		questionnaire.WithMinLength(2),
		questionnaire.WithMaxLength(20),
	)

	if err != nil {
		t.Fatalf("创建文本题失败: %v", err)
	}

	if question.GetPlaceholder() != "请输入真实姓名" {
		t.Errorf("占位符不匹配")
	}

	t.Logf("成功创建文本题: %s - %s", question.GetCode().Value(), question.GetStem())
}

// TestCreateNumberQuestion 创建数字题示例
func TestCreateNumberQuestion(t *testing.T) {
	question, err := questionnaire.NewQuestion(
		questionnaire.WithCode(meta.NewCode("Q4")),
		questionnaire.WithStem("请输入您的年龄"),
		questionnaire.WithQuestionType(questionnaire.TypeNumber),
		questionnaire.WithPlaceholder("请输入年龄"),
		questionnaire.WithRequired(),
		questionnaire.WithMinValue(0),
		questionnaire.WithMaxValue(150),
	)

	if err != nil {
		t.Fatalf("创建数字题失败: %v", err)
	}

	if question.GetType() != questionnaire.TypeNumber {
		t.Errorf("题型不匹配")
	}

	t.Logf("成功创建数字题: %s - %s", question.GetCode().Value(), question.GetStem())
}

// TestCreateSectionQuestion 创建段落题示例
func TestCreateSectionQuestion(t *testing.T) {
	question, err := questionnaire.NewQuestion(
		questionnaire.WithCode(meta.NewCode("S1")),
		questionnaire.WithStem("基本信息"),
		questionnaire.WithTips("请如实填写以下信息"),
		questionnaire.WithQuestionType(questionnaire.TypeSection),
	)

	if err != nil {
		t.Fatalf("创建段落题失败: %v", err)
	}

	if question.GetType() != questionnaire.TypeSection {
		t.Errorf("题型不匹配")
	}

	// 段落题不应有选项
	if question.GetOptions() != nil {
		t.Error("段落题不应该有选项")
	}

	t.Logf("成功创建段落题: %s - %s", question.GetCode().Value(), question.GetStem())
}

// TestBuilderChainStyle 使用参数构造器 + 工厂函数创建问题
func TestBuilderChainStyle(t *testing.T) {
	// 方式1: 一次性传入所有参数
	question, err := questionnaire.NewQuestion(
		questionnaire.WithCode(meta.NewCode("Q5")),
		questionnaire.WithStem("您对我们的服务满意吗？"),
		questionnaire.WithQuestionType(questionnaire.TypeRadio),
		questionnaire.WithOption("1", "非常不满意", 1),
		questionnaire.WithOption("2", "不满意", 2),
		questionnaire.WithOption("3", "一般", 3),
		questionnaire.WithOption("4", "满意", 4),
		questionnaire.WithOption("5", "非常满意", 5),
		questionnaire.WithRequired(),
	)

	if err != nil {
		t.Fatalf("创建问题失败: %v", err)
	}

	if len(question.GetOptions()) != 5 {
		t.Errorf("选项数量不匹配，期望 5，实际 %d", len(question.GetOptions()))
	}

	t.Logf("成功使用工厂函数创建问题: %s", question.GetCode().Value())

	// 方式2: 使用参数容器分步设置（可选）
	params := questionnaire.NewQuestionParams(
		questionnaire.WithCode(meta.NewCode("Q6")),
		questionnaire.WithStem("您的满意度评分？"),
		questionnaire.WithQuestionType(questionnaire.TypeRadio),
	)

	// 后续可以追加更多参数
	params.Apply(
		questionnaire.WithOption("A", "很好", 5),
		questionnaire.WithOption("B", "好", 4),
		questionnaire.WithRequired(),
	)

	// 通过工厂函数创建
	_, _ = questionnaire.NewQuestion() // 不传参数,使用 params 内部的参数
	// 注意:这里演示的是概念,实际应该把 params 的参数传给 NewQuestion
	// 正确做法是使用方式1,或者实现一个 BuildWith(params) 函数

	t.Log("参数容器只是收集参数,实际创建由工厂函数负责")
}

// TestQuestionInterface 测试接口类型断言
func TestQuestionInterface(t *testing.T) {
	question, _ := questionnaire.NewQuestion(
		questionnaire.WithCode(meta.NewCode("Q6")),
		questionnaire.WithStem("测试问题"),
		questionnaire.WithQuestionType(questionnaire.TypeRadio),
		questionnaire.WithOption("A", "选项A", 1),
	)

	// 类型断言 - 检查是否实现了 HasOptions 接口
	if hasOpts, ok := question.(questionnaire.HasOptions); ok {
		options := hasOpts.GetOptions()
		t.Logf("该问题有 %d 个选项", len(options))
	}

	// 类型断言 - 检查是否实现了 HasValidation 接口
	if hasVal, ok := question.(questionnaire.HasValidation); ok {
		rules := hasVal.GetValidationRules()
		t.Logf("该问题有 %d 个校验规则", len(rules))
	}

	// 类型断言 - 检查是否实现了 HasCalculation 接口
	if hasCalc, ok := question.(questionnaire.HasCalculation); ok {
		rule := hasCalc.GetCalculationRule()
		if rule != nil {
			t.Logf("该问题有计算规则: %s", rule.GetFormula())
		}
	}
}
