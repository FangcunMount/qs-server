# 答卷验证架构

## 设计理念

答卷验证架构采用了**基于问题验证规则的动态验证**设计，确保每个答案都根据其对应问题的验证规则进行校验。

## 核心组件

### 1. 接口定义（避免循环导入）

为了避免循环导入问题，我们定义了以下接口：

```go
// QuestionInfo 问题信息接口
type QuestionInfo interface {
    GetCode() string
    GetType() string
    GetOptions() []QuestionOption
    GetValidationRules() []QuestionValidationRule
}

// QuestionnaireInfo 问卷信息接口
type QuestionnaireInfo interface {
    GetCode() string
    GetQuestions() []QuestionInfo
}
```

### 2. 验证器

```go
type Validator struct {
    validationValidator *validation.Validator
}
```

## 验证流程

### 1. 单个答案验证

```go
// 根据问题验证规则验证单个答案
func (v *Validator) ValidateAnswer(ctx context.Context, answer *Answer, question QuestionInfo) error
```

验证步骤：
1. **问题匹配验证**：检查答案的问题代码和类型是否与问题匹配
2. **基础验证**：检查答案值是否为空
3. **规则验证**：根据问题的验证规则进行验证
4. **类型验证**：根据问题类型进行特定验证

### 2. 答案列表验证

```go
// 验证答案列表（需要问卷信息）
func (v *Validator) ValidateAnswers(ctx context.Context, answers []*Answer, questionnaire QuestionnaireInfo) error
```

验证步骤：
1. **问卷验证**：检查问卷是否有效
2. **问题映射**：创建问题代码到问题的映射
3. **重复检查**：确保每个问题只有一个答案
4. **逐个验证**：对每个答案调用单个答案验证

### 3. 提交请求验证

```go
// 验证提交请求
func (v *Validator) ValidateSubmitRequest(ctx context.Context, req *SubmitRequest, questionnaire QuestionnaireInfo) error
```

验证步骤：
1. **基础信息验证**：验证问卷代码、标题等
2. **测试者信息验证**：验证测试者基本信息
3. **答案验证**：调用答案列表验证

## 验证规则转换

系统会自动将问卷中的验证规则转换为验证器的规则：

```go
// 规则类型映射
"required"     → "required"     // 必填验证
"min_length"   → "min_length"   // 最小长度
"max_length"   → "max_length"   // 最大长度
"min_value"    → "min_value"    // 最小值
"max_value"    → "max_value"    // 最大值
"email"        → "email"        // 邮箱格式
"pattern"      → "pattern"      // 正则表达式
```

## 问题类型验证

### 文本类型 (text, textarea)
- 验证值是否为非空字符串
- 应用长度和格式验证规则

### 数值类型 (number, rating)
- 验证值是否为数值类型
- 应用数值范围验证规则

### 单选题 (single_choice)
- 验证值是否为非空字符串
- 验证选项是否在问题选项中

### 多选题 (multiple_choice)
- 验证值为字符串或字符串数组
- 验证所有选项都在问题选项中

## 使用示例

### 1. 创建验证器

```go
validator := answersheet.NewValidator()
```

### 2. 创建问卷适配器

```go
// 从 gRPC 获取问卷数据
questionnaireData := getQuestionnaireFromGRPC(code)

// 创建适配器
questionnaireAdapter := questionnaire.NewQuestionnaireAdapter(questionnaireData)
```

### 3. 验证提交请求

```go
err := validator.ValidateSubmitRequest(ctx, submitRequest, questionnaireAdapter)
if err != nil {
    // 处理验证错误
    return err
}
```

### 4. 单独验证答案

```go
// 获取问题
question := questionnaireAdapter.GetQuestions()[0]

// 验证答案
err := validator.ValidateAnswer(ctx, answer, question)
if err != nil {
    // 处理验证错误
    return err
}
```

## 错误处理

验证器会返回详细的错误信息，包括：
- 字段名
- 错误消息
- 错误值

```go
// 错误示例
"answer validation failed for question q1: 答案验证失败: 姓名不能为空 (值: )"
"type-specific validation failed for question q3: invalid choice: invalid_option"
```

## 扩展性

### 添加新的验证规则

1. 在 `validation` 包中添加新的验证策略
2. 在 `convertValidationRule` 方法中添加规则类型映射
3. 更新文档

### 添加新的问题类型

1. 在 `validateAnswerByType` 方法中添加新的 case
2. 实现相应的验证逻辑
3. 更新文档

## 性能考虑

- 验证器使用接口，支持依赖注入和测试
- 问题映射使用 map 提高查找效率
- 验证规则转换只在需要时进行
- 支持并发验证（在应用层实现）

## 测试

验证器设计支持单元测试：

```go
// 创建 mock 问题
mockQuestion := &MockQuestionInfo{
    Code: "q1",
    Type: "text",
    ValidationRules: []QuestionValidationRule{...},
}

// 测试验证
err := validator.ValidateAnswer(ctx, answer, mockQuestion)
assert.NoError(t, err)
``` 