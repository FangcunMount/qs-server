# 11-04-04 Validation 子域设计

> **版本**：V3.0  
> **最后更新**：2025-11-26  
> **状态**：✅ 已实现并验证  
> **所属系列**：[Survey 子域设计系列](./11-04-Survey子域设计系列.md)

---

## 1. Validation 子域概览

### 1.1 子域职责

Validation 子域是 Survey 领域中的**独立子域**，专门负责输入数据的校验：

* 🎯 **可扩展校验**：基于策略模式的校验系统
* 🔌 **松耦合**：通过接口与其他聚合解耦
* 📋 **规则定义**：ValidationRule 值对象描述校验规则
* ✅ **统一校验**：Validator 领域服务执行校验
* 📊 **结果反馈**：ValidationResult 返回校验结果

### 1.2 子域组成

```text
Validation 子域
├── 领域服务
│   └── Validator                 (校验器)
│       └── DefaultValidator      (默认实现)
│
├── 值对象
│   ├── ValidationRule            (校验规则)
│   ├── RuleType                  (规则类型枚举)
│   ├── ValidationResult          (校验结果)
│   └── ValidationError           (校验错误)
│
├── 策略接口
│   └── ValidationStrategy        (校验策略接口)
│
├── 策略实现（8种）
│   ├── RequiredStrategy          (必填校验)
│   ├── MinLengthStrategy         (最小长度)
│   ├── MaxLengthStrategy         (最大长度)
│   ├── MinValueStrategy          (最小值)
│   ├── MaxValueStrategy          (最大值)
│   ├── MinSelectionsStrategy     (最少选择)
│   ├── MaxSelectionsStrategy     (最多选择)
│   └── PatternStrategy           (正则表达式)
│
├── 适配接口
│   └── ValidatableValue          (可校验值接口)
│
└── 注册器
    └── strategyRegistry          (策略注册表)
```

### 1.3 设计特点

**与其他子域的对比**：

| 特性 | Questionnaire | AnswerSheet | Validation |
| ----- | --------------- | ------------- | ------------ |
| **类型** | 聚合 | 聚合 | 子域 |
| **核心对象** | Question | Answer | ValidationStrategy |
| **扩展模式** | 注册器 + 工厂 | 简单工厂 | 注册器 + 策略 |
| **领域服务** | 5个 | 0个 | 1个（Validator） |
| **连接方式** | - | 适配器 | 接口 |

**关键设计模式**：

* ✅ **策略模式**：每种校验规则是一个策略
* ✅ **注册器模式**：自动注册所有策略
* ✅ **适配器模式**：通过 ValidatableValue 连接不同聚合
* ✅ **领域服务模式**：Validator 协调多个策略

---

## 2. ValidationRule 值对象

### 2.1 规则定义

```go
// ValidationRule 校验规则（伪代码）
type ValidationRule struct {
    typ    RuleType  // 规则类型
    params any       // 规则参数（不同类型有不同参数）
}

// 创建各类规则的工厂方法
func NewRequiredRule() ValidationRule
func NewMinLengthRule(minLength int) ValidationRule
func NewMaxLengthRule(maxLength int) ValidationRule
func NewMinValueRule(minValue float64) ValidationRule
func NewMaxValueRule(maxValue float64) ValidationRule
func NewMinSelectionsRule(min int) ValidationRule
func NewMaxSelectionsRule(max int) ValidationRule
func NewPatternRule(pattern string) ValidationRule
```

> **查看完整实现**：[internal/apiserver/domain/survey/validation/rule.go](../../internal/apiserver/domain/survey/validation/rule.go)

### 2.2 RuleType 枚举

```go
// RuleType 规则类型（伪代码）
type RuleType string

const (
    RuleTypeRequired       = "required"        // 必填
    RuleTypeMinLength      = "min_length"      // 最小长度
    RuleTypeMaxLength      = "max_length"      // 最大长度
    RuleTypeMinValue       = "min_value"       // 最小值
    RuleTypeMaxValue       = "max_value"       // 最大值
    RuleTypeMinSelections  = "min_selections"  // 最少选择数
    RuleTypeMaxSelections  = "max_selections"  // 最多选择数
    RuleTypePattern        = "pattern"         // 正则表达式
)
```

> **查看完整实现**：[internal/apiserver/domain/survey/validation/rule.go](../../internal/apiserver/domain/survey/validation/rule.go)

### 2.3 规则使用示例

```go
// 为文本题设置校验规则
rules := []ValidationRule{
    NewRequiredRule(),                    // 必填
    NewMinLengthRule(2),                  // 至少2个字符
    NewMaxLengthRule(50),                 // 最多50个字符
    NewPatternRule("^[a-zA-Z\\s]+$"),    // 只能包含字母和空格
}

// 为数字题设置校验规则
rules := []ValidationRule{
    NewRequiredRule(),      // 必填
    NewMinValueRule(0),     // 最小值0
    NewMaxValueRule(150),   // 最大值150
}

// 为多选题设置校验规则
rules := []ValidationRule{
    NewMinSelectionsRule(1),  // 至少选1个
    NewMaxSelectionsRule(3),  // 最多选3个
}
```

---

## 3. ValidatableValue 接口

### 3.1 接口定义

**作用**：解耦 Validation 子域与其他聚合。

```go
// ValidatableValue 可校验值接口（伪代码）
type ValidatableValue interface {
    IsEmpty() bool                    // 是否为空
    AsString() string                 // 转为字符串
    AsNumber() (float64, error)       // 转为数字
    AsArray() []string                // 转为字符串数组
}
```

> **查看完整实现**：[internal/apiserver/domain/survey/validation/validator.go](../../internal/apiserver/domain/survey/validation/validator.go)

### 3.2 为什么需要这个接口？

**问题**：

* Answer 有自己的 AnswerValue 接口
* Validation 不应该依赖 AnswerSheet 聚合
* 不同聚合可能有不同的值类型

**解决方案**：定义通用接口 ValidatableValue

```text
┌─────────────────────────────────────┐
│    Validation 子域                   │
│                                     │
│  ValidatableValue 接口              │
│  ├── IsEmpty() bool                 │
│  ├── AsString() string              │
│  ├── AsNumber() (float64, error)   │
│  └── AsArray() []string             │
└──────────────┬──────────────────────┘
               │ 需要实现
               │
    ┌──────────┴──────────┐
    │                     │
    ▼                     ▼
┌─────────────┐   ┌──────────────┐
│AnswerValue  │   │  其他值类型   │
│Adapter      │   │  的适配器     │
└─────────────┘   └──────────────┘
```

### 3.3 AnswerValueAdapter 实现

```go
// AnswerValueAdapter 答案值适配器（伪代码）
type AnswerValueAdapter struct {
    answerValue AnswerValue
}

func NewAnswerValueAdapter(value AnswerValue) ValidatableValue {
    return &AnswerValueAdapter{answerValue: value}
}

func (a *AnswerValueAdapter) IsEmpty() bool {
    // 根据 Raw() 返回的类型判断
}

func (a *AnswerValueAdapter) AsString() string {
    // 将 Raw() 转换为字符串
}

func (a *AnswerValueAdapter) AsNumber() (float64, error) {
    // 将 Raw() 转换为数字
}

func (a *AnswerValueAdapter) AsArray() []string {
    // 将 Raw() 转换为字符串数组
}
```

> **查看完整实现**：[internal/apiserver/domain/survey/answersheet/validation_adapter.go](../../internal/apiserver/domain/survey/answersheet/validation_adapter.go)

---

## 4. ValidationStrategy 策略模式

### 4.1 策略接口

```go
// ValidationStrategy 校验策略接口（伪代码）
type ValidationStrategy interface {
    // 校验值是否满足规则
    Validate(value ValidatableValue, rule ValidationRule) error
}
```

> **查看完整实现**：[internal/apiserver/domain/survey/validation/strategy.go](../../internal/apiserver/domain/survey/validation/strategy.go)

### 4.2 策略注册器

```go
// 策略注册器（伪代码）
var strategyRegistry = map[RuleType]ValidationStrategy{}

// 注册策略（在各策略的 init() 中调用）
func RegisterStrategy(ruleType RuleType, strategy ValidationStrategy) {
    strategyRegistry[ruleType] = strategy
}

// 获取策略
func GetStrategy(ruleType RuleType) (ValidationStrategy, bool) {
    strategy, ok := strategyRegistry[ruleType]
    return strategy, ok
}
```

> **查看完整实现**：[internal/apiserver/domain/survey/validation/strategy.go](../../internal/apiserver/domain/survey/validation/strategy.go)

### 4.3 自动注册机制

```go
// 各策略在 init() 中自动注册（伪代码）
func init() {
    RegisterStrategy(RuleTypeRequired, &RequiredStrategy{})
    RegisterStrategy(RuleTypeMinLength, &MinLengthStrategy{})
    RegisterStrategy(RuleTypeMaxLength, &MaxLengthStrategy{})
    // ... 其他策略
}
```

**优点**：

* ✅ 新增策略只需实现接口 + init() 注册
* ✅ 无需手动维护策略列表
* ✅ 编译时就完成注册

---

## 5. 8种校验策略实现

### 5.1 RequiredStrategy（必填校验）

**规则**：值不能为空

```go
type RequiredStrategy struct{}

func (s *RequiredStrategy) Validate(value ValidatableValue, rule ValidationRule) error {
    if value.IsEmpty() {
        return errors.New("此项为必填项")
    }
    return nil
}
```

> **查看完整实现**：[internal/apiserver/domain/survey/validation/required.go](../../internal/apiserver/domain/survey/validation/required.go)

### 5.2 MinLengthStrategy（最小长度）

**规则**：字符串长度不能小于指定值（按UTF-8字符数计算）

```go
type MinLengthStrategy struct{}

func (s *MinLengthStrategy) Validate(value ValidatableValue, rule ValidationRule) error {
    str := value.AsString()
    length := utf8.RuneCountInString(str)
    minLength := rule.GetParams().(int)
    
    if length < minLength {
        return fmt.Errorf("长度不能少于%d个字符", minLength)
    }
    return nil
}
```

> **查看完整实现**：[internal/apiserver/domain/survey/validation/min_length.go](../../internal/apiserver/domain/survey/validation/min_length.go)

**为什么用 utf8.RuneCountInString？**

* ✅ "你好" = 2个字符（而非6个字节）
* ✅ 符合用户直觉

### 5.3 MaxLengthStrategy（最大长度）

**规则**：字符串长度不能超过指定值

```go
type MaxLengthStrategy struct{}

func (s *MaxLengthStrategy) Validate(value ValidatableValue, rule ValidationRule) error {
    str := value.AsString()
    length := utf8.RuneCountInString(str)
    maxLength := rule.GetParams().(int)
    
    if length > maxLength {
        return fmt.Errorf("长度不能超过%d个字符", maxLength)
    }
    return nil
}
```

> **查看完整实现**：[internal/apiserver/domain/survey/validation/max_length.go](../../internal/apiserver/domain/survey/validation/max_length.go)

### 5.4 MinValueStrategy（最小值）

**规则**：数值不能小于指定值

```go
type MinValueStrategy struct{}

func (s *MinValueStrategy) Validate(value ValidatableValue, rule ValidationRule) error {
    num, err := value.AsNumber()
    if err != nil {
        return errors.New("无效的数值")
    }
    
    minValue := rule.GetParams().(float64)
    if num < minValue {
        return fmt.Errorf("值不能小于%.2f", minValue)
    }
    return nil
}
```

> **查看完整实现**：[internal/apiserver/domain/survey/validation/min_value.go](../../internal/apiserver/domain/survey/validation/min_value.go)

### 5.5 MaxValueStrategy（最大值）

**规则**：数值不能大于指定值

```go
type MaxValueStrategy struct{}

func (s *MaxValueStrategy) Validate(value ValidatableValue, rule ValidationRule) error {
    num, err := value.AsNumber()
    if err != nil {
        return errors.New("无效的数值")
    }
    
    maxValue := rule.GetParams().(float64)
    if num > maxValue {
        return fmt.Errorf("值不能大于%.2f", maxValue)
    }
    return nil
}
```

> **查看完整实现**：[internal/apiserver/domain/survey/validation/max_value.go](../../internal/apiserver/domain/survey/validation/max_value.go)

### 5.6 MinSelectionsStrategy（最少选择）

**规则**：多选题至少选择N个选项

```go
type MinSelectionsStrategy struct{}

func (s *MinSelectionsStrategy) Validate(value ValidatableValue, rule ValidationRule) error {
    selections := value.AsArray()
    minSelections := rule.GetParams().(int)
    
    if len(selections) < minSelections {
        return fmt.Errorf("至少选择%d项", minSelections)
    }
    return nil
}
```

> **查看完整实现**：[internal/apiserver/domain/survey/validation/selections.go](../../internal/apiserver/domain/survey/validation/selections.go)

### 5.7 MaxSelectionsStrategy（最多选择）

**规则**：多选题最多选择N个选项

```go
type MaxSelectionsStrategy struct{}

func (s *MaxSelectionsStrategy) Validate(value ValidatableValue, rule ValidationRule) error {
    selections := value.AsArray()
    maxSelections := rule.GetParams().(int)
    
    if len(selections) > maxSelections {
        return fmt.Errorf("最多选择%d项", maxSelections)
    }
    return nil
}
```

> **查看完整实现**：[internal/apiserver/domain/survey/validation/selections.go](../../internal/apiserver/domain/survey/validation/selections.go)

### 5.8 PatternStrategy（正则表达式）

**规则**：字符串必须匹配正则表达式

```go
type PatternStrategy struct{}

func (s *PatternStrategy) Validate(value ValidatableValue, rule ValidationRule) error {
    str := value.AsString()
    pattern := rule.GetParams().(string)
    
    matched, err := regexp.MatchString(pattern, str)
    if err != nil {
        return errors.New("正则表达式无效")
    }
    
    if !matched {
        return errors.New("格式不正确")
    }
    return nil
}
```

> **查看完整实现**：[internal/apiserver/domain/survey/validation/pattern.go](../../internal/apiserver/domain/survey/validation/pattern.go)

**常用正则示例**：

```go
// 邮箱
NewPatternRule(`^[\w-\.]+@([\w-]+\.)+[\w-]{2,4}$`)

// 手机号
NewPatternRule(`^1[3-9]\d{9}$`)

// 身份证号
NewPatternRule(`^\d{17}[\dXx]$`)

// 只包含字母
NewPatternRule(`^[a-zA-Z]+$`)
```

---

## 6. Validator 领域服务

### 6.1 Validator 接口

```go
// Validator 校验器接口（伪代码）
type Validator interface {
    // 校验单个值
    ValidateValue(value ValidatableValue, rules []ValidationRule) ValidationResult
    
    // 批量校验多个值
    ValidateValues(values map[string]ValidatableValue, rulesMap map[string][]ValidationRule) ValidationResult
}
```

> **查看完整实现**：[internal/apiserver/domain/survey/validation/validator.go](../../internal/apiserver/domain/survey/validation/validator.go)

### 6.2 DefaultValidator 实现

```go
// DefaultValidator 默认校验器（伪代码）
type DefaultValidator struct{}

func NewDefaultValidator() Validator {
    return &DefaultValidator{}
}

func (v *DefaultValidator) ValidateValue(
    value ValidatableValue, 
    rules []ValidationRule,
) ValidationResult {
    errors := []ValidationError{}
    
    // 遍历所有规则
    for _, rule := range rules {
        // 1. 获取对应的策略
        strategy, ok := GetStrategy(rule.GetType())
        if !ok {
            continue  // 未知规则类型，跳过
        }
        
        // 2. 执行校验
        if err := strategy.Validate(value, rule); err != nil {
            errors = append(errors, NewValidationError(rule.GetType(), err.Error()))
        }
    }
    
    return NewValidationResult(errors)
}
```

> **查看完整实现**：[internal/apiserver/domain/survey/validation/validator.go](../../internal/apiserver/domain/survey/validation/validator.go)

### 6.3 ValidationResult 值对象

```go
// ValidationResult 校验结果（伪代码）
type ValidationResult struct {
    errors []ValidationError
}

func (r ValidationResult) IsValid() bool {
    return len(r.errors) == 0
}

func (r ValidationResult) GetErrors() []ValidationError {
    return r.errors
}
```

### 6.4 ValidationError 值对象

```go
// ValidationError 校验错误（伪代码）
type ValidationError struct {
    ruleType RuleType  // 规则类型
    message  string    // 错误消息
}

func (e ValidationError) GetRuleType() RuleType {
    return e.ruleType
}

func (e ValidationError) GetMessage() string {
    return e.message
}
```

---

## 7. 完整使用示例

### 7.1 为 Question 定义校验规则

```go
// 创建单行文本题，带校验规则
question, _ := NewQuestion(
    WithCode(meta.NewCode("name")),
    WithType(TypeText),
    WithStem("请输入您的姓名"),
    WithRequired(true),
    WithValidationRules([]ValidationRule{
        NewRequiredRule(),                    // 必填
        NewMinLengthRule(2),                  // 至少2个字符
        NewMaxLengthRule(20),                 // 最多20个字符
        NewPatternRule("^[\\u4e00-\\u9fa5]+$"),  // 只能是中文
    }),
)
```

### 7.2 校验答案

```go
// 1. 创建答案
answerValue := NewStringValue("张三")
answer, _ := NewAnswer(
    meta.NewCode("name"),
    TypeText,
    answerValue,
    0,
)

// 2. 获取校验规则
question := findQuestion("name")
rules := question.GetValidationRules()

// 3. 通过适配器校验
validator := NewDefaultValidator()
validatableValue := NewAnswerValueAdapter(answer.Value())
result := validator.ValidateValue(validatableValue, rules)

// 4. 处理校验结果
if !result.IsValid() {
    for _, err := range result.GetErrors() {
        fmt.Printf("校验失败 [%s]: %s\n", err.GetRuleType(), err.GetMessage())
    }
} else {
    fmt.Println("校验通过")
}
```

### 7.3 批量校验多个答案

```go
// 准备数据
values := map[string]ValidatableValue{
    "Q1": NewAnswerValueAdapter(answer1.Value()),
    "Q2": NewAnswerValueAdapter(answer2.Value()),
    "Q3": NewAnswerValueAdapter(answer3.Value()),
}

rulesMap := map[string][]ValidationRule{
    "Q1": question1.GetValidationRules(),
    "Q2": question2.GetValidationRules(),
    "Q3": question3.GetValidationRules(),
}

// 批量校验
validator := NewDefaultValidator()
result := validator.ValidateValues(values, rulesMap)

if !result.IsValid() {
    // 处理错误
}
```

---

## 8. 扩展示例：新增日期范围校验

**场景**：校验日期是否在指定范围内

### 步骤 1：定义新规则类型

```go
// 新增规则类型
const (
    RuleTypeDateRange = "date_range"  // 日期范围
)

// 创建规则的工厂方法
func NewDateRangeRule(minDate, maxDate string) ValidationRule {
    return ValidationRule{
        typ: RuleTypeDateRange,
        params: map[string]string{
            "min": minDate,
            "max": maxDate,
        },
    }
}
```

### 步骤 2：实现校验策略

```go
// DateRangeStrategy 日期范围校验策略
type DateRangeStrategy struct{}

func (s *DateRangeStrategy) Validate(value ValidatableValue, rule ValidationRule) error {
    dateStr := value.AsString()
    date, err := time.Parse("2006-01-02", dateStr)
    if err != nil {
        return errors.New("无效的日期格式")
    }
    
    params := rule.GetParams().(map[string]string)
    minDate, _ := time.Parse("2006-01-02", params["min"])
    maxDate, _ := time.Parse("2006-01-02", params["max"])
    
    if date.Before(minDate) || date.After(maxDate) {
        return fmt.Errorf("日期必须在%s至%s之间", params["min"], params["max"])
    }
    return nil
}
```

### 步骤 3：注册策略

```go
// 在 init() 中自动注册
func init() {
    RegisterStrategy(RuleTypeDateRange, &DateRangeStrategy{})
}
```

### 步骤 4：使用

```go
// 为日期题添加日期范围校验
question, _ := NewQuestion(
    WithCode(meta.NewCode("birthdate")),
    WithType(TypeDate),
    WithStem("请输入您的出生日期"),
    WithValidationRules([]ValidationRule{
        NewRequiredRule(),
        NewDateRangeRule("1900-01-01", "2025-12-31"),
    }),
)
```

✅ **完成！** 只需4个步骤，无需修改现有代码。

---

## 9. 设计模式总结

Validation 子域使用的设计模式：

| 模式 | 应用位置 | 价值 |
| ----- | --------- | ------ |
| **策略模式** | ValidationStrategy | 每种校验规则独立实现 |
| **注册器模式** | strategyRegistry | 自动注册所有策略 |
| **适配器模式** | ValidatableValue + AnswerValueAdapter | 解耦不同聚合 |
| **领域服务模式** | Validator | 协调多个策略执行校验 |
| **值对象模式** | ValidationRule, ValidationResult | 封装规则和结果 |

### 9.1 策略模式 + 注册器模式的优势

**对比传统 if-else 方式**：

```go
// ❌ 传统方式（不推荐）
func validate(value string, rule ValidationRule) error {
    switch rule.GetType() {
    case "required":
        if value == "" {
            return errors.New("必填")
        }
    case "min_length":
        if len(value) < rule.Params.(int) {
            return errors.New("太短")
        }
    // ... 更多 case
    default:
        return errors.New("unknown rule")
    }
}
```

**问题**：

* ❌ 违反开闭原则（新增规则需修改函数）
* ❌ 单个函数过长
* ❌ 难以测试
* ❌ 难以复用

**✅ 策略模式 + 注册器方式**：

```go
// ✅ 策略模式（推荐）
// 1. 新增策略只需实现接口
type NewStrategy struct{}
func (s *NewStrategy) Validate(value ValidatableValue, rule ValidationRule) error {
    // ...
}

// 2. 在 init() 中注册
func init() {
    RegisterStrategy(RuleTypeNew, &NewStrategy{})
}

// 3. Validator 自动使用
```

**优势**：

* ✅ 开闭原则：新增策略无需修改现有代码
* ✅ 单一职责：每个策略只负责一种校验
* ✅ 易于测试：可单独测试每个策略
* ✅ 易于扩展：添加新策略非常简单

---

## 10. 架构价值分析

### 10.1 为什么独立为子域？

**对比**：如果将校验逻辑放在 Questionnaire 聚合中

| 方面 | 放在 Questionnaire | 独立 Validation 子域 |
| ----- | ------------------- | ------------------- |
| **职责** | Questionnaire 负责校验 | Validation 独立职责 |
| **复用** | 只能用于 Question | 任何需要校验的地方 |
| **扩展** | 修改 Questionnaire | 不影响其他聚合 |
| **测试** | 测试聚合+校验 | 独立测试校验 |
| **依赖** | 其他聚合依赖 Questionnaire | 通过接口解耦 |

**独立子域的价值**：

* ✅ **单一职责**：专注于校验
* ✅ **高内聚**：校验相关逻辑集中
* ✅ **低耦合**：通过接口连接其他聚合
* ✅ **可复用**：不限于 Survey 子域
* ✅ **易扩展**：策略模式支持无限扩展

### 10.2 接口隔离原则

**ValidatableValue 接口设计**：

```go
type ValidatableValue interface {
    IsEmpty() bool
    AsString() string
    AsNumber() (float64, error)
    AsArray() []string
}
```

**为什么不直接使用 AnswerValue？**

| 方案 | 优点 | 缺点 |
| ----- | ------ | ------ |
| **直接使用 AnswerValue** | 简单直接 | ❌ Validation 依赖 AnswerSheet<br>❌ 无法复用到其他场景 |
| **定义 ValidatableValue** | ✅ 解耦<br>✅ 可复用 | 需要适配器 |

**接口隔离的价值**：

* ✅ Validation 子域不知道 AnswerValue 的存在
* ✅ 任何实现 ValidatableValue 的类型都可以校验
* ✅ 便于单元测试（Mock ValidatableValue）

---

## 11. 下一步阅读

* **[11-04-05 应用服务层设计](./11-04-05-应用服务层设计.md)** - 如何在应用服务中使用 Validation
* **[11-04-06 设计模式应用总结](./11-04-06-设计模式应用总结.md)** - 7种模式的对比与选择
* **[11-04-07 扩展指南](./11-04-07-扩展指南.md)** - 完整的扩展实战示例

---

> **相关文档**：
>
> * [Survey 子域设计系列](./11-04-Survey子域设计系列.md) - 系列文档索引
> * [11-04-01 Survey 子域架构总览](./11-04-01-Survey子域架构总览.md) - 架构设计
> * [11-04-02 Questionnaire 聚合设计](./11-04-02-Questionnaire聚合设计.md) - 题型设计
> * [11-04-03 AnswerSheet 聚合设计](./11-04-03-AnswerSheet聚合设计.md) - 答案设计
