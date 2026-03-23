# 11-04-01 Survey 子域架构总览

> **版本**：V3.0  
> **最后更新**：2025-11-26  
> **状态**：✅ 已实现并验证  
> **所属系列**：[Survey 子域设计系列](./11-04-Survey子域设计系列.md)

---

## 1. Survey 子域的定位与职责

### 1.1 子域在业务上下文中的位置

Survey 子域是**问卷&量表 BC（Bounded Context）**的核心子域之一，专注于问卷的**结构定义**和**答案收集**。

**业务价值**：

* 📋 提供灵活可扩展的问卷模板管理
* ✍️ 支持多种题型的答案收集
* ✅ 确保答案数据的完整性和合法性

**在业务流程中的位置**：

```text
测评流程：
1. [Survey] 定义问卷结构（题目、选项、校验规则）
2. [Survey] 收集用户答案
3. [Survey] 校验答案合法性
4. [Scale] 根据问卷和答案进行计分  ← Survey 为 Scale 提供数据
5. [Scale] 生成解读报告
6. [Assessment] 管理整个测评过程    ← Survey 被 Assessment 引用
```

### 1.2 子域职责边界

#### ✅ Survey 关心什么

| 职责领域 | 具体内容 | 实现方式 |
| --------- | --------- | --------- |
| **问卷结构** | 题目、选项、题型、顺序 | Questionnaire 聚合 |
| **答案收集** | 记录答案、管理答卷状态 | AnswerSheet 聚合 |
| **输入校验** | 必填、长度、范围、格式 | Validation 子域 |
| **版本管理** | 问卷版本演化、草稿发布 | Versioning 领域服务 |

#### ❌ Survey 不关心什么

| 非职责领域 | 说明 | 负责子域 |
| ----------- | ------ | --------- |
| **分数含义** | 分数代表什么心理状态 | Scale 子域 |
| **计分逻辑** | 如何根据答案计算分数 | Scale 子域 |
| **解读规则** | 分数对应的解读文本 | Scale 子域 |
| **测评管理** | 测评计划、测评记录 | Assessment 子域 |
| **用户管理** | 患者信息、医生信息 | Actor 子域 |

**关键理解**：

> Survey 是一个**纯粹的数据结构领域**，它只定义"怎么问"和"怎么答"，不关心"答案意味着什么"。这种职责分离使得：
>
> 1. Survey 可以独立演化，不受业务规则变更影响
> 2. 同一份问卷可以被不同的量表使用（一对多关系）
> 3. 系统具有更好的可测试性和可维护性

### 1.3 与其他子域的依赖关系

```text
┌─────────────────────────────────────────────┐
│            Assessment 子域                   │
│     (测评计划、测评记录、测评管理)            │
│                                             │
│   引用: QuestionnaireCode, AnswerSheetID    │
└──────────────────┬──────────────────────────┘
                   │ 依赖 (读取)
                   ↓
┌─────────────────────────────────────────────┐
│              Scale 子域                      │
│      (计分规则、解读规则、分数计算)           │
│                                             │
│   读取: Question, Answer 的只读视图          │
└──────────────────┬──────────────────────────┘
                   │ 依赖 (读取)
                   ↓
┌─────────────────────────────────────────────┐
│            Survey 子域 (本文档)               │
│     (问卷结构、答案收集、输入校验)            │
│                                             │
│   ✨ 独立存在，无外部依赖                    │
└─────────────────────────────────────────────┘
```

**依赖方向特点**：

* ✅ Survey **不依赖**任何其他子域 → 最底层、最稳定
* ✅ Scale **单向依赖** Survey 的只读视图 → 读取 Question 和 Answer
* ✅ Assessment **单向依赖** Survey 和 Scale → 引用 ID 和结果

---

## 2. 核心聚合与领域模型

### 2.1 聚合划分

Survey 子域包含 **3 个核心组件**：

#### 1️⃣ Questionnaire 聚合（问卷模板）

**聚合根**：`Questionnaire`

**核心实体/值对象**：

* `Question` - 题目（接口）
* `Option` - 选项（值对象）
* `ValidationRule` - 校验规则（值对象）
* `Version` - 版本（值对象）

**职责**：

* 管理问卷的生命周期（草稿 → 发布 → 归档）
* 管理题目列表（增删改查）
* 维护问卷版本
* 确保问卷发布前的完整性

#### 2️⃣ AnswerSheet 聚合（答卷实例）

**聚合根**：`AnswerSheet`

**核心实体/值对象**：

* `Answer` - 答案（实体）
* `AnswerValue` - 答案值（接口）
* `StringValue/NumberValue/OptionValue/OptionsValue` - 具体答案值

**职责**：

* 收集用户答案
* 管理答卷状态（草稿 → 已提交）
* 关联问卷和填写人
* 提供答案查询

#### 3️⃣ Validation 子域（校验规则）

**核心组件**：

* `ValidationRule` - 校验规则（值对象）
* `ValidationStrategy` - 校验策略（接口）
* `Validator` - 验证器（领域服务）
* `ValidatableValue` - 可校验值（接口）

**职责**：

* 定义校验规则类型
* 实现各种校验策略
* 执行答案校验
* 返回校验结果

### 2.2 领域模型概览

```text
┌────────────────────────────────────────────────────────────┐
│                    Questionnaire 聚合                       │
│                                                             │
│  ┌──────────────────┐     ┌─────────────────────────┐     │
│  │  Questionnaire   │────>│   Question (接口)        │     │
│  │   (聚合根)       │     │  - RadioQuestion        │     │
│  │                  │     │  - CheckboxQuestion     │     │
│  │  - code          │     │  - TextQuestion         │     │
│  │  - title         │     │  - NumberQuestion       │     │
│  │  - version       │     │  - SectionQuestion      │     │
│  │  - status        │     │  - TextareaQuestion     │     │
│  │  - questions[]   │     └─────────────────────────┘     │
│  └──────────────────┘              │                       │
│           │                        │                       │
│           │                        v                       │
│           │              ┌──────────────────┐             │
│           │              │     Option       │             │
│           │              │   (值对象)       │             │
│           │              │  - code          │             │
│           │              │  - content       │             │
│           │              │  - score         │             │
│           │              └──────────────────┘             │
│           │                        │                       │
│           v                        v                       │
│  ┌──────────────────┐   ┌──────────────────┐             │
│  │     Version      │   │ ValidationRule   │             │
│  │   (值对象)       │   │   (值对象)       │             │
│  │  - 0.0.1         │   │  - ruleType      │             │
│  │  - 1.0.1         │   │  - targetValue   │             │
│  └──────────────────┘   └──────────────────┘             │
└────────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────────┐
│                    AnswerSheet 聚合                         │
│                                                             │
│  ┌──────────────────┐     ┌─────────────────────────┐     │
│  │   AnswerSheet    │────>│        Answer           │     │
│  │    (聚合根)      │     │       (实体)            │     │
│  │                  │     │                         │     │
│  │  - id            │     │  - questionCode         │     │
│  │  - questionnaireRef    │  - questionType         │     │
│  │  - fillerRef     │     │  - value (AnswerValue)  │     │
│  │  - answers[]     │     │  - score                │     │
│  │  - status        │     └─────────────────────────┘     │
│  └──────────────────┘              │                       │
│                                    v                       │
│                          ┌──────────────────┐             │
│                          │  AnswerValue     │             │
│                          │    (接口)        │             │
│                          │                  │             │
│                          │  - StringValue   │             │
│                          │  - NumberValue   │             │
│                          │  - OptionValue   │             │
│                          │  - OptionsValue  │             │
│                          └──────────────────┘             │
└────────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────────┐
│                    Validation 子域                          │
│                                                             │
│  ┌──────────────────┐     ┌─────────────────────────┐     │
│  │    Validator     │────>│ ValidationStrategy      │     │
│  │  (领域服务)      │     │     (接口)              │     │
│  │                  │     │                         │     │
│  │  - ValidateValue │     │  - RequiredStrategy     │     │
│  └──────────────────┘     │  - MinLengthStrategy    │     │
│           │               │  - MaxLengthStrategy    │     │
│           │               │  - MinValueStrategy     │     │
│           │               │  - MaxValueStrategy     │     │
│           v               │  - MinSelectionsStrategy│     │
│  ┌──────────────────┐     │  - MaxSelectionsStrategy│     │
│  │ ValidatableValue │     │  - PatternStrategy      │     │
│  │    (接口)        │     └─────────────────────────┘     │
│  │                  │                                      │
│  │  - IsEmpty()     │                                      │
│  │  - AsString()    │                                      │
│  │  - AsNumber()    │                                      │
│  │  - AsArray()     │                                      │
│  └──────────────────┘                                      │
└────────────────────────────────────────────────────────────┘
```

---

## 3. 架构分层设计

### 3.1 整体分层架构

Survey 子域采用经典的 **DDD 分层架构**：

```text
┌─────────────────────────────────────────────────────────┐
│                    Interface 层                          │
│              (gRPC Service, REST Handler)               │
│                                                         │
│  - actor_service.go    (C 端接口)                       │
│  - questionnaire_handler.go  (管理端接口)               │
│  - answersheet_handler.go    (提交答卷接口)             │
└────────────────────┬────────────────────────────────────┘
                     │ 调用
                     ↓
┌─────────────────────────────────────────────────────────┐
│                  Application 层                          │
│                (应用服务、DTO、编排)                      │
│                                                         │
│  questionnaire/                                         │
│    - lifecycle_service.go    (发布、归档)               │
│    - content_service.go      (问卷内容管理)             │
│    - query_service.go        (问卷查询)                 │
│                                                         │
│  answersheet/                                           │
│    - submission_service.go   (提交答卷)                 │
│    - management_service.go   (答卷管理)                 │
│    - scoring_service.go      (答卷评分)                 │
└────────────────────┬────────────────────────────────────┘
                     │ 调用
                     ↓
┌─────────────────────────────────────────────────────────┐
│                    Domain 层                             │
│              (聚合、实体、值对象、领域服务)                │
│                                                         │
│  survey/questionnaire/                                  │
│    ┌─────────────────────────────────────┐             │
│    │  聚合根: questionnaire.go           │             │
│    │  实体: question.go                  │             │
│    │  值对象: option.go, types.go        │             │
│    │  工厂: factory.go                   │             │
│    └─────────────────────────────────────┘             │
│    ┌─────────────────────────────────────┐             │
│    │  领域服务:                          │             │
│    │  - lifecycle.go    (生命周期)       │             │
│    │  - baseinfo.go     (基础信息)       │             │
│    │  - question_manager.go (问题管理)   │             │
│    │  - versioning.go   (版本管理)       │             │
│    │  - validator.go    (业务规则验证)   │             │
│    └─────────────────────────────────────┘             │
│                                                         │
│  survey/answersheet/                                    │
│    ┌─────────────────────────────────────┐             │
│    │  聚合根: answersheet.go             │             │
│    │  实体: answer.go                    │             │
│    │  值对象: answer values (inline)     │             │
│    │  适配器: validation_adapter.go      │             │
│    └─────────────────────────────────────┘             │
│                                                         │
│  survey/validation/                                     │
│    ┌─────────────────────────────────────┐             │
│    │  领域服务: validator.go             │             │
│    │  策略接口: strategy.go              │             │
│    │  值对象: rule.go                    │             │
│    │  策略实现: required.go, min_*.go... │             │
│    └─────────────────────────────────────┘             │
└────────────────────┬────────────────────────────────────┘
                     │ 持久化
                     ↓
┌─────────────────────────────────────────────────────────┐
│                Infrastructure 层                         │
│            (Repository 实现、数据库访问)                  │
│                                                         │
│  persistence/questionnaire/                             │
│    - questionnaire_repo.go   (MongoDB)                  │
│                                                         │
│  persistence/answersheet/                               │
│    - answersheet_repo.go     (MongoDB)                  │
└─────────────────────────────────────────────────────────┘
```

### 3.2 各层职责详解

#### 📍 Domain 层（领域层）

**职责**：

* 封装核心业务逻辑和业务规则
* 定义领域模型（聚合、实体、值对象）
* 提供领域服务处理复杂业务逻辑
* 保持领域纯粹性，不依赖外部技术

**关键原则**：

* ✅ 富领域模型（行为 + 数据）
* ✅ 不依赖应用层和基础设施层
* ✅ 使用领域语言命名
* ✅ 聚合内部保持一致性

**示例组件**：

```go
// 聚合根
type Questionnaire struct {
    id       meta.ID
    code     meta.Code
    title    string
    version  Version
    status   Status
    questions []Question
}

// 领域服务
type Versioning struct{}
func (Versioning) IncrementMajorVersion(q *Questionnaire) error
```

#### 📍 Application 层（应用层）

**职责**：

* 编排领域对象完成用例
* 定义应用服务接口
* 处理 DTO 转换（防腐层）
* 管理事务边界
* 协调多个聚合

**关键原则**：

* ✅ 薄应用层（协调而非实现业务逻辑）
* ✅ 事务在应用层开启和提交
* ✅ DTO 不污染领域层
* ✅ 一个应用服务对应一组相关用例

**示例组件**：

```go
// 应用服务
type LifecycleService struct {
    repo       Repository
    lifecycle  questionnaire.Lifecycle
    versioning questionnaire.Versioning
    validator  questionnaire.Validator
}

// 用例方法
func (s *LifecycleService) PublishQuestionnaire(ctx context.Context, code string) error {
    // 1. 加载聚合
    // 2. 调用领域服务
    // 3. 持久化
}
```

#### 📍 Interface 层（接口层）

**职责**：

* 提供外部访问接口（gRPC、REST）
* 参数验证和格式转换
* 错误处理和日志记录
* 权限校验

**关键原则**：

* ✅ 接口协议与领域解耦
* ✅ 参数校验在此层完成
* ✅ 统一的错误响应格式

#### 📍 Infrastructure 层（基础设施层）

**职责**：

* 实现 Repository 接口
* 数据库访问和 ORM 映射
* 外部服务集成
* 缓存实现

**关键原则**：

* ✅ 实现领域层定义的接口
* ✅ 技术细节不泄漏到领域层
* ✅ 可替换性（易于切换数据库）

---

## 4. 目录结构设计

### 4.1 实际目录结构

```text
internal/apiserver/
├── domain/                          # 领域层
│   └── survey/                      # Survey 子域
│       ├── questionnaire/           # Questionnaire 聚合
│       │   ├── questionnaire.go     # 聚合根
│       │   ├── types.go             # 值对象（Version、Status）
│       │   ├── question.go          # Question 接口 + 具体实现
│       │   ├── factory.go           # 题型工厂 + 注册器
│       │   ├── option.go            # Option 值对象
│       │   ├── repository.go        # Repository 接口
│       │   │
│       │   ├── lifecycle.go         # 生命周期领域服务
│       │   ├── baseinfo.go          # 基础信息领域服务
│       │   ├── question_manager.go  # 问题管理领域服务
│       │   ├── versioning.go        # 版本管理领域服务
│       │   └── validator.go         # 业务规则验证领域服务
│       │
│       ├── answersheet/             # AnswerSheet 聚合
│       │   ├── answersheet.go       # 聚合根
│       │   ├── answer.go            # Answer 实体 + AnswerValue 实现
│       │   ├── types.go             # 值对象（Status）
│       │   ├── validation_adapter.go # 适配器（连接 validation）
│       │   └── repository.go        # Repository 接口
│       │
│       └── validation/              # Validation 子域
│           ├── validator.go         # 验证器领域服务
│           ├── strategy.go          # ValidationStrategy 接口 + 注册器
│           ├── rule.go              # ValidationRule 值对象
│           ├── required.go          # RequiredStrategy 实现
│           ├── min_length.go        # MinLengthStrategy 实现
│           ├── max_length.go        # MaxLengthStrategy 实现
│           ├── min_value.go         # MinValueStrategy 实现
│           ├── max_value.go         # MaxValueStrategy 实现
│           ├── selections.go        # Min/MaxSelectionsStrategy 实现
│           └── pattern.go           # PatternStrategy 实现
│
├── application/                     # 应用层
│   └── survey/                      # Survey 应用服务
│       ├── questionnaire/           # Questionnaire 应用服务
│       │   ├── interface.go         # 应用服务接口定义
│       │   ├── lifecycle_service.go # 生命周期应用服务
│       │   ├── content_service.go   # 内容管理应用服务
│       │   ├── query_service.go     # 查询应用服务
│       │   └── dto.go               # DTO 定义
│       │
│       └── answersheet/             # AnswerSheet 应用服务
│           ├── interface.go         # 应用服务接口定义
│           ├── submission_service.go # 提交应用服务
│           ├── management_service.go # 管理应用服务
│           ├── scoring_service.go   # 评分应用服务
│           └── dto.go               # DTO 定义
│
├── interface/                       # 接口层
│   ├── grpc/
│   │   └── service/
│   │       ├── actor_service.go     # C 端 Actor 服务
│   │       ├── questionnaire.go     # 问卷 gRPC 服务
│   │       └── answersheet.go       # 答卷 gRPC 服务
│   │
│   └── restful/
│       └── handler/
│           ├── questionnaire_handler.go
│           └── answersheet_handler.go
│
└── infra/                           # 基础设施层
    └── persistence/
        ├── questionnaire/
        │   └── questionnaire_repo.go
        └── answersheet/
            └── answersheet_repo.go
```

### 4.2 目录组织原则

#### 1️⃣ 按聚合划分（而非技术层）

```text
❌ 不好的组织方式（按技术层划分）：
survey/
  ├── entities/      (所有实体混在一起)
  ├── services/      (所有服务混在一起)
  └── repositories/  (所有仓储混在一起)

✅ 好的组织方式（按聚合划分）：
survey/
  ├── questionnaire/  (Questionnaire 聚合的所有内容)
  ├── answersheet/    (AnswerSheet 聚合的所有内容)
  └── validation/     (Validation 子域的所有内容)
```

**优势**：

* 高内聚：相关代码在一起，易于理解和修改
* 清晰的边界：每个文件夹代表一个聚合
* 易于重构：移动聚合只需移动一个文件夹

#### 2️⃣ 领域服务与聚合根平级

```text
questionnaire/
  ├── questionnaire.go    # 聚合根
  ├── lifecycle.go        # 领域服务（平级）
  ├── versioning.go       # 领域服务（平级）
  └── validator.go        # 领域服务（平级）
```

**原因**：

* 领域服务不属于聚合根内部
* 领域服务可以跨多个聚合根操作
* 平级结构体现了它们的独立性

#### 3️⃣ 接口与实现分离

```text
Question (接口) → question.go (包含所有具体实现)
AnswerValue (接口) → answer.go (包含所有具体实现)
ValidationStrategy (接口) → 多个 *_strategy.go 文件
```

**灵活性**：

* 小型实现：接口与实现在同一文件（Question、AnswerValue）
* 大型实现：每个策略独立文件（ValidationStrategy）

---

## 5. 核心设计模式概览

Survey 子域大量使用设计模式来保证扩展性和可维护性：

### 5.1 注册器模式（Registry Pattern）

**应用场景**：

* ✅ 题型工厂注册（QuestionFactory）
* ✅ 校验策略注册（ValidationStrategy）

**核心思想**：

* 将"类型"到"工厂/策略"的映射关系存储在注册表中
* 通过 `init()` 函数自动注册
* 运行时根据类型查找对应的工厂/策略

**价值**：

* 新增题型/策略无需修改核心代码
* 实现了开闭原则（对扩展开放、对修改封闭）

### 5.2 工厂模式（Factory Pattern）

**应用场景**：

* ✅ Question 创建（`NewQuestion`）
* ✅ AnswerValue 创建（`CreateAnswerValueFromRaw`）

**核心思想**：

* 统一的创建入口
* 根据参数决定创建哪种具体类型
* 封装创建逻辑

### 5.3 参数容器模式（Parameter Object Pattern）

**应用场景**：

* ✅ QuestionParams（题目参数容器）

**核心思想**：

* 分离参数收集与对象创建
* QuestionParams 只负责收集和校验参数
* 工厂函数负责创建对象

**职责分离**：

```text
QuestionParams  → 收集参数、校验参数、提供 Getter
QuestionFactory → 根据参数创建对象
NewQuestion     → 协调流程
```

### 5.4 函数式选项模式（Functional Options Pattern）

**应用场景**：

* ✅ 题目创建配置（WithCode、WithStem、WithOption...）

**核心思想**：

* 使用函数闭包配置对象
* 链式调用，可读性强
* 易于扩展新选项

**示例**：

```go
question, _ := NewQuestion(
    WithCode(meta.NewCode("Q1")),
    WithStem("您的性别是？"),
    WithQuestionType(TypeRadio),
    WithOption("A", "男", 0),
    WithOption("B", "女", 0),
    WithRequired(),
)
```

### 5.5 策略模式（Strategy Pattern）

**应用场景**：

* ✅ 校验策略（ValidationStrategy）

**核心思想**：

* 定义统一的策略接口
* 每种校验规则对应一个策略实现
* 运行时动态选择策略

**优势**：

* 避免大量 if-else
* 每个策略独立实现和测试
* 易于新增策略

### 5.6 适配器模式（Adapter Pattern）

**应用场景**：

* ✅ AnswerValueAdapter（连接 answersheet 和 validation）

**核心思想**：

* 将 AnswerValue 适配为 ValidatableValue
* 解决两个不兼容接口的协作问题

### 5.7 领域服务模式（Domain Service Pattern）

**应用场景**：

* ✅ Lifecycle（生命周期管理）
* ✅ Versioning（版本管理）
* ✅ Validator（业务规则验证）
* ✅ QuestionManager（问题管理）
* ✅ BaseInfo（基础信息管理）

**核心思想**：

* 处理不属于单个聚合根的业务逻辑
* 避免聚合根过于臃肿
* 可以跨聚合操作

---

## 6. 关键技术决策

### 6.1 为什么 Question 是接口而非抽象类？

**决策**：Question 定义为接口，具体题型实现接口

**原因**：

1. Go 语言推荐面向接口编程
2. 接口更灵活，支持组合
3. 避免继承带来的耦合
4. 更容易测试（Mock）

### 6.2 为什么 AnswerValue 不使用注册器？

**决策**：AnswerValue 使用简单的工厂方法而非注册器

**原因**：

1. 答案类型数量有限（4 种）
2. 答案类型与题型一一对应
3. 简单的映射关系不需要复杂的注册器
4. 代码更直观易懂

**对比**：

```go
// Question: 使用注册器（6+ 种题型，可能继续扩展）
RegisterQuestionFactory(TypeRadio, newRadioQuestionFactory)

// AnswerValue: 直接工厂方法（4 种类型，基本固定）
func CreateAnswerValueFromRaw(qType QuestionType, raw any) (AnswerValue, error) {
    switch qType {
    case TypeRadio:
        return NewOptionValue(raw.(string)), nil
    // ...
    }
}
```

### 6.3 为什么需要 Validation 独立子域？

**决策**：将 Validation 抽取为独立子域

**原因**：

1. **复用性**：校验规则可能被其他子域使用
2. **扩展性**：新增校验策略不影响聚合
3. **单一职责**：聚合只关心业务逻辑，不关心校验细节
4. **可测试性**：校验逻辑独立测试

### 6.4 为什么使用领域服务而非聚合根方法？

**决策**：使用 5 个领域服务（Lifecycle、Versioning 等）而非全部放在聚合根

**原因**：

1. **避免聚合根臃肿**：聚合根专注核心业务逻辑
2. **职责分离**：每个服务处理一类相关操作
3. **可测试性**：服务独立测试
4. **可复用性**：服务可能被多个聚合使用

---

## 7. 下一步阅读

根据您的需求选择后续文档：

### 📚 深入理解设计

* **[11-04-02 Questionnaire 聚合设计](./11-04-02-Questionnaire聚合设计.md)** - 最复杂的聚合，包含题型设计、领域服务
* **[11-04-04 Validation 子域设计](./11-04-04-Validation子域设计.md)** - 策略模式的完整实现

### 🛠️ 实战应用

* **[11-04-05 应用服务层设计](./11-04-05-应用服务层设计.md)** - 如何使用领域对象
* **[11-04-07 扩展指南](./11-04-07-扩展指南.md)** - 如何新增题型、答案、校验策略

### 🎯 模式学习

* **[11-04-06 设计模式应用总结](./11-04-06-设计模式应用总结.md)** - 所有模式的总结和对比

---

> **相关文档**：
>
> * [Survey 子域设计系列](./11-04-Survey子域设计系列.md) - 系列文档索引
> * 《11-01-问卷&量表BC领域模型总览-v2.md》 - BC 整体设计
> * 《11-02-qs-apiserver领域层代码结构设计-v2.md》 - 代码结构设计
