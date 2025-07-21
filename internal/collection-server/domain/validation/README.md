# Domain Validation 架构

## 概述

本模块实现了基于策略模式的验证架构，提供了灵活、可扩展的数据验证功能。

## 架构设计

### 核心组件

1. **Rules (规则)** - 定义验证规则
2. **Strategies (策略)** - 实现具体的验证逻辑
3. **Validator (验证器)** - 协调规则和策略进行验证
4. **Builders (构建器)** - 提供便捷的规则创建方式

### 目录结构

```
validation/
├── rules/           # 验证规则定义
│   ├── rule.go      # 基础规则接口和类型
│   ├── required.go  # 必填验证规则
│   ├── length.go    # 长度验证规则
│   └── value.go     # 数值验证规则
├── strategies/      # 验证策略实现
│   ├── strategy.go  # 策略接口和工厂
│   ├── strategies.go # 具体策略实现
│   └── factory.go   # 策略工厂
├── validator.go     # 主验证器
├── builders.go      # 规则构建器
└── README.md        # 本文档
```

## 使用方式

### 1. 基本验证

```go
import "github.com/yshujie/questionnaire-scale/internal/collection-server/domain/validation"

// 创建验证器
validator := validation.NewValidator()

// 创建验证规则
rule := validation.Required("此字段为必填项")

// 验证单个值
err := validator.Validate("", rule)
if err != nil {
    // 处理验证错误
}
```

### 2. 多个规则验证

```go
rules := []*rules.BaseRule{
    validation.Required("此字段为必填项"),
    validation.MinLength(3, "长度不能少于3个字符"),
    validation.MaxLength(10, "长度不能超过10个字符"),
}

errors := validator.ValidateMultiple("hi", rules)
if len(errors) > 0 {
    // 处理验证错误
}
```

### 3. 结构体验证

```go
type User struct {
    Name  string  `json:"name"`
    Age   int     `json:"age"`
    Email string  `json:"email"`
}

user := User{
    Name:  "John",
    Age:   25,
    Email: "john@example.com",
}

rules := map[string][]*rules.BaseRule{
    "name": {
        validation.Required("姓名不能为空"),
        validation.MinLength(2, "姓名长度不能少于2个字符"),
    },
    "age": {
        validation.MinValue(18, "年龄不能小于18岁"),
        validation.MaxValue(100, "年龄不能大于100岁"),
    },
    "email": {
        validation.Required("邮箱不能为空"),
        validation.Email("邮箱格式不正确"),
    },
}

errors := validator.ValidateStruct(user, rules)
if len(errors) > 0 {
    // 处理验证错误
}
```

### 4. 使用构建器

```go
// 字符串规则构建器
stringRules := validation.NewStringRules().
    SetRequired(true).
    SetMinLength(3).
    SetMaxLength(10).
    SetPattern(`^[a-zA-Z]+$`).
    SetEmail(false)

rules := stringRules.Build()

// 数值规则构建器
numberRules := validation.NewNumberRules().
    SetRequired(true).
    SetMinValue(0).
    SetMaxValue(100)

rules := numberRules.Build()
```

## 内置验证规则

### 基础规则

- **required** - 必填验证
- **min_length** - 最小长度验证
- **max_length** - 最大长度验证
- **min_value** - 最小值验证
- **max_value** - 最大值验证
- **pattern** - 正则表达式验证
- **email** - 邮箱格式验证

### 便捷函数

- `Required(message)` - 创建必填规则
- `MinLength(length, message)` - 创建最小长度规则
- `MaxLength(length, message)` - 创建最大长度规则
- `MinValue(value, message)` - 创建最小值规则
- `MaxValue(value, message)` - 创建最大值规则
- `Pattern(pattern, message)` - 创建正则表达式规则
- `Email(message)` - 创建邮箱规则
- `Range(min, max, message)` - 创建范围规则

## 扩展验证策略

### 1. 创建自定义策略

```go
type CustomStrategy struct {
    strategies.BaseStrategy
}

func NewCustomStrategy() *CustomStrategy {
    return &CustomStrategy{
        BaseStrategy: strategies.BaseStrategy{Name: "custom"},
    }
}

func (s *CustomStrategy) Validate(value interface{}, rule *rules.BaseRule) error {
    // 实现自定义验证逻辑
    return nil
}
```

### 2. 注册自定义策略

```go
validator := validation.NewValidator()
customStrategy := NewCustomStrategy()
validator.RegisterCustomStrategy(customStrategy)
```

## 错误处理

验证错误实现了标准的 `error` 接口，包含以下信息：

- **Field** - 字段名
- **Message** - 错误消息
- **Value** - 验证的值
- **Rule** - 验证规则名

```go
if err != nil {
    if validationErr, ok := err.(*rules.ValidationError); ok {
        fmt.Printf("字段: %s, 错误: %s, 值: %v, 规则: %s\n",
            validationErr.Field,
            validationErr.Message,
            validationErr.Value,
            validationErr.Rule)
    }
}
```

## 性能考虑

1. **策略工厂使用单例模式** - 避免重复创建策略实例
2. **规则缓存** - 验证器会缓存已创建的规则
3. **并发安全** - 验证器使用读写锁保证并发安全

## 最佳实践

1. **复用验证器实例** - 避免重复创建验证器
2. **使用构建器模式** - 提高代码可读性
3. **合理设置验证规则** - 避免过度验证影响性能
4. **统一错误处理** - 使用统一的错误处理方式
5. **扩展性考虑** - 通过策略模式支持自定义验证逻辑

## 测试

运行测试：

```bash
go test ./domain/validation/... -v
```

测试覆盖了以下场景：
- 各种验证规则的正确性
- 多个规则组合验证
- 结构体验证
- 策略工厂功能
- 构建器模式
- 错误处理 