# Validation 领域设计

## 核心设计原则

**validation 领域只关心三件事**：

1. **校验规则**（ValidationRule）
2. **校验值**（ValidatableValue）
3. **校验结果**（ValidationResult）

**完全不依赖**问卷（Questionnaire）和答卷（AnswerSheet）等业务对象。

## 架构图

```text
┌─────────────────────────────────────────┐
│      validation 包（独立领域）            │
├─────────────────────────────────────────┤
│                                         │
│  ValidationRule (值对象)                 │
│  RuleType (枚举)                        │
│                                         │
│  ValidatableValue (抽象接口)             │
│  ├─ IsEmpty()                          │
│  ├─ AsString()                         │
│  ├─ AsNumber()                         │
│  └─ AsArray()                          │
│                                         │
│  Validator (校验器)                     │
│  └─ ValidateValue(value, rules)        │
│                                         │
│  ValidationStrategy (策略接口)          │
│  └─ Validate(value, rule)              │
│                                         │
│  ValidationResult (结果)                │
│  └─ ValidationError[]                  │
│                                         │
└─────────────────────────────────────────┘
              ▲
              │ 实现接口
              │
┌─────────────┴───────────────────────────┐
│    answersheet/answer 包                │
│                                         │
│  AnswerValue 实现 ValidatableValue      │
└─────────────────────────────────────────┘
```

## 依赖方向

```text
validation 包（定义接口和规则）
    ↑
    │ 实现 ValidatableValue 接口
    │
answersheet 包（答案值）

    ↑
    │ 使用 validation 包
    │
应用层（协调问卷、答卷、校验）
```

## 核心接口

### 1. Validator 接口

```go
type Validator interface {
    // 校验单个值
    ValidateValue(value ValidatableValue, rules []ValidationRule) *ValidationResult
}
```

**特点**：

- ✅ 只接受抽象的 `ValidatableValue`
- ✅ 只接受规则列表 `[]ValidationRule`
- ✅ 返回纯粹的校验结果
- ✅ 不依赖任何业务对象

### 2. ValidatableValue 接口

```go
type ValidatableValue interface {
    IsEmpty() bool
    AsString() string
    AsNumber() (float64, error)
    AsArray() []string
}
```

**特点**：

- ✅ 定义在 validation 包内
- ✅ answersheet 包实现此接口
- ✅ 依赖倒置原则的体现

### 3. ValidationStrategy 接口

```go
type ValidationStrategy interface {
    Validate(value ValidatableValue, rule ValidationRule) error
    SupportRuleType() RuleType
}
```

**特点**：

- ✅ 策略模式
- ✅ 只依赖抽象接口
- ✅ 易于扩展

## 使用示例

### 场景：应用层协调校验

```go
// 应用层服务
type AnswerSheetService struct {
    validator validation.Validator
}

func (s *AnswerSheetService) SubmitAnswerSheet(
    sheetID string,
    questionnaireID string,
) error {
    // 1. 加载答卷和问卷
    sheet := s.sheetRepo.FindByID(sheetID)
    questionnaire := s.questionnaireRepo.FindByID(questionnaireID)
    
    // 2. 遍历答案进行校验
    for _, ans := range sheet.GetAnswers() {
        // 2.1 查找对应的题目
        q := questionnaire.FindQuestionByCode(ans.GetQuestionCode())
        if q == nil {
            return errors.New("题目不存在")
        }
        
        // 2.2 获取题目的校验规则
        rules := q.GetValidationRules()
        
        // 2.3 获取答案值（实现了 ValidatableValue 接口）
        answerValue := ans.GetValue()
        
        // 2.4 调用 validation 领域进行校验
        result := s.validator.ValidateValue(answerValue, rules)
        
        // 2.5 处理校验结果
        if !result.IsValid() {
            return s.handleValidationErrors(result.GetErrors())
        }
    }
    
    // 3. 提交答卷
    sheet.Submit()
    return s.sheetRepo.Save(sheet)
}
```

### 场景：实现 ValidatableValue

```go
// answersheet/answer 包
type StringValue struct {
    value string
}

// 实现 validation.ValidatableValue 接口
func (v StringValue) IsEmpty() bool {
    return v.value == ""
}

func (v StringValue) AsString() string {
    return v.value
}

func (v StringValue) AsNumber() (float64, error) {
    return strconv.ParseFloat(v.value, 64)
}

func (v StringValue) AsArray() []string {
    return []string{v.value}
}
```

### 场景：实现校验策略

```go
// validation/strategies 包
type RequiredStrategy struct{}

func (s *RequiredStrategy) SupportRuleType() validation.RuleType {
    return validation.RuleTypeRequired
}

func (s *RequiredStrategy) Validate(
    value validation.ValidatableValue,
    rule validation.ValidationRule,
) error {
    if value.IsEmpty() {
        return errors.New("不能为空")
    }
    return nil
}
```

## 设计优势

### 1. 领域独立性

- ✅ validation 领域完全独立
- ✅ 不依赖任何业务领域
- ✅ 可以单独测试、复用

### 2. 依赖倒置

- ✅ validation 定义接口（ValidatableValue）
- ✅ answersheet 实现接口
- ✅ 高层模块不依赖低层模块

### 3. 职责清晰

- ✅ validation：执行校验逻辑
- ✅ questionnaire：定义校验规则
- ✅ answersheet：提供校验值
- ✅ 应用层：协调三者

### 4. 易于扩展

- ✅ 新增规则类型：添加 RuleType + Strategy
- ✅ 新增答案类型：实现 ValidatableValue
- ✅ 自定义校验器：实现 Validator 接口

## 关键理解

**为什么 ValidationRule 在 validation 包？**

- ✅ ValidationRule 是**校验领域的核心概念**
- ✅ 定义规则的数据结构（RuleType + targetValue）
- ✅ questionnaire 使用它来**配置**校验规则
- ✅ validation 使用它来**执行**校验逻辑
- ✅ 没有循环依赖问题（validation 不依赖 questionnaire）

**应用层的职责**：

- ✅ 从 questionnaire 获取规则
- ✅ 从 answersheet 获取值
- ✅ 调用 validation 执行校验
- ✅ 处理校验结果

## 总结

这个设计完美体现了：

- ✅ **单一职责原则**：每个领域只关注自己的核心问题
- ✅ **依赖倒置原则**：通过接口解耦
- ✅ **开闭原则**：易于扩展，无需修改现有代码
- ✅ **接口隔离原则**：接口精简、职责明确
- ✅ **DDD 原则**：领域独立、边界清晰
