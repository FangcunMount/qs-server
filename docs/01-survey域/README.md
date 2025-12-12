# 11-04 Survey 子域设计

> **版本**：V2.2  
> **最后更新**：2025-11-26  
> **状态**：✅ 已实现并验证  
> **范围**：问卷&量表 BC 中的 survey 子域  
> **目标**：阐述问卷子域的核心职责、可扩展题型设计（注册器+参数容器+工厂）、Questionnaire 聚合领域服务、版本管理策略、可扩展答案设计、策略模式校验规则
>
> **文档说明**：本文档已根据实际代码实现进行更新，所有示例代码均与实际实现保持一致

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
* **被依赖于** assessment 子域：assessment 引用 QuestionnaireCode 和 AnswerSheetID

**依赖方向**：

```text
survey (独立)
   ↓
 scale (依赖 survey 的只读视图)
   ↓
assessment (依赖 survey + scale)
```

---

## 文档列表

### 📋 [11-04-01 Survey 子域架构总览](./11-04-01-Survey子域架构总览.md)

**核心内容**：

* Survey 子域的定位与职责边界
* 核心聚合（Questionnaire、AnswerSheet、Validation）
* 与其他子域的依赖关系
* 整体架构分层（领域层、应用层、基础设施层）
* 目录结构设计

**关键要点**：

* Survey 子域是独立的、无外部依赖的纯粹领域
* 三大核心：问卷模板、答卷收集、输入校验
* 清晰的分层架构和职责划分

---

### 🎯 [11-04-02 Questionnaire 聚合设计](./11-04-02-Questionnaire聚合设计.md)

**核心内容**：

* Questionnaire 聚合根设计
* Question 题型的可扩展设计（注册器+参数容器+工厂模式）
* 五大领域服务（Lifecycle、BaseInfo、QuestionManager、Versioning、Validator）
* 语义化版本管理策略
* Option 值对象设计

**关键要点**：

* 注册器模式实现题型扩展
* 参数容器模式分离参数收集与对象创建
* 函数式选项模式提供灵活配置
* 领域服务避免聚合根臃肿

---

### 📝 [11-04-03 AnswerSheet 聚合设计](./11-04-03-AnswerSheet聚合设计.md)

**核心内容**：

* AnswerSheet 聚合根设计
* Answer 实体设计
* AnswerValue 接口与具体实现（StringValue、NumberValue、OptionValue、OptionsValue）
* 工厂方法模式创建答案值
* 答案与问题的映射关系

**关键要点**：

* 简化的答案设计（无注册器，直接工厂方法）
* 根据题型自动创建对应答案值
* 不可变性保证（WithScore 模式）

---

### ✅ [11-04-04 Validation 子域设计](./11-04-04-Validation子域设计.md)

**核心内容**：

* ValidationRule 值对象
* 策略模式实现的校验器
* 8 种校验策略实现（Required, MinLength, MaxLength, MinValue, MaxValue, MinSelections, MaxSelections, Pattern）
* ValidatableValue 接口
* AnswerValueAdapter 适配器模式

**关键要点**：

* 策略模式 + 注册器模式实现可扩展校验
* 适配器模式连接 answersheet 与 validation
* 自动策略注册与选择

---

### 🏗️ [11-04-05 应用服务层设计](./11-04-05-应用服务层设计.md)

**核心内容**：

* Questionnaire 应用服务（LifecycleService、ContentService、QueryService）
* AnswerSheet 应用服务（SubmissionService、ManagementService、ScoringService）
* DTO 设计与转换
* 事务边界管理
* 校验流程集成

**关键要点**：

* 应用服务协调领域服务
* DTO 作为防腐层
* 明确的事务边界

---

### 🔧 [11-04-06 设计模式应用总结](./11-04-06-设计模式应用总结.md)

**核心内容**：

* 注册器模式（QuestionFactory、ValidationStrategy）
* 工厂模式（NewQuestion、CreateAnswerValueFromRaw）
* 参数容器模式（QuestionParams）
* 函数式选项模式（WithCode、WithStem）
* 策略模式（ValidationStrategy）
* 适配器模式（AnswerValueAdapter）
* 领域服务模式（Lifecycle、Versioning）

**关键要点**：

* 每种模式的应用场景
* 模式之间的协作关系
* 扩展性保证

---

### 📦 [11-04-07 扩展指南](./11-04-07-扩展指南.md)

**核心内容**：

* 如何新增题型
* 如何新增答案类型
* 如何新增校验策略
* 扩展示例（日期题型、日期答案、日期范围校验）

**关键要点**：

* 扩展无需修改核心代码
* 注册即可生效
* 完整的扩展示例

---

## 阅读建议

### 📖 全面学习路径

1. 从 **11-04-01** 开始，理解整体架构
2. 按顺序阅读 **11-04-02** 到 **11-04-05**，深入理解各个聚合
3. 阅读 **11-04-06** 总结设计模式
4. 参考 **11-04-07** 进行扩展开发

### 🎯 快速上手路径

1. **11-04-01** - 了解整体架构
2. **11-04-02** - 理解题型设计（最复杂）
3. **11-04-05** - 查看应用服务使用示例
4. **11-04-07** - 参考扩展指南

### 🔍 问题导向路径

* **如何新增题型？** → 11-04-02 + 11-04-07
* **如何实现校验？** → 11-04-04
* **如何管理版本？** → 11-04-02（Versioning 部分）
* **如何组织应用服务？** → 11-04-05

---

## 代码位置

```text
internal/apiserver/domain/survey/
├── questionnaire/              # Questionnaire 聚合（11-04-02）
│   ├── questionnaire.go        # 聚合根
│   ├── question.go             # 题型接口与实现
│   ├── factory.go              # 注册器 + 工厂
│   ├── option.go               # 选项值对象
│   ├── lifecycle.go            # 生命周期服务
│   ├── baseinfo.go             # 基础信息服务
│   ├── question_manager.go     # 问题管理服务
│   ├── versioning.go           # 版本管理服务
│   └── validator.go            # 业务规则验证服务
│
├── answersheet/                # AnswerSheet 聚合（11-04-03）
│   ├── answersheet.go          # 聚合根
│   ├── answer.go               # Answer 实体 + AnswerValue 实现
│   └── validation_adapter.go   # 适配器
│
└── validation/                 # Validation 子域（11-04-04）
    ├── validator.go            # 验证器
    ├── strategy.go             # 策略接口 + 注册器
    ├── rule.go                 # 规则值对象
    ├── required.go             # 必填策略
    ├── min_length.go           # 最小长度策略
    ├── max_length.go           # 最大长度策略
    ├── min_value.go            # 最小值策略
    ├── max_value.go            # 最大值策略
    ├── selections.go           # 选项数量策略
    └── pattern.go              # 正则表达式策略

internal/apiserver/application/survey/  # 应用服务（11-04-05）
├── questionnaire/
│   ├── lifecycle_service.go
│   ├── content_service.go
│   └── query_service.go
└── answersheet/
    ├── submission_service.go
    ├── management_service.go
    └── scoring_service.go
```

---

## 文档特色

✅ **与代码完全一致**：所有示例代码都来自实际实现  
✅ **金字塔结构**：从宏观到微观，层层递进  
✅ **设计模式导向**：重点阐述模式应用与协作  
✅ **扩展性保证**：详细的扩展指南和示例  
✅ **实战导向**：包含完整的使用示例  

---

> **相关文档**：
>
> * 《11-01-问卷&量表BC领域模型总览-v2.md》
> * 《11-02-qs-apiserver领域层代码结构设计-v2.md》
> * 《11-03-Testee和Staff用户模型设计-v2.md》
> * 《11-05-Scale子域设计-v2.md》
