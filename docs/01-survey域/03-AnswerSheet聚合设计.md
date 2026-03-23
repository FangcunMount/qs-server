# 11-04-03 AnswerSheet 聚合设计

> **版本**：V3.0  
> **最后更新**：2025-11-26  
> **状态**：✅ 已实现并验证  
> **所属系列**：[Survey 子域设计系列](./11-04-Survey子域设计系列.md)

---

## 1. AnswerSheet 聚合概览

### 1.1 聚合职责

AnswerSheet 聚合负责答卷的收集和管理，是 Survey 子域中相对简单但不可或缺的聚合：

* 📝 **答案收集**：记录用户对问卷的回答
* 🔗 **关联管理**：关联问卷和填写人
* 📊 **状态管理**：草稿 → 已提交
* 🎯 **答案查询**：提供答案的查找和访问
* ✅ **答案校验**：配合 Validation 子域进行校验

### 1.2 聚合组成

```text
AnswerSheet 聚合
├── 聚合根
│   └── AnswerSheet (answersheet.go)
│
├── 实体
│   └── Answer (answer.go)
│
├── 值对象
│   ├── Status                    (答卷状态)
│   ├── QuestionnaireRef          (问卷引用)
│   ├── FillerRef                 (填写人引用)
│   └── AnswerValue (接口)        (答案值接口)
│       ├── StringValue           (字符串值)
│       ├── NumberValue           (数字值)
│       ├── OptionValue           (单选值)
│       └── OptionsValue          (多选值)
│
├── 工厂方法
│   ├── NewAnswer                 (创建答案)
│   ├── CreateAnswerValueFromRaw  (从原始值创建答案值)
│   └── NewStringValue/NewNumberValue/... (创建具体答案值)
│
└── 适配器
    └── AnswerValueAdapter        (连接 validation 子域)
```

### 1.3 设计特点

**与 Questionnaire 聚合的对比**：

| 特性 | Questionnaire | AnswerSheet |
| ----- | --------------- | ------------- |
| **复杂度** | 高（6 种题型） | 低（4 种答案值） |
| **扩展频率** | 可能新增题型 | 答案类型固定 |
| **创建模式** | 注册器 + 工厂 | 简单工厂方法 |
| **领域服务** | 5 个领域服务 | 无领域服务 |
| **聚合根方法** | 私有方法为主 | 公共方法为主 |

**设计原则**：

* ✅ **简单优先**：答案类型少且稳定，使用简单的工厂方法
* ✅ **直接映射**：答案类型与题型一一对应
* ✅ **不可变性**：Answer 和 AnswerValue 都是不可变的
* ✅ **适配器模式**：通过适配器连接 validation 子域

---

## 2. AnswerValue 答案值设计

### 2.1 设计目标

* ✅ **类型安全**：每种答案类型有独立的结构
* ✅ **简单直观**：无需复杂的注册器
* ✅ **统一接口**：所有答案类型实现 AnswerValue 接口
* ✅ **自动映射**：根据题型自动创建对应答案值

### 2.2 AnswerValue 接口

**设计原则**：极简接口，只提供获取原始值的方法。

```go
// AnswerValue 答案值接口（伪代码）
type AnswerValue interface {
    Raw() any  // 返回原始值（序列化、持久化、展示）
}
```

> **查看完整实现**：[internal/apiserver/domain/survey/answersheet/answer.go](../../internal/apiserver/domain/survey/answersheet/answer.go)

**为什么这么简单？**

* AnswerValue 的主要职责是**类型标记**和**数据携带**
* 具体的校验逻辑在 Validation 子域中，通过 AnswerValueAdapter 适配
* 保持接口简单，便于实现和扩展

### 2.3 具体答案值实现

#### 2.3.1 StringValue（字符串答案）

**适用题型**：TextQuestion、TextareaQuestion

```go
// StringValue 字符串答案值（伪代码）
type StringValue struct { text string }
func NewStringValue(v string) AnswerValue { ... }
func (v StringValue) Raw() any { return v.text }
```

#### 2.3.2 NumberValue（数字答案）

**适用题型**：NumberQuestion

```go
// NumberValue 数字答案值（伪代码）
type NumberValue struct { value float64 }
func NewNumberValue(v float64) AnswerValue { ... }
func (v NumberValue) Raw() any { return v.value }
```

#### 2.3.3 OptionValue（单选答案）

**适用题型**：RadioQuestion

```go
// OptionValue 单选答案值（伪代码）
type OptionValue struct { code string }
func NewOptionValue(v string) AnswerValue { ... }
func (v OptionValue) Raw() any { return v.code }
```

#### 2.3.4 OptionsValue（多选答案）

**适用题型**：CheckboxQuestion

```go
// OptionsValue 多选答案值（伪代码）
type OptionsValue struct { codes []string }
func NewOptionsValue(values []string) AnswerValue { ... }
func (v OptionsValue) Raw() any { return v.codes }
```

> **查看完整实现**：[internal/apiserver/domain/survey/answersheet/answer.go](../../internal/apiserver/domain/survey/answersheet/answer.go)

### 2.4 工厂方法：根据题型创建答案值

**设计思路**：简单的映射函数，根据题型决定创建哪种答案值。

```go
// CreateAnswerValueFromRaw 工厂方法（伪代码）
func CreateAnswerValueFromRaw(qType QuestionType, raw any) (AnswerValue, error) {
    switch qType {
    case TypeRadio:
        return NewOptionValue(raw.(string))     // 单选 → OptionValue
    case TypeCheckbox:
        return NewOptionsValue(raw.([]string))  // 多选 → OptionsValue
    case TypeText, TypeTextarea:
        return NewStringValue(raw.(string))     // 文本 → StringValue
    case TypeNumber:
        return NewNumberValue(raw.(float64))    // 数字 → NumberValue
    case TypeSection:
        return nil, error("section no answer")  // 段落题无答案
    }
}
```

> **查看完整实现**（包含类型转换处理）：[internal/apiserver/domain/survey/answersheet/answer.go](../../internal/apiserver/domain/survey/answersheet/answer.go)

**使用示例**：

```go
// 创建单选答案
value1, _ := CreateAnswerValueFromRaw(questionnaire.TypeRadio, "A")
// value1 是 OptionValue{code: "A"}

// 创建多选答案
value2, _ := CreateAnswerValueFromRaw(questionnaire.TypeCheckbox, []string{"A", "C"})
// value2 是 OptionsValue{codes: ["A", "C"]}

// 创建文本答案
value3, _ := CreateAnswerValueFromRaw(questionnaire.TypeText, "张三")
// value3 是 StringValue{text: "张三"}

// 创建数字答案
value4, _ := CreateAnswerValueFromRaw(questionnaire.TypeNumber, 25)
// value4 是 NumberValue{value: 25.0}
```

### 2.5 为什么不使用注册器模式？

**对比 Question 的注册器模式**：

| 特性 | Question | AnswerValue |
| ----- | ---------- | ------------- |
| **类型数量** | 6 种（可能增加） | 4 种（基本固定） |
| **映射关系** | 复杂（多种配置） | 简单（一一对应） |
| **创建逻辑** | 复杂（需要工厂） | 简单（直接创建） |
| **扩展频率** | 较高 | 极低 |

**决策**：

* ✅ **简单性优先**：4 种类型，用 switch-case 足够清晰
* ✅ **一一映射**：题型与答案值类型一一对应
* ✅ **YAGNI 原则**：You Aren't Gonna Need It - 不需要的功能不要添加
* ✅ **可读性好**：switch-case 比注册器更直观

**如果未来需要扩展怎么办？**

```go
// 新增日期答案值
case questionnaire.TypeDate:
    if dateStr, ok := raw.(string); ok {
        return NewDateValue(dateStr), nil
    }
    return nil, fmt.Errorf("date question expects string value")
```

只需在 switch 中添加一个 case，非常简单。

---

## 3. Answer 实体设计

### 3.1 Answer 结构

```go
// Answer 答案实体（伪代码）
type Answer struct {
    questionCode Code          // 问题编码
    questionType QuestionType  // 问题类型
    score        float64       // 得分
    value        AnswerValue   // 答案值
}
```

> **查看完整实现**：[internal/apiserver/domain/survey/answersheet/answer.go](../../internal/apiserver/domain/survey/answersheet/answer.go)

**设计要点**：

* 实体而非值对象（因为有得分可能变化）
* 包含问题引用（便于查找）
* 包含得分字段（由 Scale 子域计算后更新）

### 3.2 Answer 方法摘要

```go
// 创建与访问（伪代码）
func NewAnswer(code Code, qType QuestionType, value AnswerValue, score float64) Answer
func (a Answer) QuestionCode() string
func (a Answer) QuestionType() string
func (a Answer) Score() float64
func (a Answer) Value() AnswerValue

// 不可变性（返回新对象）
func (a Answer) WithScore(score float64) Answer

// 判断方法
func (a Answer) IsEmpty() bool  // 检查答案值是否为空
```

> **查看完整实现**：[internal/apiserver/domain/survey/answersheet/answer.go](../../internal/apiserver/domain/survey/answersheet/answer.go)

---

## 4. AnswerSheet 聚合根设计

### 4.1 聚合根结构

```go
// AnswerSheet 答卷聚合根（伪代码）
type AnswerSheet struct {
    id               ID
    questionnaireRef QuestionnaireRef  // 问卷引用（快照）
    fillerRef        FillerRef         // 填写人引用
    answers          []Answer          // 答案列表
    status           Status            // 草稿/已提交
    filledAt         time.Time
    createdAt        time.Time
    updatedAt        time.Time
}

func NewAnswerSheet(
    questionnaireRef QuestionnaireRef,
    fillerRef FillerRef,
    answers []Answer,
    filledAt time.Time,
) (*AnswerSheet, error)
```

> **查看完整实现**：[internal/apiserver/domain/survey/answersheet/answersheet.go](../../internal/apiserver/domain/survey/answersheet/answersheet.go)

### 4.2 值对象：QuestionnaireRef 和 FillerRef

#### QuestionnaireRef（问卷引用）

```go
// QuestionnaireRef 问卷引用（伪代码）
type QuestionnaireRef struct {
    Code    string  // 问卷编码
    Version string  // 问卷版本
    Title   string  // 问卷标题（冗余，便于展示）
}
```

**为什么使用引用而非 ID？**

* ✅ **快照模式**：记录答卷创建时的问卷信息
* ✅ **版本追溯**：即使问卷更新，也能知道答卷对应的版本
* ✅ **展示友好**：包含标题，无需再次查询问卷

#### FillerRef（填写人引用）

```go
// FillerRef 填写人引用（伪代码）
type FillerRef struct {
    ID   int64      // 填写人ID
    Type FillerType // 本人/代填人
}

const (
    FillerTypeSelf  = "self"   // 本人填写
    FillerTypeProxy = "proxy"  // 代填人填写
)
```

> **定义位置**：actor 子域

### 4.3 聚合根方法摘要

```go
// 访问方法（伪代码）
func (a *AnswerSheet) ID() ID
func (a *AnswerSheet) GetQuestionnaireRef() QuestionnaireRef
func (a *AnswerSheet) GetFillerRef() FillerRef
func (a *AnswerSheet) GetAnswers() []Answer
func (a *AnswerSheet) GetStatus() Status

// 状态判断
func (a *AnswerSheet) IsDraft() bool
func (a *AnswerSheet) IsSubmitted() bool

// 业务方法
func (a *AnswerSheet) MarkAsSubmitted()                           // 标记已提交
func (a *AnswerSheet) FindAnswer(questionCode Code) *Answer       // 查找答案
func (a *AnswerSheet) AddAnswer(answer Answer) error              // 添加答案
func (a *AnswerSheet) UpdateAnswerScore(code Code, score float64) // 更新分数（Scale子域调用）
func (a *AnswerSheet) IsFilledBy(fillerRef FillerRef) bool        // 检查填写人
```

> **查看完整实现**：[internal/apiserver/domain/survey/answersheet/answersheet.go](../../internal/apiserver/domain/survey/answersheet/answersheet.go)

**设计要点**：

* ✅ AnswerSheet 的方法以**业务操作**为主
* ✅ 不需要领域服务（逻辑简单）
* ✅ 公共方法为主（应用服务可直接调用）

---

## 5. AnswerValueAdapter 适配器

### 5.1 适配器的作用

**问题**：

* Validation 子域定义了 `ValidatableValue` 接口
* AnswerSheet 聚合定义了 `AnswerValue` 接口
* 两个接口不兼容，但需要对 AnswerValue 进行校验

**解决方案**：使用**适配器模式**

```text
┌──────────────────────────────────────────────────┐
│          Validation 子域                          │
│                                                  │
│  ValidatableValue 接口                           │
│  ├── IsEmpty() bool                              │
│  ├── AsString() string                           │
│  ├── AsNumber() (float64, error)                │
│  └── AsArray() []string                          │
└────────────────┬─────────────────────────────────┘
                 │
                 │ 需要校验
                 │
                 ↓
┌──────────────────────────────────────────────────┐
│        AnswerValueAdapter (适配器)               │
│                                                  │
│  - 实现 ValidatableValue 接口                    │
│  - 持有 AnswerValue 引用                         │
│  - 将 AnswerValue 的数据转换为校验器需要的格式   │
└────────────────┬─────────────────────────────────┘
                 │
                 │ 适配
                 │
                 ↓
┌──────────────────────────────────────────────────┐
│          AnswerSheet 聚合                         │
│                                                  │
│  AnswerValue 接口                                │
│  └── Raw() any                                   │
└──────────────────────────────────────────────────┘
```

### 5.2 适配器实现

```go
// AnswerValueAdapter 答案值适配器
// 将 AnswerValue 适配为 ValidatableValue，使其可被 validation 子域校验
type AnswerValueAdapter struct {
    answerValue AnswerValue
}

// NewAnswerValueAdapter 创建答案值适配器
func NewAnswerValueAdapter(value AnswerValue) validation.ValidatableValue {
    return &AnswerValueAdapter{answerValue: value}
}

// IsEmpty 实现 ValidatableValue 接口
func (a *AnswerValueAdapter) IsEmpty() bool {
    if a.answerValue == nil {
        return true
    }
    
    raw := a.answerValue.Raw()
    if raw == nil {
        return true
    }
    
    // 根据类型判断是否为空
    switch v := raw.(type) {
    case string:
        return v == ""
    case []string:
        return len(v) == 0
    case float64:
        return false  // 数字 0 也是有效值
    case int:
        return false
    default:
        return true
    }
}

// AsString 实现 ValidatableValue 接口
func (a *AnswerValueAdapter) AsString() string {
    if a.answerValue == nil {
        return ""
    }
    
    raw := a.answerValue.Raw()
    switch v := raw.(type) {
    case string:
        return v
    case float64:
        return fmt.Sprintf("%v", v)
    case int:
        return fmt.Sprintf("%d", v)
    case []string:
        // 多选值转为逗号分隔的字符串
        return strings.Join(v, ",")
    default:
        return fmt.Sprintf("%v", v)
    }
}

// AsNumber 实现 ValidatableValue 接口
func (a *AnswerValueAdapter) AsNumber() (float64, error) {
    if a.answerValue == nil {
        return 0, errors.New("answer value is nil")
    }
    
    raw := a.answerValue.Raw()
    switch v := raw.(type) {
    case float64:
        return v, nil
    case int:
        return float64(v), nil
    case int64:
        return float64(v), nil
    case string:
        return strconv.ParseFloat(v, 64)
    default:
        return 0, fmt.Errorf("cannot convert %T to number", raw)
    }
}

// AsArray 实现 ValidatableValue 接口
func (a *AnswerValueAdapter) AsArray() []string {
    if a.answerValue == nil {
        return []string{}
    }
    
    raw := a.answerValue.Raw()
    switch v := raw.(type) {
    case []string:
        return v
    case string:
        // 单个字符串转为单元素数组
        if v == "" {
            return []string{}
        }
        return []string{v}
    default:
        return []string{}
    }
}
```

### 5.3 适配器使用示例

```go
// 在应用服务中使用
func (s *SubmissionService) Submit(ctx context.Context, dto SubmitAnswerSheetDTO) error {
    // ... 创建答案
    
    // 通过适配器进行校验
    for _, answer := range answers {
        // 1. 将 AnswerValue 适配为 ValidatableValue
        validatableValue := answersheet.NewAnswerValueAdapter(answer.Value())
        
        // 2. 使用 validator 进行校验
        validationResult := s.validator.ValidateValue(
            validatableValue, 
            question.GetValidationRules(),
        )
        
        // 3. 处理校验结果
        if !validationResult.IsValid() {
            return errors.New("validation failed")
        }
    }
    
    // ...
}
```

**适配器模式的价值**：

* ✅ **解耦两个子域**：answersheet 和 validation 互不依赖
* ✅ **单一职责**：适配器只负责转换
* ✅ **易于测试**：可以单独测试适配器
* ✅ **灵活扩展**：新增答案类型只需更新适配器

---

## 6. 使用示例

### 6.1 提交答卷完整流程

```go
// 1. 准备数据
questionnaireRef := answersheet.NewQuestionnaireRef("PHQ-9", "1.0.1", "PHQ-9 抑郁症筛查")
fillerRef := actor.NewFillerRef(int64(userID), actor.FillerTypeSelf)

// 2. 创建答案列表
answers := make([]answersheet.Answer, 0)

// 创建单选答案
value1, _ := answersheet.CreateAnswerValueFromRaw(
    questionnaire.TypeRadio, 
    "2",  // 选择了选项 2
)
answer1, _ := answersheet.NewAnswer(
    meta.NewCode("Q1"),
    questionnaire.TypeRadio,
    value1,
    0,  // 初始分数为 0
)
answers = append(answers, answer1)

// 创建文本答案
value2, _ := answersheet.CreateAnswerValueFromRaw(
    questionnaire.TypeText, 
    "张三",
)
answer2, _ := answersheet.NewAnswer(
    meta.NewCode("Q2"),
    questionnaire.TypeText,
    value2,
    0,
)
answers = append(answers, answer2)

// 创建多选答案
value3, _ := answersheet.CreateAnswerValueFromRaw(
    questionnaire.TypeCheckbox, 
    []string{"A", "C"},  // 选择了 A 和 C
)
answer3, _ := answersheet.NewAnswer(
    meta.NewCode("Q3"),
    questionnaire.TypeCheckbox,
    value3,
    0,
)
answers = append(answers, answer3)

// 3. 创建答卷
sheet, _ := answersheet.NewAnswerSheet(
    questionnaireRef,
    fillerRef,
    answers,
    time.Now(),
)

// 4. 标记为已提交
sheet.MarkAsSubmitted()

// 5. 持久化（通过 Repository）
err := repository.Create(ctx, sheet)
```

### 6.2 查找和更新答案

```go
// 查找特定问题的答案
answer := sheet.FindAnswer(meta.NewCode("Q1"))
if answer != nil {
    fmt.Printf("Q1 的答案: %v, 分数: %.2f\n", 
        answer.Value().Raw(), 
        answer.Score())
}

// 更新答案分数（由 Scale 子域计算后调用）
err := sheet.UpdateAnswerScore(meta.NewCode("Q1"), 2.0)
```

### 6.3 校验答案

```go
// 在提交前校验答案
validator := validation.NewDefaultValidator()

for _, answer := range sheet.GetAnswers() {
    // 获取问题的校验规则
    question := findQuestion(answer.QuestionCode())
    
    // 通过适配器校验
    validatableValue := answersheet.NewAnswerValueAdapter(answer.Value())
    result := validator.ValidateValue(
        validatableValue, 
        question.GetValidationRules(),
    )
    
    if !result.IsValid() {
        // 处理校验错误
        for _, err := range result.GetErrors() {
            fmt.Printf("校验错误: %s\n", err.GetMessage())
        }
    }
}
```

---

## 7. 设计模式总结

AnswerSheet 聚合使用的设计模式：

| 模式 | 应用位置 | 价值 |
| ----- | --------- | ------ |
| **简单工厂模式** | CreateAnswerValueFromRaw | 根据题型创建答案值 |
| **适配器模式** | AnswerValueAdapter | 连接 answersheet 和 validation |
| **值对象模式** | QuestionnaireRef、FillerRef | 快照、引用解耦 |
| **不可变模式** | WithScore 方法 | 保证数据一致性 |

### 7.1 与 Questionnaire 的设计对比

| 设计方面 | Questionnaire | AnswerSheet |
| --------- | --------------- | ------------- |
| **复杂度** | 高 | 低 |
| **创建模式** | 注册器 + 工厂 | 简单工厂 |
| **领域服务** | 5 个 | 0 个 |
| **扩展方式** | 注册新题型 | 直接修改 switch |
| **设计原则** | 高度抽象 | 简单直接 |

**关键启示**：
> 不是所有聚合都需要复杂的设计模式。根据实际需求选择合适的设计：
>
> * **复杂场景**：使用注册器、策略等模式
> * **简单场景**：使用简单工厂、直接实现

---

## 8. 扩展示例：新增日期答案

**场景**：支持日期题型的答案

***步骤 1：定义 DateValue***

```go
// DateValue 日期答案值
type DateValue struct {
    date time.Time
}

// NewDateValue 创建日期答案值
func NewDateValue(dateStr string) (AnswerValue, error) {
    date, err := time.Parse("2006-01-02", dateStr)
    if err != nil {
        return nil, err
    }
    return DateValue{date: date}, nil
}

// Raw 返回原始值
func (v DateValue) Raw() any {
    return v.date.Format("2006-01-02")
}
```

***步骤 2：更新工厂方法***

```go
func CreateAnswerValueFromRaw(qType questionnaire.QuestionType, raw any) (AnswerValue, error) {
    switch qType {
    // ... 现有类型
    
    case questionnaire.TypeDate:
        if dateStr, ok := raw.(string); ok {
            return NewDateValue(dateStr)
        }
        return nil, fmt.Errorf("date question expects string value")
    
    default:
        return nil, fmt.Errorf("unknown question type: %s", qType.Value())
    }
}
```

***步骤 3：更新适配器***

```go
// 在 AsString 方法中处理日期
func (a *AnswerValueAdapter) AsString() string {
    // ... 现有逻辑
    
    // 处理日期类型
    if t, ok := raw.(time.Time); ok {
        return t.Format("2006-01-02")
    }
    
    return fmt.Sprintf("%v", v)
}
```

✅ **完成！** 只需修改 3 处，无需改动核心架构。

---

## 9. 下一步阅读

* **[11-04-04 Validation 子域设计](./11-04-04-Validation子域设计.md)** - 策略模式实现校验
* **[11-04-05 应用服务层设计](./11-04-05-应用服务层设计.md)** - 如何使用 AnswerSheet
* **[11-04-06 设计模式应用总结](./11-04-06-设计模式应用总结.md)** - 模式对比和选择

---

> **相关文档**：
>
> * [Survey 子域设计系列](./11-04-Survey子域设计系列.md) - 系列文档索引
> * [11-04-01 Survey 子域架构总览](./11-04-01-Survey子域架构总览.md) - 架构设计
> * [11-04-02 Questionnaire 聚合设计](./11-04-02-Questionnaire聚合设计.md) - 题型设计
