# 11-04 Survey 子域设计（V2）

> **版本**：V2.0  
> **范围**：问卷&量表 BC 中的 survey 子域  
> **目标**：阐述问卷子域的核心职责、可扩展题型设计（注册器+构造者+工厂）、可扩展答案设计（注册器+工厂）、策略模式校验规则

---

## 1. Survey 子域的定位与职责

### 1.1 子域边界

**survey 子域关注的核心问题**：

* "怎么问"：问卷结构、题目类型、选项设计
* "怎么填"：答卷收集、答案存储
* "是否合法"：输入侧校验规则

**survey 子域不关心的问题**：

* "分数代表什么含义"：这是 scale 子域的职责
* "如何计分和解读"：这是 scale 子域的职责
* "这次测评行为"：这是 assessment 子域的职责

### 1.2 核心聚合

survey 子域包含两个核心聚合：

1. **Questionnaire 聚合**：问卷模板
   * 管理题目列表（Question）
   * 每个 Question 包含选项（Option）和校验规则（ValidationRule）
   * 支持版本管理

2. **AnswerSheet 聚合**：答卷实例
   * 记录问卷 ID、答题项列表（Answer）
   * 管理答卷状态（草稿/已提交）
   * 配合校验服务完成结构校验

### 1.3 与其他子域的关系

* **不依赖** scale 子域：survey 纯粹是"收集和校验"
* **被依赖** 于 scale 子域：scale 需要读取 Question 和 AnswerSheet 的视图
* **被依赖** 于 assessment 子域：assessment 引用 QuestionnaireID 和 AnswerSheetID

**依赖方向**：

```text
survey (独立)
   ↓
 scale (依赖 survey 的只读视图)
   ↓
assessment (依赖 survey + scale)
```

---

## 2. 可扩展的题型设计（注册器+构造者+工厂）

### 2.1 设计目标

* **封闭性**：题型列表相对固定，不会频繁新增
* **扩展性**：新增题型时无需修改核心代码
* **统一性**：所有题型实现统一接口
* **灵活性**：通过 Builder 模式配置题目

### 2.2 核心组件

#### 2.2.1 Question 接口

```go
// Question 问题接口 - 统一所有题型的方法签名
type Question interface {
    // 基础方法
    GetCode() meta.Code
    GetTitle() string
    GetType() QuestionType
    GetTips() string

    // 文本相关方法
    GetPlaceholder() string
    
    // 选项相关方法
    GetOptions() []Option
    
    // 校验相关方法
    GetValidationRules() []validation.ValidationRule
    
    // 计算相关方法
    GetCalculationRule() *calculation.CalculationRule
}
```

#### 2.2.2 QuestionType 枚举

```go
// QuestionType 题型
type QuestionType string

func (t QuestionType) Value() string {
    return string(t)
}

const (
    QuestionTypeSection  QuestionType = "Section"  // 段落
    QuestionTypeRadio    QuestionType = "Radio"    // 单选
    QuestionTypeCheckbox QuestionType = "Checkbox" // 多选
    QuestionTypeText     QuestionType = "Text"     // 文本
    QuestionTypeTextarea QuestionType = "Textarea" // 文本域
    QuestionTypeNumber   QuestionType = "Number"   // 数字
)
```

### 2.3 注册器设计

```go
// QuestionFactory 注册函数签名
type QuestionFactory func(builder *QuestionBuilder) Question

// registry 注册表本体
var registry = make(map[QuestionType]QuestionFactory)

// RegisterQuestionFactory 注册函数
func RegisterQuestionFactory(typ QuestionType, factory QuestionFactory) {
    if _, exists := registry[typ]; exists {
        log.Errorf("question type already registered: %s", typ)
    }
    registry[typ] = factory
}

// CreateQuestionFromBuilder 创建统一入口
func CreateQuestionFromBuilder(builder *QuestionBuilder) Question {
    factory, ok := registry[builder.GetQuestionType()]
    if !ok {
        log.Errorf("unknown question type: %s", builder.GetQuestionType())
        return nil
    }
    return factory(builder)
}
```

### 2.4 构造者（Builder）模式

```go
// QuestionBuilder 问题构建器 - 纯配置容器
type QuestionBuilder struct {
    // 基础信息
    code         meta.Code
    title        string
    tips         string
    questionType QuestionType

    // 特定属性
    placeholder string
    options     []Option

    // 能力配置
    validationRules []validation.ValidationRule
    calculationRule *calculation.CalculationRule
}

// NewQuestionBuilder 创建新的问题构建器
func NewQuestionBuilder() *QuestionBuilder {
    return &QuestionBuilder{
        options:         make([]Option, 0),
        validationRules: make([]validation.ValidationRule, 0),
    }
}
```

#### 2.4.1 函数式选项模式

```go
// BuilderOption 构建器选项函数类型
type BuilderOption func(*QuestionBuilder)

// WithCode 设置问题编码
func WithCode(code meta.Code) BuilderOption {
    return func(b *QuestionBuilder) {
        b.code = code
    }
}

// WithTitle 设置问题标题
func WithTitle(title string) BuilderOption {
    return func(b *QuestionBuilder) {
        b.title = title
    }
}

// WithQuestionType 设置问题类型
func WithQuestionType(questionType QuestionType) BuilderOption {
    return func(b *QuestionBuilder) {
        b.questionType = questionType
    }
}

// WithOptions 设置选项列表
func WithOptions(options []Option) BuilderOption {
    return func(b *QuestionBuilder) {
        b.options = options
    }
}

// WithValidationRule 添加单个校验规则
func WithValidationRule(ruleType validation.RuleType, targetValue string) BuilderOption {
    return func(b *QuestionBuilder) {
        rule := validation.NewValidationRule(ruleType, targetValue)
        b.validationRules = append(b.validationRules, rule)
    }
}
```

#### 2.4.2 链式调用 API

```go
// SetCode 设置编码（链式调用）
func (b *QuestionBuilder) SetCode(code meta.Code) *QuestionBuilder {
    b.code = code
    return b
}

// SetTitle 设置标题（链式调用）
func (b *QuestionBuilder) SetTitle(title string) *QuestionBuilder {
    b.title = title
    return b
}

// SetQuestionType 设置题型（链式调用）
func (b *QuestionBuilder) SetQuestionType(questionType QuestionType) *QuestionBuilder {
    b.questionType = questionType
    return b
}

// AddOption 添加选项（链式调用）
func (b *QuestionBuilder) AddOption(code, content string, score int) *QuestionBuilder {
    opt := NewOptionWithStringCode(code, content, score)
    b.options = append(b.options, opt)
    return b
}

// Build 构建问题实例
func (b *QuestionBuilder) Build() Question {
    if !b.IsValid() {
        log.Errorf("invalid question builder state")
        return nil
    }
    return CreateQuestionFromBuilder(b)
}
```

### 2.5 具体题型实现

#### 2.5.1 BaseQuestion 基类

```go
// BaseQuestion 基础问题 - 提供通用实现
type BaseQuestion struct {
    code         meta.Code
    title        string
    tips         string
    placeholder  string
    questionType QuestionType
}

func NewBaseQuestion(code meta.Code, title string, questionType QuestionType) BaseQuestion {
    return BaseQuestion{
        code:         code,
        title:        title,
        questionType: questionType,
    }
}

func (q *BaseQuestion) GetCode() meta.Code           { return q.code }
func (q *BaseQuestion) GetTitle() string             { return q.title }
func (q *BaseQuestion) GetType() QuestionType        { return q.questionType }
func (q *BaseQuestion) GetTips() string              { return q.tips }
func (q *BaseQuestion) GetPlaceholder() string       { return q.placeholder }
func (q *BaseQuestion) GetOptions() []Option         { return nil }
func (q *BaseQuestion) GetValidationRules() []validation.ValidationRule { return nil }
func (q *BaseQuestion) GetCalculationRule() *calculation.CalculationRule { return nil }
```

#### 2.5.2 RadioQuestion 单选题

```go
// RadioQuestion 单选问题
type RadioQuestion struct {
    BaseQuestion
    ability.ValidationAbility
    ability.CalculationAbility

    options []question.Option
}

// 注册单选问题
func init() {
    question.RegisterQuestionFactory(question.QuestionTypeRadio, func(builder *question.QuestionBuilder) question.Question {
        // 创建单选问题
        q := newRadioQuestion(builder.GetCode(), builder.GetTitle())

        // 设置选项
        q.setOptions(builder.GetOptions())

        // 设置校验规则
        for _, rule := range builder.GetValidationRules() {
            q.addValidationRule(rule)
        }

        // 设置计算规则
        if builder.GetCalculationRule() != nil {
            q.setCalculationRule(builder.GetCalculationRule())
        }
        return q
    })
}

// newRadioQuestion 创建单选问题
func newRadioQuestion(code meta.Code, title string) *RadioQuestion {
    return &RadioQuestion{
        BaseQuestion: NewBaseQuestion(code, title, question.QuestionTypeRadio),
    }
}

// setOptions 设置选项
func (q *RadioQuestion) setOptions(options []question.Option) {
    q.options = options
}

// GetOptions 获取选项
func (q *RadioQuestion) GetOptions() []question.Option {
    return q.options
}

// GetValidationRules 获取校验规则 - 重写BaseQuestion的默认实现
func (q *RadioQuestion) GetValidationRules() []validation.ValidationRule {
    return q.ValidationAbility.GetValidationRules()
}

// GetCalculationRule 获取计算规则 - 重写BaseQuestion的默认实现
func (q *RadioQuestion) GetCalculationRule() *calculation.CalculationRule {
    return q.CalculationAbility.GetCalculationRule()
}
```

### 2.6 使用示例

#### 2.6.1 函数式选项模式创建

```go
// 创建单选题
question := question.NewQuestionBuilder().
    Apply(
        question.WithCode(meta.NewCode("Q1")),
        question.WithTitle("您的性别是？"),
        question.WithQuestionType(question.QuestionTypeRadio),
        question.WithOption("A", "男", 0),
        question.WithOption("B", "女", 0),
        question.WithValidationRule(validation.RuleTypeRequired, ""),
    ).
    Build()
```

#### 2.6.2 链式调用创建

```go
// 创建数字题
question := question.NewQuestionBuilder().
    SetCode(meta.NewCode("Q2")).
    SetTitle("您的年龄是？").
    SetQuestionType(question.QuestionTypeNumber).
    SetPlaceholder("请输入年龄").
    AddValidationRule(validation.RuleTypeRequired, "").
    AddValidationRule(validation.RuleTypeMinValue, "0").
    AddValidationRule(validation.RuleTypeMaxValue, "150").
    Build()
```

### 2.7 扩展性保证

当需要新增题型（如日期选择器）时：

1. **定义新的 QuestionType 常量**

```go
const (
    // ... 现有题型
    QuestionTypeDate QuestionType = "Date" // 新增日期题型
)
```

2. **实现新的 Question 类型**

```go
type DateQuestion struct {
    BaseQuestion
    ability.ValidationAbility
    
    minDate time.Time
    maxDate time.Time
}

func (q *DateQuestion) GetMinDate() time.Time { return q.minDate }
func (q *DateQuestion) GetMaxDate() time.Time { return q.maxDate }
```

3. **注册到工厂**

```go
func init() {
    question.RegisterQuestionFactory(question.QuestionTypeDate, func(builder *question.QuestionBuilder) question.Question {
        q := newDateQuestion(builder.GetCode(), builder.GetTitle())
        // 设置校验规则等
        return q
    })
}
```

4. **核心代码无需改动**，Builder 和 Factory 自动支持新题型

---

## 3. 可扩展的答案设计（注册器+工厂）

### 3.1 设计目标

* **类型安全**：每种答案类型有独立的结构
* **扩展性**：新增答案类型无需修改核心代码
* **统一接口**：所有答案类型实现 AnswerValue 接口
* **自动映射**：通过工厂自动创建对应类型的答案实例

### 3.2 核心组件

#### 3.2.1 AnswerValue 接口

```go
// AnswerValue 答案值接口
type AnswerValue interface {
    // Raw 原始值
    Raw() any
}
```

#### 3.2.2 AnswerValueType 枚举

```go
// AnswerValueType 值类型
type AnswerValueType string

func (t AnswerValueType) Value() string {
    return string(t)
}

const (
    StringValueType  AnswerValueType = "String"  // 字符串
    NumberValueType  AnswerValueType = "Number"  // 数字
    OptionValueType  AnswerValueType = "Option"  // 单选项
    OptionsValueType AnswerValueType = "Options" // 多选项
)
```

### 3.3 注册器设计

```go
// AnswerValueFactory 注册函数签名
type AnswerValueFactory func(v any) AnswerValue

// registry 注册表本体
var registry = make(map[AnswerValueType]AnswerValueFactory)

// RegisterAnswerValueFactory 注册函数
func RegisterAnswerValueFactory(typ AnswerValueType, factory AnswerValueFactory) {
    if _, exists := registry[typ]; exists {
        log.Errorf("answer value type already registered: %s", typ)
    }
    registry[typ] = factory
}

// CreateAnswerValuer 创建统一入口
func CreateAnswerValuer(t AnswerValueType, v any) AnswerValue {
    factory, ok := registry[t]
    if !ok {
        log.Errorf("unknown answer value type: %s", t.Value())
        return nil
    }
    return factory(v)
}
```

### 3.4 具体答案类型实现

#### 3.4.1 OptionValue 单选答案

```go
package values

import (
    "github.com/FangcunMount/qs-server/internal/apiserver/domain/answersheet/answer"
)

// 注册选项值工厂
func init() {
    answer.RegisterAnswerValueFactory(answer.OptionValueType, func(value any) answer.AnswerValue {
        if str, ok := value.(string); ok {
            return OptionValue{Code: str}
        }
        return nil
    })
}

// OptionValue 选项值
type OptionValue struct {
    Code string
}

// Raw 原始值
func (v OptionValue) Raw() any { return v.Code }
```

#### 3.4.2 OptionsValue 多选答案

```go
// 注册多选项值工厂
func init() {
    answer.RegisterAnswerValueFactory(answer.OptionsValueType, func(value any) answer.AnswerValue {
        if codes, ok := value.([]string); ok {
            return OptionsValue{Codes: codes}
        }
        return nil
    })
}

// OptionsValue 多选项值
type OptionsValue struct {
    Codes []string
}

// Raw 原始值
func (v OptionsValue) Raw() any { return v.Codes }
```

#### 3.4.3 StringValue 字符串答案

```go
// 注册字符串值工厂
func init() {
    answer.RegisterAnswerValueFactory(answer.StringValueType, func(value any) answer.AnswerValue {
        if str, ok := value.(string); ok {
            return StringValue{Text: str}
        }
        return nil
    })
}

// StringValue 字符串值
type StringValue struct {
    Text string
}

// Raw 原始值
func (v StringValue) Raw() any { return v.Text }
```

#### 3.4.4 NumberValue 数字答案

```go
// 注册数字值工厂
func init() {
    answer.RegisterAnswerValueFactory(answer.NumberValueType, func(value any) answer.AnswerValue {
        if num, ok := value.(float64); ok {
            return NumberValue{Value: num}
        }
        return nil
    })
}

// NumberValue 数字值
type NumberValue struct {
    Value float64
}

// Raw 原始值
func (v NumberValue) Raw() any { return v.Value }
```

### 3.5 Answer 聚合

```go
// Answer 答案聚合
type Answer struct {
    questionCode meta.Code
    questionType question.QuestionType
    score        float64
    value        AnswerValue
}

// NewAnswer 创建答案
func NewAnswer(qCode meta.Code, qType question.QuestionType, score float64, v any) (Answer, error) {
    // 根据题型确定答案值类型
    valueType := mapQuestionTypeToValueType(qType)
    
    // 通过工厂创建答案值
    answerValue := CreateAnswerValuer(valueType, v)
    if answerValue == nil {
        return Answer{}, fmt.Errorf("failed to create answer value")
    }
    
    return Answer{
        questionCode: qCode,
        questionType: qType,
        score:        score,
        value:        answerValue,
    }, nil
}

// GetQuestionCode 获取题目编码
func (a Answer) GetQuestionCode() meta.Code { return a.questionCode }

// GetQuestionType 获取题目类型
func (a Answer) GetQuestionType() question.QuestionType { return a.questionType }

// GetScore 获取分数
func (a Answer) GetScore() float64 { return a.score }

// GetValue 获取答案值
func (a Answer) GetValue() AnswerValue { return a.value }

// mapQuestionTypeToValueType 题型到答案类型的映射
func mapQuestionTypeToValueType(qType question.QuestionType) AnswerValueType {
    switch qType {
    case question.QuestionTypeRadio:
        return OptionValueType
    case question.QuestionTypeCheckbox:
        return OptionsValueType
    case question.QuestionTypeText, question.QuestionTypeTextarea:
        return StringValueType
    case question.QuestionTypeNumber:
        return NumberValueType
    default:
        return StringValueType
    }
}
```

### 3.6 使用示例

```go
// 创建单选答案
answer1, _ := answer.NewAnswer(
    meta.NewCode("Q1"),
    question.QuestionTypeRadio,
    1.0,
    "A", // 原始值
)

// 创建多选答案
answer2, _ := answer.NewAnswer(
    meta.NewCode("Q2"),
    question.QuestionTypeCheckbox,
    2.0,
    []string{"A", "C"}, // 原始值
)

// 创建文本答案
answer3, _ := answer.NewAnswer(
    meta.NewCode("Q3"),
    question.QuestionTypeText,
    0.0,
    "这是我的回答", // 原始值
)

// 获取答案值
value := answer1.GetValue()
if optionValue, ok := value.(OptionValue); ok {
    fmt.Println(optionValue.Code) // 输出: A
}
```

### 3.7 扩展性保证

当需要新增答案类型（如日期类型）时：

1. **定义新的 AnswerValueType 常量**

```go
const (
    // ... 现有类型
    DateValueType AnswerValueType = "Date" // 新增日期类型
)
```

2. **实现新的 AnswerValue 类型**

```go
type DateValue struct {
    Date time.Time
}

func (v DateValue) Raw() any { return v.Date }
```

3. **注册到工厂**

```go
func init() {
    answer.RegisterAnswerValueFactory(answer.DateValueType, func(value any) answer.AnswerValue {
        if dateStr, ok := value.(string); ok {
            date, _ := time.Parse("2006-01-02", dateStr)
            return DateValue{Date: date}
        }
        return nil
    })
}
```

4. **更新题型映射**

```go
func mapQuestionTypeToValueType(qType question.QuestionType) AnswerValueType {
    switch qType {
    // ... 现有映射
    case question.QuestionTypeDate:
        return DateValueType
    default:
        return StringValueType
    }
}
```

---

## 4. 策略模式实现的校验规则

### 4.1 校验规则概述

问卷校验涵盖：

* 必填项校验
* 文本长度校验
* 数值范围校验
* 选项个数校验

特点：

* 每题可配置 0~N 个规则
* 规则本身无状态，仅依赖题目定义与答案内容
* 配置驱动、可扩展

### 4.2 ValidationRule 值对象

```go
package validation

type RuleType string

const (
    RuleTypeRequired      RuleType = "required"       // 必填
    RuleTypeMinLength     RuleType = "min_length"     // 最小长度
    RuleTypeMaxLength     RuleType = "max_length"     // 最大长度
    RuleTypeMinValue      RuleType = "min_value"      // 最小值
    RuleTypeMaxValue      RuleType = "max_value"      // 最大值
    RuleTypeMinSelections RuleType = "min_selections" // 最少选择
    RuleTypeMaxSelections RuleType = "max_selections" // 最多选择
)

// ValidationRule 校验规则值对象
type ValidationRule struct {
    ruleType    RuleType
    targetValue string
}

// NewValidationRule 创建校验规则
func NewValidationRule(ruleType RuleType, targetValue string) ValidationRule {
    return ValidationRule{
        ruleType:    ruleType,
        targetValue: targetValue,
    }
}

// GetRuleType 获取规则类型
func (r *ValidationRule) GetRuleType() RuleType {
    return r.ruleType
}

// GetTargetValue 获取目标值
func (r *ValidationRule) GetTargetValue() string {
    return r.targetValue
}
```

### 4.3 Question 与 ValidationRule 的关系

```go
// Question 接口包含校验规则
type Question interface {
    // ... 其他方法
    GetValidationRules() []validation.ValidationRule
}

// ValidationAbility 校验能力（组合模式）
type ValidationAbility struct {
    validationRules []validation.ValidationRule
}

func (a *ValidationAbility) AddValidationRule(rule validation.ValidationRule) {
    a.validationRules = append(a.validationRules, rule)
}

func (a *ValidationAbility) GetValidationRules() []validation.ValidationRule {
    return a.validationRules
}
```

### 4.4 校验规则的存储

校验规则作为 Question 的一部分存储在 MongoDB 的 Questionnaire 文档中：

```json
{
  "_id": "questionnaire-001",
  "code": "PHQ-9",
  "title": "PHQ-9 抑郁症筛查量表",
  "questions": [
    {
      "code": "Q1",
      "type": "Radio",
      "title": "做事时提不起劲或没有兴趣",
      "options": [
        {"code": "0", "content": "完全不会", "score": 0},
        {"code": "1", "content": "几天", "score": 1},
        {"code": "2", "content": "一半以上的天数", "score": 2},
        {"code": "3", "content": "几乎每天", "score": 3}
      ],
      "validationRules": [
        {
          "ruleType": "required",
          "targetValue": ""
        }
      ]
    },
    {
      "code": "Q10",
      "type": "Number",
      "title": "您的年龄",
      "validationRules": [
        {
          "ruleType": "required",
          "targetValue": ""
        },
        {
          "ruleType": "min_value",
          "targetValue": "0"
        },
        {
          "ruleType": "max_value",
          "targetValue": "150"
        }
      ]
    }
  ]
}
```

### 4.5 校验执行

校验规则的具体执行逻辑由 **应用服务层** 或 **领域服务** 实现，根据 `ValidationRule.ruleType` 选择对应的校验策略。

```go
// AnswerSheetValidator 答卷校验服务（应用服务层）
type AnswerSheetValidator struct {
    questionnaireRepo QuestionnaireRepository
}

// Validate 校验答卷
func (v *AnswerSheetValidator) Validate(sheet *AnswerSheet) []ValidationError {
    var errors []ValidationError
    
    // 加载问卷定义
    questionnaire, _ := v.questionnaireRepo.FindByID(sheet.QuestionnaireID)
    
    for _, question := range questionnaire.GetQuestions() {
        // 查找该题的答案
        answer := sheet.FindAnswer(question.GetCode())
        
        // 执行该题的所有规则
        for _, rule := range question.GetValidationRules() {
            if err := v.validateRule(question, answer, rule); err != nil {
                errors = append(errors, *err)
            }
        }
    }
    
    return errors
}

// validateRule 根据规则类型执行校验（策略选择）
func (v *AnswerSheetValidator) validateRule(
    question question.Question, 
    answer *answer.Answer, 
    rule validation.ValidationRule,
) *ValidationError {
    switch rule.GetRuleType() {
    case validation.RuleTypeRequired:
        return v.validateRequired(question, answer, rule)
    case validation.RuleTypeMinValue:
        return v.validateMinValue(question, answer, rule)
    case validation.RuleTypeMaxValue:
        return v.validateMaxValue(question, answer, rule)
    case validation.RuleTypeMinSelections:
        return v.validateMinSelections(question, answer, rule)
    case validation.RuleTypeMaxSelections:
        return v.validateMaxSelections(question, answer, rule)
    default:
        return nil
    }
}

// validateRequired 必填校验
func (v *AnswerSheetValidator) validateRequired(
    question question.Question, 
    answer *answer.Answer, 
    rule validation.ValidationRule,
) *ValidationError {
    if answer == nil || answer.GetValue() == nil {
        return &ValidationError{
            QuestionCode: question.GetCode(),
            RuleType:     rule.GetRuleType(),
            Message:      fmt.Sprintf("题目 %s 为必填项", question.GetCode()),
        }
    }
    return nil
}

// validateMinValue 最小值校验
func (v *AnswerSheetValidator) validateMinValue(
    question question.Question, 
    answer *answer.Answer, 
    rule validation.ValidationRule,
) *ValidationError {
    if answer == nil {
        return nil // 空值不校验（交给 Required 规则）
    }
    
    minValue, _ := strconv.ParseFloat(rule.GetTargetValue(), 64)
    
    if numValue, ok := answer.GetValue().(NumberValue); ok {
        if numValue.Value < minValue {
            return &ValidationError{
                QuestionCode: question.GetCode(),
                RuleType:     rule.GetRuleType(),
                Message:      fmt.Sprintf("数值不能小于 %v", minValue),
            }
        }
    }
    return nil
}
```

### 4.6 使用示例

```go
// 在应用服务中使用校验器
func (s *AnswerSheetAppService) SubmitAnswerSheet(ctx context.Context, cmd SubmitAnswerSheetCmd) error {
    // 1. 加载答卷
    sheet, err := s.sheetRepo.FindByID(ctx, cmd.AnswerSheetID)
    if err != nil {
        return err
    }
    
    // 2. 执行校验
    validator := NewAnswerSheetValidator(s.questionnaireRepo)
    errors := validator.Validate(sheet)
    if len(errors) > 0 {
        return &ValidationErrors{Errors: errors}
    }
    
    // 3. 标记为已提交
    sheet.MarkAsSubmitted()
    
    // 4. 持久化
    return s.sheetRepo.Save(ctx, sheet)
}
```

---

## 5. 总结

### 5.1 Survey 子域的核心职责

1. **题型管理**：通过注册器+构造者+工厂模式支持可扩展的题型体系
2. **答案管理**：通过注册器+工厂模式支持可扩展的答案类型体系
3. **校验规则**：通过 ValidationRule 值对象存储规则配置，由应用层执行校验策略

### 5.2 设计模式应用

* **注册器模式**：QuestionFactory、AnswerValueFactory 注册表
* **构造者模式**：QuestionBuilder 配置题目
* **工厂模式**：CreateQuestionFromBuilder、CreateAnswerValuer 统一创建入口
* **策略模式**：ValidationRule + 应用层校验器实现不同的校验策略
* **组合模式**：ValidationAbility、CalculationAbility 能力组合

### 5.3 与其他子域的关系

* **Survey → Scale**：提供 Question、Answer 的只读视图，Scale 读取后进行计分和解读
* **Survey ← Assessment**：Assessment 引用 QuestionnaireID 和 AnswerSheetID
* **Survey 不依赖任何子域**：保持领域纯粹性

### 5.4 扩展性保证

* **新增题型**：注册新的 QuestionFactory，无需修改核心代码
* **新增答案类型**：注册新的 AnswerValueFactory，更新题型映射
* **新增校验规则**：扩展 RuleType 枚举，在校验器中添加对应的校验方法

---

## 附录：目录结构

```text
internal/apiserver/domain/
├── questionnaire/              # Questionnaire 聚合
│   ├── questionnaire.go        # 聚合根
│   └── question/               # Question 子实体
│       ├── question.go         # Question 接口 + QuestionType
│       ├── factory.go          # 注册器 + 工厂
│       ├── builder.go          # QuestionBuilder
│       ├── option.go           # Option 值对象
│       ├── ability/            # 能力组合
│       │   ├── validation-ability.go
│       │   └── calculation-ability.go
│       └── types/              # 具体题型实现
│           ├── base-question.go
│           ├── radio-question.go
│           ├── checkbox-question.go
│           ├── text-question.go
│           ├── num-question.go
│           └── section-question.go
└── answersheet/                # AnswerSheet 聚合
    ├── answersheet.go          # 聚合根
    └── answer/                 # Answer 子实体
        ├── answer.go           # Answer 聚合
        ├── answer-value.go     # AnswerValue 接口 + 注册器
        └── types/              # 具体答案类型实现
            ├── option-value.go
            ├── options-value.go
            ├── string-value.go
            └── number-value.go

internal/pkg/
└── validation/
    └── validation-rule.go      # ValidationRule 值对象
```

---

> **相关文档**：  
>
> * 《11-01-问卷&量表BC领域模型总览-v2.md》  
> * 《11-02-qs-apiserver领域层代码结构设计-v2.md》  
> * 《11-05-Scale子域设计-v2.md》（计分与解读）
