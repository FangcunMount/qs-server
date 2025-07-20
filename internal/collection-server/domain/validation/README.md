# 答卷答案验证系统

这是一个基于策略模式的答卷答案验证系统，专门用于验证问卷填写时的答案是否合规。

## 设计理念

- **策略模式**: 每种验证规则都是一个独立的策略，易于扩展和维护
- **业务导向**: 专门针对问卷答案验证场景设计
- **类型安全**: 支持多种答案类型（字符串、数字、选项等）
- **可扩展**: 支持自定义验证策略

## 核心组件

### 1. 验证策略 (ValidationStrategy)

每种验证规则都实现了 `ValidationStrategy` 接口：

```go
type ValidationStrategy interface {
    Validate(value interface{}, rule *ValidationRule) error
    GetStrategyName() string
}
```

### 2. 验证规则 (ValidationRule)

验证规则包含策略名称、规则值、错误消息和额外参数：

```go
type ValidationRule struct {
    Strategy   string                 // 策略名称
    Value      interface{}            // 规则值
    Message    string                 // 错误消息
    Params     map[string]interface{} // 额外参数
}
```

### 3. 验证器 (Validator)

验证器负责管理策略和执行验证：

```go
type Validator struct {
    strategies map[string]ValidationStrategy
    mu         sync.RWMutex
}
```

## 内置验证策略

### 1. 必填验证 (RequiredStrategy)
- **策略名**: `required`
- **用途**: 验证答案不能为空
- **支持类型**: 字符串、选项值、切片、指针

```go
rule := Required("请填写答案")
```

### 2. 选项代码验证 (OptionCodeStrategy)
- **策略名**: `option_code`
- **用途**: 验证选择的选项是否在允许范围内
- **支持类型**: 字符串、选项对象

```go
rule := OptionCode([]string{"A", "B", "C", "D"}, "选择的选项不在允许范围内")
```

### 3. 数值范围验证 (RangeStrategy)
- **策略名**: `range`
- **用途**: 验证数值是否在指定范围内
- **支持类型**: 各种数字类型、字符串数字

```go
rule := Range(0, 100, "答案必须在0到100之间")
```

### 4. 最大值验证 (MaxValueStrategy)
- **策略名**: `max_value`
- **用途**: 验证数值不能超过最大值

```go
rule := MaxValue(100, "答案不能超过100")
```

### 5. 最小值验证 (MinValueStrategy)
- **策略名**: `min_value`
- **用途**: 验证数值不能小于最小值

```go
rule := MinValue(0, "答案不能小于0")
```

### 6. 最大长度验证 (MaxLengthStrategy)
- **策略名**: `max_length`
- **用途**: 验证文本长度不能超过最大值

```go
rule := MaxLength(500, "答案长度不能超过500个字符")
```

### 7. 最小长度验证 (MinLengthStrategy)
- **策略名**: `min_length`
- **用途**: 验证文本长度不能少于最小值

```go
rule := MinLength(10, "答案长度不能少于10个字符")
```

### 8. 正则表达式验证 (PatternStrategy)
- **策略名**: `pattern`
- **用途**: 使用正则表达式验证答案格式

```go
rule := Pattern(`^[^<>]*$`, "答案不能包含HTML标签")
```

### 9. 邮箱验证 (EmailStrategy)
- **策略名**: `email`
- **用途**: 验证邮箱格式

```go
rule := Email("邮箱格式不正确")
```

### 10. 手机号验证 (PhoneStrategy)
- **策略名**: `phone`
- **用途**: 验证手机号格式（中国大陆）

```go
rule := Phone("手机号格式不正确")
```

## 使用示例

### 基本使用

```go
// 创建验证器
validator := NewValidator()

// 定义验证规则
rules := []*ValidationRule{
    Required("请填写答案"),
    MinLength(10, "答案长度不能少于10个字符"),
    MaxLength(500, "答案长度不能超过500个字符"),
}

// 验证答案
answer := "这是一个测试答案"
errors := validator.ValidateMultiple(answer, rules)

if len(errors) > 0 {
    fmt.Printf("验证失败: %v\n", errors[0].Error())
} else {
    fmt.Println("验证通过")
}
```

### 不同题型的验证规则

```go
// 单选题验证
radioRules := []*ValidationRule{
    Required("请选择一个选项"),
    OptionCode([]string{"A", "B", "C", "D"}, "选择的选项不在允许范围内"),
}

// 文本题验证
textRules := []*ValidationRule{
    Required("请填写答案"),
    MinLength(10, "答案长度不能少于10个字符"),
    MaxLength(500, "答案长度不能超过500个字符"),
    Pattern(`^[^<>]*$`, "答案不能包含HTML标签"),
}

// 数字题验证
numberRules := []*ValidationRule{
    Required("请填写数字"),
    Range(0, 100, "答案必须在0到100之间"),
}

// 邮箱题验证
emailRules := []*ValidationRule{
    Required("请填写邮箱"),
    Email("邮箱格式不正确"),
}
```

### 使用构建器模式

```go
// 字符串规则构建器
stringRules := NewStringRules().
    SetRequired(true).
    SetMinLength(10).
    SetMaxLength(200).
    SetPattern(`^[^<>]*$`)

rules := stringRules.Build()

// 数值规则构建器
numberRules := NewNumberRules().
    SetRequired(true).
    SetRange(0, 100)

rules := numberRules.Build()
```

### 自定义验证策略

```go
// 自定义策略
type CustomStrategy struct{}

func (s *CustomStrategy) Validate(value interface{}, rule *ValidationRule) error {
    // 自定义验证逻辑
    return nil
}

func (s *CustomStrategy) GetStrategyName() string {
    return "custom"
}

// 注册自定义策略
validator := NewValidator()
validator.RegisterStrategy(&CustomStrategy{})

// 使用自定义策略
rule := NewRule("custom").
    WithValue([]string{"敏感词1", "敏感词2"}).
    WithMessage("答案包含敏感词汇").
    Build()
```

## 答案类型支持

### 1. 字符串答案
```go
answer := "这是一个文本答案"
```

### 2. 选项答案
```go
answer := "A" // 选项代码
// 或者
answer := map[string]interface{}{
    "code": "A",
    "text": "选项A",
}
```

### 3. 数值答案
```go
answer := 75 // 整数
answer := 75.5 // 浮点数
answer := "75" // 字符串数字
```

### 4. 多选答案
```go
answer := []string{"A", "B", "C"} // 字符串数组
answer := []interface{}{"A", "B", "C"} // 接口数组
```

## 错误处理

验证错误包含字段名、错误消息和答案值：

```go
type ValidationError struct {
    Field   string      // 字段名
    Message string      // 错误消息
    Value   interface{} // 答案值
}
```

## 性能考虑

- 验证器使用读写锁保证并发安全
- 策略注册在初始化时完成，运行时只读
- 支持空值跳过验证，提高性能
- 验证规则可以缓存和复用

## 扩展指南

### 添加新的验证策略

1. 实现 `ValidationStrategy` 接口
2. 在 `RegisterDefaultStrategies` 中注册策略
3. 添加便捷的创建函数（可选）

```go
// 1. 实现策略
type NewStrategy struct{}

func (s *NewStrategy) Validate(value interface{}, rule *ValidationRule) error {
    // 验证逻辑
    return nil
}

func (s *NewStrategy) GetStrategyName() string {
    return "new_strategy"
}

// 2. 注册策略
func (v *Validator) RegisterDefaultStrategies() {
    // ... 其他策略
    v.RegisterStrategy(&NewStrategy{})
}

// 3. 添加便捷函数（可选）
func NewStrategyRule(value interface{}, message string) *ValidationRule {
    if message == "" {
        message = "自定义验证失败"
    }
    return NewRule("new_strategy").WithValue(value).WithMessage(message).Build()
}
```

这个验证系统专门为问卷答案验证设计，提供了灵活、可扩展的验证能力，能够满足各种问卷题型的验证需求。 