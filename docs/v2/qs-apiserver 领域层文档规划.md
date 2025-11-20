# qs-apiserver 领域层文档规划

## 一、总体说明

本规划聚焦问卷&量表 BC 在 `qs-apiserver` 内部的**领域层**，将已讨论清楚的内容（领域模块拆分、聚合设计、校验/计分策略、Evaluator 职责链、qs-worker 评估流程）整理成一组可维护的设计文档。

目标：

* 与现有文档（《01-架构设计总览》《01-01-问卷领域设计》《10-Redis 消息队列实现》等）风格保持一致
* 让领域建模 → 代码结构 → 校验&计分 → 评估工作流形成完整闭环
* 后续按本规划逐篇落地为独立的 Markdown 文件

---

## 二、建议新增/细化的文档列表

> 这里只给出“有哪些文档、每篇负责什么”，暂不展开具体正文。

### 1. 《01-02-问卷&量表 BC 领域模型总览》

**定位：** 站在领域建模视角，对问卷&量表 BC 的领域对象做鸟瞰式梳理。

**主要内容：**

* 限界上下文与子域：survey / scale / assessment / user / plan / screening
* 各子域下的聚合、实体、值对象清单：

  * survey：Questionnaire、Question、Option、AnswerSheet
  * scale：MedicalScale、Factor、InterpretationRule
  * assessment：Assessment、AssessmentScore、InterpretReport
  * user：Testee、Staff
  * plan：AssessmentPlan、AssessmentTask
  * screening：ScreeningProject
* 子域之间的关系：

  * Assessment 作为桥接聚合，连接 AnswerSheet 和 MedicalScale
  * Testee/Staff 与 IAM.User 的 ID 映射关系
  * Plan/Screening 与 Assessment 的从属关系
* 领域依赖方向：

  * survey 不依赖 scale
  * scale 依赖 survey 的 Question/AnswerSheet 视图
  * assessment 依赖 survey + scale + user

### 2. 《01-03-qs-apiserver 领域层代码结构设计》

**定位：** 把领域模型映射为 Go 代码结构与目录划分。

**主要内容：**

* `internal/domain` 目录树：

  * `survey/`：questionnaire.go, question.go, answersheet.go, repository.go, validator.go
  * `scale/`：medical_scale.go, evaluator.go, question_scoring.go, factor_scoring.go, repository.go
  * `assessment/`：assessment.go, score.go, interpret_report.go, events.go, repository.go
  * `user/`：testee.go, staff.go, repository.go
  * `plan/`：plan.go, task.go, repository.go
  * `screening/`：screening_project.go, repository.go
  * `common/`：id.go, types.go
* 各聚合的核心字段与关键行为（构造函数、新建/发布/提交/状态迁移等）
* 仓储接口定义与基础设施实现分层原则
* qs-apiserver 与 qs-worker 如何共享同一套 domain 包

### 3. 《01-04-校验与计分规则设计（策略 + 职责链）》

**定位：** 专门说明“校验”和“计分/解读”这两块无状态业务逻辑的设计。

**主要内容：**

* 校验部分（survey 子域）：

  * `RuleConfig`：题目上的规则配置（RuleType + Params）
  * `QuestionRule` 策略接口及实现：必填、数值范围、选项个数、依赖题、正则……
  * `QuestionRuleFactory`：配置 → 策略实例
  * `AnswerSheetValidator`：组合多规则，对整份答卷做领域校验
* 题目计分部分（scale 子域）：

  * `ScoringConfig`：题目计分配置（ScoreStrategyCode + Params）
  * `QuestionScoringStrategy`：option 映射、数值题等策略
  * `QuestionScoringFactory`：配置 → 计分策略
* 因子计分部分：

  * `FactorScoreStrategyCode`：sum / avg / 自定义
  * `FactorScoringStrategy` + factory
* Evaluator 职责链：

  * EvalContext：贯穿原始分、因子分、最终结果
  * EvalStep 接口与三个核心 Step：RawScoreStep / FactorScoreStep / OverallInterpretStep
  * ChainEvaluator：按顺序执行 Step，输出 EvaluationResult

### 4. 《01-05-评估工作流与 qs-worker 设计》

**定位：** 把“提交答卷 → 异步评估 → 生成报告 → 通知”的整条链路讲清楚，连接领域层和消息队列设计。

**主要内容：**

* Assessment 生命周期：pending → submitted → interpreted → failed
* 领域事件：AssessmentSubmittedEvent / AssessmentInterpretedEvent
* 消息中间件：

  * NSQ/RedisMQ Topic & Channel 约定（例如 `assessment_events` / `evaluation`）
  * qs-apiserver 作为 Producer，qs-worker 作为 Consumer
* qs-worker 内部流程：

  * 消费 AssessmentSubmittedEvent
  * 加载 Questionnaire / AnswerSheet / MedicalScale
  * 调用 ChainEvaluator 得到 EvaluationResult
  * 映射为 AssessmentScore + InterpretReport（由 assessment.ReportFactory 完成）
  * 更新 Assessment 状态并发出 AssessmentInterpretedEvent
* 与高并发设计的连接：

  * 提交答卷后立即返回，评估异步
  * 前端轮询 `/status` 查询 Assessment 状态
  * 后续可接入长轮询 / 通知消息

---

## 三、小结

当前问卷&量表 BC 在 qs-apiserver 内部适合新增/细化以下 4 篇设计文档：

1. 领域模型总览（概念视角）
2. 领域层代码结构（实现视角）
3. 校验与计分规则（无状态领域服务视角）
4. 评估工作流与 qs-worker（事件流视角）

建议按优先级依次落地正文，一般先写 **01-02 总览**和 **01-03 代码结构**，再将第三、第四篇作为细化章节补充。
