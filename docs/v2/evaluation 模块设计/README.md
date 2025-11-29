# 11-06 Evaluation 子域设计

> **版本**：V1.0  
> **最后更新**：2025-11-29  
> **状态**：✅ 已实现并验证  
> **范围**：问卷&量表 BC 中的 evaluation 子域  
> **目标**：阐述测评评估子域的核心职责、Assessment 聚合设计、计算策略模式、解读策略模式、报告生成器模式、事件驱动架构
>
> **文档说明**：本文档已根据实际代码实现进行更新，所有示例代码均与实际实现保持一致

---

## 1. Evaluation 子域的定位与职责

### 1.1 子域边界

**evaluation 子域关注的核心问题**：

* "如何评估"：测评流程管理、状态流转
* "如何计算"：答案到分数的计算策略
* "如何解读"：分数到结论的解读规则
* "如何呈现"：测评报告的生成与导出

**evaluation 子域不关心的问题**：

* "问卷结构"：这是 survey 子域的职责
* "量表定义"：这是 scale 子域的职责
* "用户管理"：这是 actor 子域的职责

### 1.2 核心聚合

evaluation 子域包含四个核心领域概念：

1. **Assessment 聚合**：测评实例
   * 管理测评的完整生命周期
   * 协调计算、解读、报告生成流程
   * 记录测评状态和评估结果

2. **Calculation 领域服务**：计算策略
   * 多种计算策略（求和、平均、加权和、极值等）
   * 策略模式实现可扩展计算
   * 支持因子级和总分级计算

3. **Interpretation 领域服务**：解读策略
   * 阈值策略（根据分数范围解读）
   * 复合策略（多维度条件组合）
   * 策略模式实现可扩展解读

4. **Report 聚合**：测评报告
   * 结构化报告数据（维度、建议）
   * 报告导出（PDF、Word）
   * 报告版本管理

### 1.3 与其他子域的关系

* **依赖** survey 子域：读取问卷和答卷数据
* **依赖** scale 子域：读取量表定义和计算规则
* **依赖** actor 子域：关联受试者信息
* **被依赖** 于上层应用：提供测评服务

**依赖方向**：

```text
survey (问卷&答卷)
   ↓
 scale (量表&因子)
   ↓
 actor (用户信息)
   ↓
evaluation (测评评估)
```

---

## 文档列表

### 📋 [11-06-01 Evaluation 子域架构总览](./11-06-01-Evaluation子域架构总览.md)

**核心内容**：

* Evaluation 子域的定位与职责边界
* 核心聚合（Assessment、Calculation、Interpretation、Report）
* 与其他子域的依赖关系
* 整体架构分层（领域层、应用层、基础设施层）
* 目录结构设计
* 领域事件设计

**关键要点**：

* Evaluation 是业务流程的核心协调者
* 四大核心：测评管理、分数计算、结果解读、报告生成
* 清晰的分层架构和职责划分
* 事件驱动的异步处理

---

### 🎯 [11-06-02 Assessment 聚合设计](./11-06-02-Assessment聚合设计.md)

**核心内容**：

* Assessment 聚合根设计
* Score 实体设计（因子得分、总分）
* 测评状态机（Pending → Evaluating → Completed / Failed）
* AssessmentCreator 创建器（Builder 模式）
* Origin 值对象（测评来源）
* 领域事件（AssessmentCreated、EvaluationCompleted、EvaluationFailed）

**关键要点**：

* 聚合根协调整个测评流程
* 状态机保证状态转换合法性
* Builder 模式简化复杂对象创建
* 事件驱动异步处理

---

### 🧮 [11-06-03 Calculation 计算策略设计](./11-06-03-Calculation计算策略设计.md)

**核心内容**：

* CalculationStrategy 接口
* 7 种计算策略实现
  * SumStrategy - 求和
  * AverageStrategy - 平均值
  * WeightedSumStrategy - 加权和
  * MaxStrategy - 最大值
  * MinStrategy - 最小值
  * CountStrategy - 计数
  * FirstValueStrategy - 首值
* 策略注册器模式
* 计算上下文传递

**关键要点**：

* 策略模式实现算法族封装
* 注册器模式支持动态扩展
* 函数式配置参数传递
* 支持因子和总分两级计算

---

### 🔍 [11-06-04 Interpretation 解读策略设计](./11-06-04-Interpretation解读策略设计.md)

**核心内容**：

* InterpretationStrategy 接口
* ThresholdStrategy - 阈值策略
  * ScoreRange 分数区间
  * InterpretItem 解读条目
* CompositeStrategy - 复合策略
  * Condition 条件表达式
  * 多维度条件组合（AND/OR）
* 策略注册器模式
* 高危因子识别

**关键要点**：

* 策略模式实现解读规则封装
* 分数区间的连续性和互斥性校验
* 复合条件支持复杂解读逻辑
* 自动识别高危因子

---

### 📊 [11-06-05 Report 聚合设计](./11-06-05-Report聚合设计.md)

**核心内容**：

* InterpretReport 聚合根
* ReportDimension 维度实体
* Suggestion 建议值对象
* ReportBuilder 构建器（Builder 模式）
* ReportExporter 导出器（策略模式）
  * PDF 导出
  * Word 导出
* MongoDB 存储设计

**关键要点**：

* Builder 模式构建复杂报告
* 维度化组织解读内容
* 分离报告生成与导出
* NoSQL 存储灵活结构

---

### 🏗️ [11-06-06 应用服务层设计](./11-06-06-应用服务层设计.md)

**核心内容**：

* Assessment 应用服务
  * EvaluationService - 评估编排服务
  * SubmissionService - 提交查询服务
  * QueryService - 测评查询服务
* Report 应用服务
  * ReportQueryService - 报告查询服务
  * ReportExportService - 报告导出服务
* Score 应用服务
  * ScoreQueryService - 得分查询服务
  * ScoreTrendService - 趋势分析服务
* DTO 设计与转换
* 事务边界管理
* 异步处理集成

**关键要点**：

* 应用服务协调领域服务
* 编排跨聚合的业务流程
* DTO 作为防腐层
* 明确的事务边界

---

### 🔧 [11-06-07 设计模式应用总结](./11-06-07-设计模式应用总结.md)

**核心内容**：

* 策略模式（CalculationStrategy、InterpretationStrategy、ReportExporter）
* 构建器模式（AssessmentCreator、ReportBuilder）
* 注册器模式（StrategyRegistry）
* 状态机模式（Assessment Status）
* 领域事件模式（AssessmentCreated、EvaluationCompleted）
* 仓储模式（AssessmentRepository、ReportRepository）

**关键要点**：

* 每种模式的应用场景
* 模式之间的协作关系
* 扩展性保证
* 性能优化考虑

---

### 📦 [11-06-08 扩展指南](./11-06-08-扩展指南.md)

**核心内容**：

* 如何新增计算策略
* 如何新增解读策略
* 如何新增导出格式
* 如何自定义报告模板
* 扩展示例（中位数策略、百分位解读）

**关键要点**：

* 扩展无需修改核心代码
* 注册即可生效
* 完整的扩展示例
* 性能优化建议

---

## 阅读建议

### 📖 全面学习路径

1. 从 **11-06-01** 开始，理解整体架构
2. 按顺序阅读 **11-06-02** 到 **11-06-05**，深入理解各个聚合
3. 阅读 **11-06-06** 了解应用服务编排
4. 阅读 **11-06-07** 总结设计模式
5. 参考 **11-06-08** 进行扩展开发

### 🎯 快速上手路径

1. **11-06-01** - 了解整体架构
2. **11-06-02** - 理解测评流程（核心）
3. **11-06-03** + **11-06-04** - 理解计算和解读
4. **11-06-06** - 查看应用服务使用示例

### 🔍 问题导向路径

* **如何创建测评？** → 11-06-02（AssessmentCreator）
* **如何新增计算策略？** → 11-06-03 + 11-06-08
* **如何配置解读规则？** → 11-06-04
* **如何生成报告？** → 11-06-05
* **如何异步处理？** → 11-06-01（事件） + 11-06-06

---

## 代码位置

```text
internal/apiserver/domain/evaluation/
├── assessment/                 # Assessment 聚合（11-06-02）
│   ├── assessment.go           # 聚合根
│   ├── score.go                # Score 实体
│   ├── creator.go              # 创建器（Builder）
│   ├── types.go                # 值对象（Status、Origin）
│   ├── events.go               # 领域事件
│   ├── errors.go               # 领域错误
│   └── repository.go           # 仓储接口
│
├── calculation/                # Calculation 策略（11-06-03）
│   ├── strategy.go             # 策略接口 + 注册器
│   ├── sum.go                  # 求和策略
│   ├── average.go              # 平均值策略
│   ├── weighted_sum.go         # 加权和策略
│   ├── extremum.go             # 极值策略
│   ├── auxiliary.go            # 辅助策略
│   └── types.go                # 通用类型
│
├── interpretation/             # Interpretation 策略（11-06-04）
│   ├── strategy.go             # 策略接口 + 注册器
│   ├── threshold.go            # 阈值策略
│   ├── composite.go            # 复合策略
│   ├── types.go                # 值对象（ScoreRange、InterpretItem）
│   └── errors.go               # 领域错误
│
└── report/                     # Report 聚合（11-06-05）
    ├── report.go               # 聚合根
    ├── dimension.go            # Dimension 实体
    ├── suggestion.go           # Suggestion 值对象
    ├── builder.go              # 构建器（Builder）
    ├── exporter.go             # 导出器（Strategy）
    ├── types.go                # 值对象
    ├── errors.go               # 领域错误
    └── repository.go           # 仓储接口

internal/apiserver/application/evaluation/  # 应用服务（11-06-06）
├── assessment/
│   ├── evaluation_service.go   # 评估编排服务
│   ├── submission_service.go   # 提交服务
│   ├── query_service.go        # 查询服务
│   └── dto.go                  # DTO 定义
└── report/
    ├── query_service.go        # 报告查询服务
    ├── export_service.go       # 报告导出服务
    └── dto.go                  # DTO 定义

internal/apiserver/infra/
├── mysql/evaluation/           # MySQL 持久化
│   ├── assessment_repository.go
│   └── score_repository.go
└── mongo/evaluation/           # MongoDB 持久化
    └── report_repository.go
```

---

## 核心流程图

### 测评评估流程

```text
1. 创建测评
   ├─ 验证测评对象（Testee）
   ├─ 验证问卷（Questionnaire）
   ├─ 验证答卷（AnswerSheet）
   ├─ 验证量表（MedicalScale）
   └─ 创建 Assessment（Pending 状态）

2. 启动评估（异步）
   ├─ 状态：Pending → Evaluating
   ├─ 计算分数（CalculationStrategy）
   │   ├─ 因子得分计算
   │   └─ 总分计算
   ├─ 生成解读（InterpretationStrategy）
   │   ├─ 因子解读
   │   └─ 总体解读
   ├─ 构建报告（ReportBuilder）
   │   ├─ 维度化组织
   │   └─ 生成建议
   └─ 状态：Evaluating → Completed

3. 报告导出
   ├─ 查询报告
   ├─ 选择导出格式
   └─ 生成文件（PDF/Word）
```

---

## 文档特色

✅ **与代码完全一致**：所有示例代码都来自实际实现  
✅ **金字塔结构**：从宏观到微观，层层递进  
✅ **设计模式导向**：重点阐述模式应用与协作  
✅ **扩展性保证**：详细的扩展指南和示例  
✅ **实战导向**：包含完整的使用示例  
✅ **流程可视化**：清晰的流程图和状态机  

---

## 关键设计决策

### 为什么使用策略模式？

* **计算策略**：不同量表需要不同的计算方法（求和、加权、平均等）
* **解读策略**：不同量表有不同的解读规则（阈值、复合条件）
* **导出策略**：支持多种导出格式（PDF、Word、Excel）

### 为什么使用 Builder 模式？

* **Assessment 创建**：需要验证多个依赖对象，创建过程复杂
* **Report 构建**：报告结构复杂，需要分步构建

### 为什么使用事件驱动？

* **异步处理**：评估计算耗时，不阻塞主流程
* **解耦合**：评估完成后可能触发多个后续操作（通知、统计等）
* **可扩展**：新增事件监听器不影响核心逻辑

### 为什么分离 Assessment 和 Report？

* **职责单一**：Assessment 管理流程，Report 管理结果
* **存储分离**：Assessment 用关系型（MySQL），Report 用文档型（MongoDB）
* **生命周期不同**：Assessment 可能重新评估，Report 是历史快照

---

> **相关文档**：
>
> * 《11-01-问卷&量表BC领域模型总览-v2.md》
> * 《11-02-qs-apiserver领域层代码结构设计-v2.md》
> * 《11-04-Survey子域设计》
> * 《11-05-Scale子域设计-v2.md》
