# 11-02 qs-apiserver 领域层代码结构设计（V2）

> 版本：V2.0
> 目标：将问卷&量表 BC 的领域模型，映射为 `qs-apiserver` / `qs-worker` 可复用的 Go 领域层代码结构。

本篇文档站在 **实现视角**，在《11-01-问卷&量表 BC 领域模型总览（V2）》基础上，给出：

* `internal/domain` 目录结构；
* 各包内核心聚合/实体/VO 的结构草图；
* 仓储接口划分（MySQL / Mongo）；
* qs-apiserver 与 qs-worker 如何共用这套领域模型。

---

## 1. 目录结构总览

```text
internal/domain/
  common/
  survey/
  scale/
  assessment/
  user/
  plan/
  screening/
```

说明：

* `common/`：通用 VO、ID 类型、枚举等；
* `survey/`：问卷模板、答卷聚合 + 校验规则接口；
* `scale/`：量表定义、计分与解读 Evaluator；
* `assessment/`：测评案例、维度分数、解读报告；
* `user/`：Testee / Staff 模型；
* `plan/`：测评计划/任务模型；
* `screening/`：入校筛查项目模型。

`qs-worker` 服务不单独拷贝领域模型，而是直接依赖同一个 `internal/domain` 包（通过 go module / go work 管理）。

---

## 2. common 包

ID、时间、通用枚举等定义在此，避免循环依赖。

示例：

```go
package common

type ID string
type Time = time.Time
```

也可以在各子域中定义 `type AssessmentID string` 之类，common 只提供工具函数。

---

## 3. survey 包：问卷与答卷

**路径：** `internal/domain/survey`

核心聚合：

* `Questionnaire`：问卷模板
* `AnswerSheet`：答卷实例

示意：

```go
type QuestionnaireID string
type QuestionID string

type QuestionnaireStatus string

const (
    QuestionnaireStatusDraft     QuestionnaireStatus = "draft"
    QuestionnaireStatusPublished QuestionnaireStatus = "published"
    QuestionnaireStatusArchived  QuestionnaireStatus = "archived"
)

type Questionnaire struct {
    id          QuestionnaireID
    code        string
    title       string
    description string
    version     int
    status      QuestionnaireStatus
    questions   []Question
    createdAt   time.Time
    updatedAt   time.Time
}
```

题目与选项：

```go
type QuestionType string

const (
    QuestionTypeSingleChoice QuestionType = "single_choice"
    QuestionTypeMultiChoice  QuestionType = "multi_choice"
    QuestionTypeNumber       QuestionType = "number"
    QuestionTypeText         QuestionType = "text"
)

type RuleType string
type RuleConfig struct {
    Type   RuleType
    Params map[string]string
}

type ScoreStrategyCode string

type ScoringConfig struct {
    Strategy ScoreStrategyCode
    Params   map[string]string
}

type Question struct {
    id             QuestionID
    qType          QuestionType
    title          string
    required       bool
    options        []Option
    validationRule []RuleConfig
    scoringConfig  *ScoringConfig
}

type Option struct {
    code  string
    text  string
    value string
}
```

答卷：

```go
type AnswerSheetID string

type AnswerSheetStatus string

const (
    AnswerSheetStatusDraft     AnswerSheetStatus = "draft"
    AnswerSheetStatusSubmitted AnswerSheetStatus = "submitted"
)

type AnswerItem struct {
    QuestionID QuestionID
    Values     []string
}

type AnswerSheet struct {
    id              AnswerSheetID
    questionnaireID QuestionnaireID
    items           []AnswerItem
    status          AnswerSheetStatus
    submittedAt     *time.Time
    createdAt       time.Time
    updatedAt       time.Time
}
```

校验接口（详见 12-01）：

```go
type AnswerSheetValidator interface {
    Validate(ctx context.Context, q *Questionnaire, s *AnswerSheet) error
}
```

仓储接口：

```go
type QuestionnaireRepository interface {
    FindByID(ctx context.Context, id QuestionnaireID) (*Questionnaire, error)
    FindByCode(ctx context.Context, code string) (*Questionnaire, error)
    Save(ctx context.Context, q *Questionnaire) error
}

type AnswerSheetRepository interface {
    FindByID(ctx context.Context, id AnswerSheetID) (*AnswerSheet, error)
    Save(ctx context.Context, s *AnswerSheet) error
}
```

---

## 4. scale 包：量表与评估

**路径：** `internal/domain/scale`

量表定义：

```go
type MedicalScaleID string
type FactorCode string

type Factor struct {
    Code        FactorCode
    Name        string
    QuestionIDs []survey.QuestionID
    Strategy    FactorScoreStrategyCode
    Params      map[string]string
}

type RiskLevel string

const (
    RiskLevelNone RiskLevel = "none"
    RiskLevelLow  RiskLevel = "low"
    RiskLevelMid  RiskLevel = "mid"
    RiskLevelHigh RiskLevel = "high"
)

type InterpretationRule struct {
    MinScore   float64
    MaxScore   float64
    RiskLevel  RiskLevel
    Conclusion string
    Suggestion string
}

type MedicalScale struct {
    id        MedicalScaleID
    code      string
    name      string
    version   int
    factors   []Factor
    rules     []InterpretationRule
    createdAt time.Time
    updatedAt time.Time
}
```

评估结果：

```go
type FactorScore struct {
    FactorCode FactorCode
    RawScore   float64
    RiskLevel  RiskLevel
}

type EvaluationResult struct {
    TotalScore   float64
    RiskLevel    RiskLevel
    FactorScores []FactorScore
    Conclusion   string
    Suggestion   string
}
```

Evaluator 接口（详见 12-02）：

```go
type Evaluator interface {
    Evaluate(
        ctx context.Context,
        scale *MedicalScale,
        questionnaire *survey.Questionnaire,
        sheet *survey.AnswerSheet,
    ) (*EvaluationResult, error)
}
```

---

## 5. assessment 包：测评案例

**路径：** `internal/domain/assessment`

Assessment：

```go
type AssessmentID string

type OriginType string

const (
    OriginTypeAdhoc     OriginType = "adhoc"
    OriginTypePlan      OriginType = "plan"
    OriginTypeScreening OriginType = "screening"
)

type Status string

const (
    StatusPending     Status = "pending"
    StatusSubmitted   Status = "submitted"
    StatusInterpreted Status = "interpreted"
    StatusFailed      Status = "failed"
)

type Assessment struct {
    id              AssessmentID
    testeeID        user.TesteeID
    questionnaireID survey.QuestionnaireID
    answerSheetID   survey.AnswerSheetID
    medicalScaleID  *scale.MedicalScaleID
    originType      OriginType
    originID        *string
    status          Status
    totalScore      *float64
    riskLevel       *scale.RiskLevel
    createdAt       time.Time
    updatedAt       time.Time
    interpretedAt   *time.Time
}
```

维度分与报告：

```go
type AssessmentScore struct {
    AssessmentID AssessmentID
    FactorCode   scale.FactorCode
    RawScore     float64
    RiskLevel    scale.RiskLevel
}

type InterpretReportID = AssessmentID

type InterpretReport struct {
    ID          InterpretReportID
    ScaleName   string
    TotalScore  float64
    RiskLevel   scale.RiskLevel
    Dimensions  []DimensionReport
    Conclusion  string
    Suggestions []string
}

type DimensionReport struct {
    Code      scale.FactorCode
    Name      string
    Score     float64
    RiskLevel scale.RiskLevel
    Comment   string
}
```

仓储与事件接口：

```go
type AssessmentRepository interface {
    FindByID(ctx context.Context, id AssessmentID) (*Assessment, error)
    Save(ctx context.Context, a *Assessment) error
}

type InterpretReportRepository interface {
    FindByID(ctx context.Context, id InterpretReportID) (*InterpretReport, error)
    Save(ctx context.Context, r *InterpretReport) error
}

type AssessmentScoreRepository interface {
    SaveScores(ctx context.Context, scores []AssessmentScore) error
    FindByAssessmentID(ctx context.Context, id AssessmentID) ([]AssessmentScore, error)
}
```

---

## 6. user / plan / screening 包

这里只给简要草图，完整说明详见 11-03、12-05。

* `user`：Testee / Staff
* `plan`：AssessmentPlan / AssessmentTask
* `screening`：ScreeningProject

---

## 7. qs-apiserver 与 qs-worker 对领域层的使用

* **qs-apiserver**

  * 面向小程序/后台提供 API；
  * 主要职责：

    * 加载 Questionnaire / Testee / Staff；
    * 创建 & 校验 & 保存 AnswerSheet；
    * 创建 Assessment，发布 AssessmentSubmittedEvent；
    * 提供 Assessment / InterpretReport / AssessmentScore 查询；
  * 依赖：

    * `internal/domain/*` + `internal/infra`（仓储实现） + MQ Producer。

* **qs-worker**

  * 专职评估任务：

    * 消费 AssessmentSubmittedEvent；
    * 加载 Assessment / Questionnaire / AnswerSheet / MedicalScale；
    * 调用 scale.Evaluator；
    * 写回 AssessmentScore / InterpretReport / Assessment 状态；
    * 发布 AssessmentInterpretedEvent。
  * 依赖同一套 `internal/domain/*` 包。

---

## 8. 实施建议

1. 按本文件结构先搭起目录与 type stub，再逐步充实现。
2. 领域层不直接依赖 GORM/Mongo Driver，只通过仓储接口做 IO。
3. 严格控制依赖方向，禁止子域间的环状引用。
4. 优先抽象“行为”而不是一次性预埋所有字段，保持聚合内不变量清晰。

本结构是 V2 代码层面的“蓝本”，后续代码实现必须与之保持同步，如有重大调整需更新本文件。
