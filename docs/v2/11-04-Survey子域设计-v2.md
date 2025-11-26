# 11-04 Survey 子域设计（V2）

> **版本**：V2.1  
> **最后更新**：2025-11-26  
> **范围**：问卷&量表 BC 中的 survey 子域  
> **目标**：阐述问卷子域的核心职责、可扩展题型设计（注册器+参数容器+工厂）、Questionnaire 聚合领域服务、版本管理策略、可扩展答案设计（注册器+工厂）、策略模式校验规则

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

## 2. 可扩展的题型设计（注册器+参数容器+工厂）

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
    RadioQuestion    QuestionType = "Radio"    // 单选
    CheckboxQuestion QuestionType = "Checkbox" // 多选
    TextQuestion     QuestionType = "Text"     // 文本
    TextareaQuestion QuestionType = "Textarea" // 文本域
    NumberQuestion   QuestionType = "Number"   // 数字
)
```

### 2.3 注册器设计

```go
// QuestionFactory 工厂函数签名
type QuestionFactory func(*QuestionParams) (Question, error)

// registry 注册表
var questionRegistry = make(map[QuestionType]QuestionFactory)

// RegisterQuestionFactory 注册题型工厂
func RegisterQuestionFactory(typ QuestionType, factory QuestionFactory) {
    if _, exists := questionRegistry[typ]; exists {
        panic(fmt.Sprintf("question type already registered: %s", typ))
    }
    questionRegistry[typ] = factory
}

// NewQuestion 统一创建入口
func NewQuestion(opts ...QuestionParamsOption) (Question, error) {
    // 1. 创建参数容器并收集参数
    params := NewQuestionParams(opts...)
    
    // 2. 校验参数完整性
    if err := params.Validate(); err != nil {
        return nil, err
    }
    
    // 3. 根据题型选择工厂函数
    factory, ok := questionRegistry[params.GetCore().typ]
    if !ok {
        return nil, fmt.Errorf("unknown question type: %s", params.GetCore().typ)
    }
    
    // 4. 使用工厂函数创建实例
    return factory(params)
}
```

### 2.4 QuestionParams 参数容器

**设计原则**：QuestionParams 是纯数据容器，只负责收集参数，不负责创建 Question 实例。

```go
// QuestionParams 题型参数容器
type QuestionParams struct {
    core            QuestionCore
    placeholder     string
    options         []Option
    validationRules []validation.ValidationRule
    calculationRule *calculation.CalculationRule
}

// QuestionCore 所有题型共享的核心字段
type QuestionCore struct {
    code meta.Code
    stem string
    typ  QuestionType
    tips string
}

// NewQuestionParams 创建参数容器
func NewQuestionParams(opts ...QuestionParamsOption) *QuestionParams {
    params := &QuestionParams{
        options:         make([]Option, 0),
        validationRules: make([]validation.ValidationRule, 0),
    }
    // 应用所有选项
    for _, opt := range opts {
        opt(params)
    }
    return params
}

// Validate 校验参数完整性
func (p *QuestionParams) Validate() error {
    if p.core.code.IsEmpty() {
        return errors.New("question code is required")
    }
    if p.core.stem == "" {
        return errors.New("question stem is required")
    }
    if p.core.typ == "" {
        return errors.New("question type is required")
    }
    return nil
}
```

**职责分离**：

| 组件 | 职责 | 不负责 |
|-----|-----|-------|
| **QuestionParams** | 参数收集、参数校验、提供 Getter | ❌ 不创建 Question 实例 |
| **QuestionFactory** | 根据参数创建 Question 实例 | - |
| **NewQuestion** | 协调流程：收集→校验→选择工厂→创建 | - |

#### 2.4.1 函数式选项模式

```go
// QuestionParamsOption 参数选项函数类型
type QuestionParamsOption func(*QuestionParams)

// WithCode 设置问题编码
func WithCode(code meta.Code) QuestionParamsOption {
    return func(p *QuestionParams) {
        p.core.code = code
    }
}

// WithStem 设置问题题干
func WithStem(stem string) QuestionParamsOption {
    return func(p *QuestionParams) {
        p.core.stem = stem
    }
}

// WithQuestionType 设置问题类型
func WithQuestionType(typ QuestionType) QuestionParamsOption {
    return func(p *QuestionParams) {
        p.core.typ = typ
    }
}

// WithOption 添加选项
func WithOption(code, content string, score int) QuestionParamsOption {
    return func(p *QuestionParams) {
        opt := NewOption(code, content, score)
        p.options = append(p.options, opt)
    }
}

// WithRequired 设置必填
func WithRequired() QuestionParamsOption {
    return func(p *QuestionParams) {
        rule := validation.NewValidationRule(validation.RuleTypeRequired, "")
        p.validationRules = append(p.validationRules, rule)
    }
}

// WithCalculationRule 设置计算规则
func WithCalculationRule(formulaType calculation.FormulaType) QuestionParamsOption {
    return func(p *QuestionParams) {
        rule := calculation.NewCalculationRule(formulaType)
        p.calculationRule = &rule
    }
}
```

### 2.5 具体题型实现

#### 2.5.1 RadioQuestion 单选题

```go
// RadioQuestion 单选问题
type RadioQuestion struct {
    QuestionCore                                      // 核心字段
    options         []Option                          // 选项列表
    validationRules []validation.ValidationRule       // 校验规则
    calculationRule *calculation.CalculationRule      // 计算规则
}

// 注册单选问题工厂
func init() {
    RegisterQuestionFactory(TypeRadio, newRadioQuestionFactory)
}

// newRadioQuestionFactory 单选题工厂函数
func newRadioQuestionFactory(params *QuestionParams) (Question, error) {
    // 业务规则校验
    if len(params.GetOptions()) == 0 {
        return nil, errors.New("radio question requires at least one option")
    }
    
    return &RadioQuestion{
        QuestionCore:    params.GetCore(),
        options:         params.GetOptions(),
        validationRules: params.GetValidationRules(),
        calculationRule: params.GetCalculationRule(),
    }, nil
}

// Question 接口实现
func (q *RadioQuestion) GetCode() meta.Code                     { return q.code }
func (q *RadioQuestion) GetStem() string                        { return q.stem }
func (q *RadioQuestion) GetType() QuestionType                  { return q.typ }
func (q *RadioQuestion) GetOptions() []Option                   { return q.options }
func (q *RadioQuestion) GetValidationRules() []validation.ValidationRule {
    return q.validationRules
}
func (q *RadioQuestion) GetCalculationRule() *calculation.CalculationRule {
    return q.calculationRule
}
```

**关键理解**：

> **QuestionParams 是参数容器，不是构造者**
>
> - ✅ 它收集参数
> - ✅ 它验证参数
> - ✅ 它提供 Getter
> - ❌ 它不创建对象
>
> **创建对象是 QuestionFactory 的职责！**

这种设计使得参数收集和对象创建完全解耦，便于扩展和测试。

### 2.6 使用示例

#### 2.6.1 创建单选题

```go
question, err := questionnaire.NewQuestion(
    questionnaire.WithCode(meta.NewCode("Q1")),
    questionnaire.WithStem("您的性别是？"),
    questionnaire.WithQuestionType(questionnaire.TypeRadio),
    questionnaire.WithOption("A", "男", 0),
    questionnaire.WithOption("B", "女", 0),
    questionnaire.WithRequired(),
    questionnaire.WithCalculationRule(calculation.FormulaTypeScore),
)
if err != nil {
    // 处理错误
}

// NewQuestion 内部流程：
// 1. NewQuestionParams(opts...) - 创建参数容器收集参数
// 2. params.Validate() - 校验参数完整性
// 3. questionRegistry[params.GetCore().typ] - 选择工厂函数
// 4. factory(params) - 工厂函数创建 RadioQuestion 实例
```

#### 2.6.2 创建多选题

```go
question, err := questionnaire.NewQuestion(
    questionnaire.WithCode(meta.NewCode("Q2")),
    questionnaire.WithStem("您的兴趣爱好？"),
    questionnaire.WithQuestionType(questionnaire.TypeCheckbox),
    questionnaire.WithOption("A", "运动", 1),
    questionnaire.WithOption("B", "阅读", 1),
    questionnaire.WithOption("C", "音乐", 1),
    questionnaire.WithRequired(),
    questionnaire.WithMinSelections(1),
    questionnaire.WithMaxSelections(3),
)
```

#### 2.6.3 创建文本题

```go
question, err := questionnaire.NewQuestion(
    questionnaire.WithCode(meta.NewCode("Q3")),
    questionnaire.WithStem("请输入您的姓名"),
    questionnaire.WithQuestionType(questionnaire.TypeText),
    questionnaire.WithPlaceholder("请输入真实姓名"),
    questionnaire.WithRequired(),
    questionnaire.WithMinLength(2),
    questionnaire.WithMaxLength(20),
)
```

### 2.7 扩展性保证

当需要新增题型（如日期选择器）时：

**1. 定义题型常量**

```go
// types.go
const (
    TypeDate QuestionType = "Date"  // 新增日期题型
)
```

**2. 定义题型结构**

```go
// question.go
type DateQuestion struct {
    QuestionCore
    placeholder     string
    validationRules []validation.ValidationRule
    minDate         string
    maxDate         string
}

func (q *DateQuestion) GetPlaceholder() string { return q.placeholder }
func (q *DateQuestion) GetValidationRules() []validation.ValidationRule {
    return q.validationRules
}
```

**3. 注册工厂函数**

```go
// question.go
func init() {
    RegisterQuestionFactory(TypeDate, newDateQuestionFactory)
}

func newDateQuestionFactory(params *QuestionParams) (Question, error) {
    return &DateQuestion{
        QuestionCore:    params.GetCore(),
        placeholder:     params.GetPlaceholder(),
        validationRules: params.GetValidationRules(),
        // 可以从 params 扩展字段中获取 minDate, maxDate
    }, nil
}
```

**4. 核心代码无需改动**

- ✅ QuestionParams 自动支持新题型
- ✅ NewQuestion 自动路由到新工厂
- ✅ 其他题型代码不受影响

---

## 3. Questionnaire 聚合的领域服务

Questionnaire 聚合采用**领域服务**模式来管理复杂的业务逻辑，避免聚合根过于臃肿。

### 3.1 领域服务概览

| 领域服务 | 职责 | 核心方法 |
|---------|-----|---------|
| **Lifecycle** | 生命周期管理 | Publish(), Unpublish(), Archive() |
| **BaseInfo** | 基础信息管理 | UpdateTitle(), UpdateDescription() |
| **QuestionManager** | 问题管理 | AddQuestion(), RemoveQuestion(), UpdateQuestion() |
| **Versioning** | 版本管理 | InitializeVersion(), IncrementMinorVersion(), IncrementMajorVersion() |
| **Validator** | 业务规则验证 | ValidateForPublish(), ValidateBasicInfo(), ValidateQuestion() |

### 3.2 Lifecycle - 生命周期服务

```go
// Lifecycle 生命周期服务
type Lifecycle struct {
    versioning *Versioning
    validator  *Validator
}

// NewLifecycle 创建生命周期服务
func NewLifecycle(versioning *Versioning, validator *Validator) *Lifecycle {
    return &Lifecycle{
        versioning: versioning,
        validator:  validator,
    }
}

// Publish 发布问卷
func (l *Lifecycle) Publish(q *Questionnaire) error {
    // 1. 检查状态
    if q.status == StatusPublished {
        return errors.New("questionnaire already published")
    }
    if q.status == StatusArchived {
        return errors.New("archived questionnaire cannot be published")
    }
    
    // 2. 业务规则验证
    if err := l.validator.ValidateForPublish(q); err != nil {
        return err
    }
    
    // 3. 大版本递增（发布是重大变更）
    l.versioning.IncrementMajorVersion(q)
    
    // 4. 更新状态
    q.status = StatusPublished
    return nil
}

// Unpublish 下架问卷
func (l *Lifecycle) Unpublish(q *Questionnaire) error {
    if q.status != StatusPublished {
        return errors.New("only published questionnaire can be unpublished")
    }
    q.status = StatusDraft
    return nil
}

// Archive 归档问卷
func (l *Lifecycle) Archive(q *Questionnaire) error {
    if q.status == StatusArchived {
        return errors.New("questionnaire already archived")
    }
    q.status = StatusArchived
    return nil
}
```

### 3.3 BaseInfo - 基础信息服务

```go
// BaseInfo 基础信息服务
type BaseInfo struct{}

// NewBaseInfo 创建基础信息服务
func NewBaseInfo() *BaseInfo {
    return &BaseInfo{}
}

// UpdateTitle 更新标题
func (b *BaseInfo) UpdateTitle(q *Questionnaire, title string) error {
    if title == "" {
        return errors.New("title cannot be empty")
    }
    if len(title) > 100 {
        return errors.New("title length cannot exceed 100")
    }
    q.title = title
    return nil
}

// UpdateDescription 更新描述
func (b *BaseInfo) UpdateDescription(q *Questionnaire, description string) error {
    if len(description) > 500 {
        return errors.New("description length cannot exceed 500")
    }
    q.description = description
    return nil
}
```

### 3.4 QuestionManager - 问题管理服务

```go
// QuestionManager 问题管理服务
type QuestionManager struct{}

// NewQuestionManager 创建问题管理服务
func NewQuestionManager() *QuestionManager {
    return &QuestionManager{}
}

// AddQuestion 添加问题
func (m *QuestionManager) AddQuestion(q *Questionnaire, question Question) error {
    // 检查编码唯一性
    for _, existingQ := range q.questions {
        if existingQ.GetCode().Equals(question.GetCode()) {
            return fmt.Errorf("question code already exists: %s", question.GetCode().Value())
        }
    }
    q.questions = append(q.questions, question)
    return nil
}

// RemoveQuestion 移除问题
func (m *QuestionManager) RemoveQuestion(q *Questionnaire, code meta.Code) error {
    for i, question := range q.questions {
        if question.GetCode().Equals(code) {
            q.questions = append(q.questions[:i], q.questions[i+1:]...)
            return nil
        }
    }
    return fmt.Errorf("question not found: %s", code.Value())
}

// UpdateQuestion 更新问题
func (m *QuestionManager) UpdateQuestion(q *Questionnaire, code meta.Code, newQuestion Question) error {
    for i, question := range q.questions {
        if question.GetCode().Equals(code) {
            q.questions[i] = newQuestion
            return nil
        }
    }
    return fmt.Errorf("question not found: %s", code.Value())
}
```

### 3.5 Versioning - 版本管理服务

**设计原则**：采用**语义化版本管理策略**，格式为 `x.y.z`，自动化管理，不支持手动设置。

**版本规则**：

* **默认版本**：新建问卷从 `0.0.1` 开始
* **小版本递增**（存草稿）：`0.0.1 → 0.0.2` （递增第三位）
* **大版本递增**（发布）：`0.0.5 → 1.0.1`、`1.0.3 → 2.0.1` （递增第一位，重置为 x.0.1）
* **发布后再编辑**：再次发布时继续递增大版本

```go
// Versioning 版本管理服务
type Versioning struct{}

// NewVersioning 创建版本管理服务
func NewVersioning() *Versioning {
    return &Versioning{}
}

// InitializeVersion 初始化版本
// 新建问卷时将版本设置为 0.0.1
func (v *Versioning) InitializeVersion(q *Questionnaire) {
    if q.version.IsEmpty() {
        q.version = Version("0.0.1")
    }
}

// IncrementMinorVersion 小版本递增（存草稿）
// 示例：0.0.1 → 0.0.2, 1.0.5 → 1.0.6
func (v *Versioning) IncrementMinorVersion(q *Questionnaire) {
    if q.version.IsEmpty() {
        q.version = Version("0.0.1")
        return
    }
    q.version = q.version.IncrementMinor()
}

// IncrementMajorVersion 大版本递增（发布）
// 示例：0.0.5 → 1.0.1, 1.0.3 → 2.0.1
func (v *Versioning) IncrementMajorVersion(q *Questionnaire) {
    if q.version.IsEmpty() {
        q.version = Version("1.0.1")
        return
    }
    q.version = q.version.IncrementMajor()
}
```

**Version 值对象实现**：

```go
// Version 版本值对象
type Version string

// IncrementMinor 小版本递增
func (v Version) IncrementMinor() Version {
    parts := strings.Split(strings.TrimPrefix(string(v), "v"), ".")
    
    // 处理不同格式
    switch len(parts) {
    case 1:
        return Version(parts[0] + ".0.1")
    case 2:
        return Version(parts[0] + "." + parts[1] + ".1")
    case 3:
        minor := parseNumber(parts[2])
        return Version(fmt.Sprintf("%s.%s.%d", parts[0], parts[1], minor+1))
    default:
        return v
    }
}

// IncrementMajor 大版本递增
func (v Version) IncrementMajor() Version {
    parts := strings.Split(strings.TrimPrefix(string(v), "v"), ".")
    
    major := parseNumber(parts[0])
    return Version(fmt.Sprintf("%d.0.1", major+1))
}
```

**版本管理工作流示例**：

```
创建问卷: 0.0.1
存草稿: 0.0.1 → 0.0.2
存草稿: 0.0.2 → 0.0.3
发布: 0.0.3 → 1.0.1
编辑+存草稿: 1.0.1 → 1.0.2
存草稿: 1.0.2 → 1.0.3
再次发布: 1.0.3 → 2.0.1
```

### 3.6 Validator - 业务规则验证服务

```go
// Validator 验证服务
type Validator struct{}

// NewValidator 创建验证服务
func NewValidator() *Validator {
    return &Validator{}
}

// ValidateForPublish 发布前验证
func (v *Validator) ValidateForPublish(q *Questionnaire) error {
    var errs []error
    
    // 验证基础信息
    if err := v.ValidateBasicInfo(q); err != nil {
        errs = append(errs, err)
    }
    
    // 验证版本
    if q.version.IsEmpty() || q.version.Validate() != nil {
        errs = append(errs, errors.New("invalid version"))
    }
    
    // 验证问题列表
    if err := v.ValidateQuestions(q); err != nil {
        errs = append(errs, err)
    }
    
    if len(errs) > 0 {
        return fmt.Errorf("validation failed: %v", errs)
    }
    return nil
}

// ValidateBasicInfo 验证基础信息
func (v *Validator) ValidateBasicInfo(q *Questionnaire) error {
    if q.title == "" {
        return errors.New("title cannot be empty")
    }
    if len(q.title) > 100 {
        return errors.New("title length cannot exceed 100")
    }
    return nil
}

// ValidateQuestions 验证问题列表
func (v *Validator) ValidateQuestions(q *Questionnaire) error {
    if len(q.questions) == 0 {
        return errors.New("questionnaire must have at least one question")
    }
    
    // 检查问题编码唯一性
    codeSet := make(map[string]bool)
    for _, question := range q.questions {
        code := question.GetCode().Value()
        if codeSet[code] {
            return fmt.Errorf("duplicate question code: %s", code)
        }
        codeSet[code] = true
        
        // 验证单个问题
        if err := v.ValidateQuestion(question); err != nil {
            return err
        }
    }
    return nil
}

// ValidateQuestion 验证单个问题
func (v *Validator) ValidateQuestion(q Question) error {
    if q.GetCode().IsEmpty() {
        return errors.New("question code cannot be empty")
    }
    if q.GetStem() == "" {
        return errors.New("question stem cannot be empty")
    }
    
    // 验证选项题的选项数量
    if q.GetType() == TypeRadio || q.GetType() == TypeCheckbox {
        if len(q.GetOptions()) < 2 {
            return fmt.Errorf("question %s requires at least 2 options", q.GetCode().Value())
        }
    }
    
    return nil
}
```

### 3.7 领域服务的使用示例

```go
// 创建领域服务实例
versioning := NewVersioning()
validator := NewValidator()
lifecycle := NewLifecycle(versioning, validator)
baseInfo := NewBaseInfo()
questionManager := NewQuestionManager()

// 创建问卷
questionnaire := NewQuestionnaire(...)
versioning.InitializeVersion(questionnaire) // 设置为 0.0.1

// 添加问题
q1, _ := NewQuestion(...)
questionManager.AddQuestion(questionnaire, q1)

// 存草稿（小版本递增）
versioning.IncrementMinorVersion(questionnaire) // 0.0.1 → 0.0.2

// 继续编辑
q2, _ := NewQuestion(...)
questionManager.AddQuestion(questionnaire, q2)
versioning.IncrementMinorVersion(questionnaire) // 0.0.2 → 0.0.3

// 发布（大版本递增，带验证）
err := lifecycle.Publish(questionnaire) // 0.0.3 → 1.0.1
if err != nil {
    // 处理验证错误
}

// 发布后编辑
baseInfo.UpdateTitle(questionnaire, "新标题")
versioning.IncrementMinorVersion(questionnaire) // 1.0.1 → 1.0.2

// 再次发布
lifecycle.Publish(questionnaire) // 1.0.2 → 2.0.1
```

---

## 4. 可扩展的答案设计（注册器+工厂）

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
    QuestionType question.QuestionType
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
        QuestionType: qType,
        score:        score,
        value:        answerValue,
    }, nil
}

// GetQuestionCode 获取题目编码
func (a Answer) GetQuestionCode() meta.Code { return a.questionCode }

// GetQuestionType 获取题目类型
func (a Answer) GetQuestionType() question.QuestionType { return a.QuestionType }

// GetScore 获取分数
func (a Answer) GetScore() float64 { return a.score }

// GetValue 获取答案值
func (a Answer) GetValue() AnswerValue { return a.value }

// mapQuestionTypeToValueType 题型到答案类型的映射
func mapQuestionTypeToValueType(qType question.QuestionType) AnswerValueType {
    switch qType {
    case question.RadioQuestion:
        return OptionValueType
    case question.CheckboxQuestion:
        return OptionsValueType
    case question.TextQuestion, question.TextareaQuestion:
        return StringValueType
    case question.NumberQuestion:
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
    question.RadioQuestion,
    1.0,
    "A", // 原始值
)

// 创建多选答案
answer2, _ := answer.NewAnswer(
    meta.NewCode("Q2"),
    question.CheckboxQuestion,
    2.0,
    []string{"A", "C"}, // 原始值
)

// 创建文本答案
answer3, _ := answer.NewAnswer(
    meta.NewCode("Q3"),
    question.TextQuestion,
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

## 5. 策略模式实现的校验规则

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

## 6. 总结

### 6.1 Survey 子域的核心职责

1. **题型管理**：通过注册器+参数容器+工厂模式支持可扩展的题型体系
2. **Questionnaire 领域服务**：通过 Lifecycle、BaseInfo、QuestionManager、Versioning、Validator 服务管理复杂业务逻辑
3. **版本管理**：采用语义化版本管理策略（x.y.z），小版本对应草稿，大版本对应发布
4. **答案管理**：通过注册器+工厂模式支持可扩展的答案类型体系
5. **校验规则**：通过 ValidationRule 值对象存储规则配置，由应用层执行校验策略

### 6.2 设计模式应用

* **注册器模式**：QuestionFactory、AnswerValueFactory 注册表
* **参数容器模式**：QuestionParams 收集和校验参数
* **工厂模式**：QuestionFactory、CreateAnswerValuer 统一创建入口
* **函数式选项模式**：WithCode、WithStem 等函数式选项配置参数
* **策略模式**：ValidationRule + 应用层校验器实现不同的校验策略
* **领域服务模式**：Lifecycle、Versioning、Validator 等服务管理复杂逻辑

### 6.3 与其他子域的关系

* **Survey → Scale**：提供 Question、Answer 的只读视图，Scale 读取后进行计分和解读
* **Survey ← Assessment**：Assessment 引用 QuestionnaireID 和 AnswerSheetID
* **Survey 不依赖任何子域**：保持领域纯粹性

### 6.4 扩展性保证

* **新增题型**：注册新的 QuestionFactory，无需修改核心代码
* **新增答案类型**：注册新的 AnswerValueFactory，更新题型映射
* **新增校验规则**：扩展 RuleType 枚举，在校验器中添加对应的校验方法

---

## 附录：目录结构

**实际代码组织结构**：

```text
internal/apiserver/domain/survey/
├── questionnaire/              # Questionnaire 聚合
│   ├── questionnaire.go        # 聚合根
│   ├── types.go                # 值对象（Version、Status等）
│   ├── question.go             # Question 接口 + 具体实现
│   ├── factory.go              # 注册器 + 工厂
│   ├── question_builder.go     # QuestionParams 参数容器
│   ├── option.go               # Option 值对象
│   ├── repository.go           # 仓储接口
│   ├── # --- 领域服务 ---
│   ├── lifecycle.go            # 生命周期服务
│   ├── baseinfo.go             # 基础信息服务
│   ├── question_manager.go     # 问题管理服务
│   ├── versioning.go           # 版本管理服务
│   ├── validator.go            # 业务规则验证服务
│   ├── # --- 测试文件 ---
│   ├── question_example_test.go
│   ├── versioning_test.go
│   ├── version_test.go
│   ├── validator_test.go
│   ├── # --- 文档 ---
│   ├── ARCHITECTURE.md         # 架构说明
│   └── QUESTION_README.md      # Question 设计说明
│
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
├── validation/
│   └── validation-rule.go      # ValidationRule 值对象
└── calculation/
    └── calculation-rule.go     # CalculationRule 值对象
```

**设计特点**：

1. **扩状结构**：为简化实现，所有 Question 相关代码位于 questionnaire 目录下，而非嵌套的 question 子目录
2. **领域服务集中管理**：Lifecycle、BaseInfo、QuestionManager、Versioning、Validator 五个领域服务独立文件
3. **参数容器模式**：question_builder.go 实际是 QuestionParams 参数容器，不是构造者
4. **注册器模式**：factory.go 包含 QuestionFactory 注册表和 NewQuestion 统一创建入口

---

> **相关文档**：  
>
> * 《11-01-问卷&量表BC领域模型总览-v2.md》  
> * 《11-02-qs-apiserver领域层代码结构设计-v2.md》  
> * 《11-05-Scale子域设计-v2.md》（计分与解读）
