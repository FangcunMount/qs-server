# 11-06 Assessment 子域设计（V2）

> **版本**：V2.0  
> **范围**：问卷&量表 BC 中的 assessment 子域  
> **目标**：阐述测评案例子域的核心职责、Assessment 聚合根设计、状态机、领域事件、与其他子域的协作

---

## 1. Assessment 子域的定位与职责

### 1.1 子域边界

**assessment 子域关注的核心问题**：

* "测评实例"：一次具体的测评行为（谁、什么时候、用什么问卷/量表、结果如何）
* "生命周期管理"：测评从创建到完成评估的完整流程
* "结果追溯"：记录测评结果，支持历史查询和趋势分析
* "业务桥接"：连接 survey（问卷答卷）、scale（量表规则）、user（受试者）、plan/screening（业务来源）

**assessment 子域不关心的问题**：

* "问卷怎么展示"：这是 survey 子域的职责
* "如何计分和解读"：这是 scale 子域的职责
* "用户权限管理"：这是 IAM 系统的职责

### 1.2 核心聚合

assessment 子域包含三个核心聚合：

1. **Assessment 聚合根**：测评实例
   * 记录一次测评行为的元数据（谁、什么时候、用什么）
   * 管理测评状态（pending / submitted / interpreted / failed）
   * 记录评估结果（总分、风险等级）

2. **AssessmentScore 实体**：维度/因子得分
   * 记录每个因子的详细得分
   * 支持按维度查询和趋势分析
   * 存储在 MySQL（便于统计查询）

3. **InterpretReport 聚合根**：解读报告
   * 对外展示的完整解读报告
   * 包含总分、风险等级、因子解读、结论、建议
   * 存储在 MongoDB（文档型，便于灵活扩展）

### 1.3 与其他子域的关系

* **依赖** survey 子域：引用 QuestionnaireID 和 AnswerSheetID
* **依赖** scale 子域：引用 MedicalScaleID，调用 Evaluator 执行评估
* **依赖** user 子域：引用 TesteeID
* **依赖** plan/screening 子域：引用 PlanID/ScreeningProjectID（可选）

**依赖方向**：

```text
survey、scale、user、plan、screening（独立或相互独立）
             ↓
          assessment（桥接所有子域）
```

**assessment 是依赖链的末端**，负责把各个子域的能力组合成完整的业务流程。

---

## 2. Assessment 聚合根

### 2.1 聚合根定位

Assessment 是 assessment 子域的核心聚合根，代表"一次具体的测评行为"。

**核心职责**：

1. 记录测评元数据（谁做的、用什么问卷/量表、来源于哪个业务场景）
2. 管理测评生命周期（创建 → 提交 → 评估 → 完成/失败）
3. 记录评估结果（总分、风险等级）
4. 发布领域事件（AssessmentSubmittedEvent、AssessmentInterpretedEvent）

### 2.2 聚合根结构

```go
// Assessment 测评聚合根
type Assessment struct {
    // 基本标识
    id     AssessmentID
    orgID  string
    
    // 关联实体（通过 ID 引用，不直接持有对象）
    testeeID        TesteeID
    questionnaireID QuestionnaireID
    answerSheetID   AnswerSheetID
    medicalScaleID  *MedicalScaleID // 可选：纯问卷模式为 nil
    
    // 业务来源
    originType AssessmentOriginType // adhoc / plan / screening
    originID   *string              // 对应 PlanID / ScreeningProjectID
    
    // 状态与结果
    status    AssessmentStatus
    totalScore *float64
    riskLevel  *RiskLevel
    
    // 时间戳
    createdAt     time.Time
    submittedAt   *time.Time
    interpretedAt *time.Time
    failedAt      *time.Time
    
    // 失败信息
    failureReason *string
    
    // 领域事件（未持久化）
    events []DomainEvent
}
```

### 2.3 核心类型定义

#### 2.3.1 AssessmentStatus 枚举

```go
// AssessmentStatus 测评状态
type AssessmentStatus string

const (
    // AssessmentStatusPending 待提交：已创建，但答卷尚未提交
    AssessmentStatusPending AssessmentStatus = "pending"
    
    // AssessmentStatusSubmitted 已提交：答卷已提交，等待评估
    AssessmentStatusSubmitted AssessmentStatus = "submitted"
    
    // AssessmentStatusInterpreted 已解读：评估完成，报告已生成
    AssessmentStatusInterpreted AssessmentStatus = "interpreted"
    
    // AssessmentStatusFailed 评估失败
    AssessmentStatusFailed AssessmentStatus = "failed"
)
```

#### 2.3.2 AssessmentOriginType 枚举

```go
// AssessmentOriginType 测评来源类型
type AssessmentOriginType string

const (
    // AssessmentOriginAdhoc 一次性测评：手动创建，不属于任何计划或筛查
    AssessmentOriginAdhoc AssessmentOriginType = "adhoc"
    
    // AssessmentOriginPlan 测评计划：由 AssessmentPlan 生成的 AssessmentTask 创建
    AssessmentOriginPlan AssessmentOriginType = "plan"
    
    // AssessmentOriginScreening 入校筛查：由 ScreeningProject 创建
    AssessmentOriginScreening AssessmentOriginType = "screening"
)
```

### 2.4 构造函数与工厂方法

```go
// NewAssessment 创建测评（工厂方法）
func NewAssessment(
    orgID string,
    testeeID TesteeID,
    questionnaireID QuestionnaireID,
    answerSheetID AnswerSheetID,
    medicalScaleID *MedicalScaleID,
    originType AssessmentOriginType,
    originID *string,
) *Assessment {
    now := time.Now()
    
    return &Assessment{
        id:              NewAssessmentID(),
        orgID:           orgID,
        testeeID:        testeeID,
        questionnaireID: questionnaireID,
        answerSheetID:   answerSheetID,
        medicalScaleID:  medicalScaleID,
        originType:      originType,
        originID:        originID,
        status:          AssessmentStatusPending,
        createdAt:       now,
        events:          []DomainEvent{},
    }
}

// NewAdhocAssessment 创建一次性测评（快捷方法）
func NewAdhocAssessment(
    orgID string,
    testeeID TesteeID,
    questionnaireID QuestionnaireID,
    answerSheetID AnswerSheetID,
    medicalScaleID *MedicalScaleID,
) *Assessment {
    return NewAssessment(
        orgID,
        testeeID,
        questionnaireID,
        answerSheetID,
        medicalScaleID,
        AssessmentOriginAdhoc,
        nil,
    )
}

// NewPlanAssessment 从测评计划创建测评（快捷方法）
func NewPlanAssessment(
    orgID string,
    testeeID TesteeID,
    questionnaireID QuestionnaireID,
    answerSheetID AnswerSheetID,
    medicalScaleID *MedicalScaleID,
    planID string,
) *Assessment {
    return NewAssessment(
        orgID,
        testeeID,
        questionnaireID,
        answerSheetID,
        medicalScaleID,
        AssessmentOriginPlan,
        &planID,
    )
}

// NewScreeningAssessment 从入校筛查创建测评（快捷方法）
func NewScreeningAssessment(
    orgID string,
    testeeID TesteeID,
    questionnaireID QuestionnaireID,
    answerSheetID AnswerSheetID,
    medicalScaleID *MedicalScaleID,
    screeningProjectID string,
) *Assessment {
    return NewAssessment(
        orgID,
        testeeID,
        questionnaireID,
        answerSheetID,
        medicalScaleID,
        AssessmentOriginScreening,
        &screeningProjectID,
    )
}
```

### 2.5 状态迁移方法

#### 2.5.1 提交答卷

```go
// Submit 提交答卷
func (a *Assessment) Submit() error {
    // 前置条件：只有 pending 状态可以提交
    if a.status != AssessmentStatusPending {
        return NewDomainError(
            "assessment_invalid_status",
            fmt.Sprintf("cannot submit assessment in status %s", a.status),
        )
    }
    
    // 状态迁移
    now := time.Now()
    a.status = AssessmentStatusSubmitted
    a.submittedAt = &now
    
    // 发布领域事件
    a.addEvent(NewAssessmentSubmittedEvent(
        a.id,
        a.testeeID,
        a.questionnaireID,
        a.answerSheetID,
        a.medicalScaleID,
        now,
    ))
    
    return nil
}
```

#### 2.5.2 应用评估结果

```go
// ApplyEvaluation 应用评估结果
func (a *Assessment) ApplyEvaluation(result *scale.EvaluationResult) error {
    // 前置条件：只有 submitted 状态可以应用评估结果
    if a.status != AssessmentStatusSubmitted {
        return NewDomainError(
            "assessment_invalid_status",
            fmt.Sprintf("cannot apply evaluation in status %s", a.status),
        )
    }
    
    // 前置条件：必须绑定了量表
    if a.medicalScaleID == nil {
        return NewDomainError(
            "assessment_no_scale",
            "cannot apply evaluation without medical scale",
        )
    }
    
    // 更新评估结果
    now := time.Now()
    a.totalScore = &result.TotalScore
    a.riskLevel = &result.RiskLevel
    a.status = AssessmentStatusInterpreted
    a.interpretedAt = &now
    
    // 发布领域事件
    a.addEvent(NewAssessmentInterpretedEvent(
        a.id,
        a.testeeID,
        *a.medicalScaleID,
        result.TotalScore,
        result.RiskLevel,
        now,
    ))
    
    return nil
}
```

#### 2.5.3 标记失败

```go
// MarkAsFailed 标记评估失败
func (a *Assessment) MarkAsFailed(reason string) error {
    // 前置条件：只有 submitted 状态可以标记失败
    if a.status != AssessmentStatusSubmitted {
        return NewDomainError(
            "assessment_invalid_status",
            fmt.Sprintf("cannot mark as failed in status %s", a.status),
        )
    }
    
    // 状态迁移
    now := time.Now()
    a.status = AssessmentStatusFailed
    a.failedAt = &now
    a.failureReason = &reason
    
    // 发布领域事件
    a.addEvent(NewAssessmentFailedEvent(
        a.id,
        a.testeeID,
        reason,
        now,
    ))
    
    return nil
}
```

### 2.6 查询方法

```go
// ID 相关
func (a *Assessment) ID() AssessmentID { return a.id }
func (a *Assessment) OrgID() string { return a.orgID }

// 关联实体
func (a *Assessment) TesteeID() TesteeID { return a.testeeID }
func (a *Assessment) QuestionnaireID() QuestionnaireID { return a.questionnaireID }
func (a *Assessment) AnswerSheetID() AnswerSheetID { return a.answerSheetID }
func (a *Assessment) MedicalScaleID() *MedicalScaleID { return a.medicalScaleID }

// 业务来源
func (a *Assessment) OriginType() AssessmentOriginType { return a.originType }
func (a *Assessment) OriginID() *string { return a.originID }

// 状态与结果
func (a *Assessment) Status() AssessmentStatus { return a.status }
func (a *Assessment) TotalScore() *float64 { return a.totalScore }
func (a *Assessment) RiskLevel() *RiskLevel { return a.riskLevel }

// 时间戳
func (a *Assessment) CreatedAt() time.Time { return a.createdAt }
func (a *Assessment) SubmittedAt() *time.Time { return a.submittedAt }
func (a *Assessment) InterpretedAt() *time.Time { return a.interpretedAt }
func (a *Assessment) FailedAt() *time.Time { return a.failedAt }

// 失败信息
func (a *Assessment) FailureReason() *string { return a.failureReason }

// 领域事件
func (a *Assessment) Events() []DomainEvent { return a.events }
func (a *Assessment) ClearEvents() { a.events = []DomainEvent{} }

// 辅助方法
func (a *Assessment) IsPending() bool { return a.status == AssessmentStatusPending }
func (a *Assessment) IsSubmitted() bool { return a.status == AssessmentStatusSubmitted }
func (a *Assessment) IsInterpreted() bool { return a.status == AssessmentStatusInterpreted }
func (a *Assessment) IsFailed() bool { return a.status == AssessmentStatusFailed }
func (a *Assessment) HasMedicalScale() bool { return a.medicalScaleID != nil }
```

### 2.7 领域事件管理

```go
// addEvent 添加领域事件（私有方法）
func (a *Assessment) addEvent(event DomainEvent) {
    a.events = append(a.events, event)
}
```

---

## 3. AssessmentScore 实体

### 3.1 实体定位

AssessmentScore 记录测评的每个因子/维度的详细得分，是 Assessment 的从属实体。

**核心职责**：

1. 记录因子级别的得分和风险等级
2. 支持按维度查询和趋势分析
3. 便于生成折线图、雷达图等可视化

### 3.2 实体结构

```go
// AssessmentScore 测评得分
type AssessmentScore struct {
    assessmentID AssessmentID
    factorCode   FactorCode
    rawScore     float64
    riskLevel    RiskLevel
    createdAt    time.Time
}
```

**设计要点**：

* 联合主键：(AssessmentID, FactorCode)
* 存储在 MySQL：便于 SQL 聚合查询（如按 TesteeID + FactorCode 查询趋势）
* 不是聚合根：生命周期完全依赖 Assessment

### 3.3 构造函数

```go
// NewAssessmentScore 创建测评得分
func NewAssessmentScore(
    assessmentID AssessmentID,
    factorCode FactorCode,
    rawScore float64,
    riskLevel RiskLevel,
) *AssessmentScore {
    return &AssessmentScore{
        assessmentID: assessmentID,
        factorCode:   factorCode,
        rawScore:     rawScore,
        riskLevel:    riskLevel,
        createdAt:    time.Now(),
    }
}

// FromEvaluationResult 从评估结果批量创建
func FromEvaluationResult(
    assessmentID AssessmentID,
    result *scale.EvaluationResult,
) []*AssessmentScore {
    scores := make([]*AssessmentScore, 0, len(result.FactorScores))
    
    for _, fs := range result.FactorScores {
        scores = append(scores, NewAssessmentScore(
            assessmentID,
            fs.FactorCode,
            fs.RawScore,
            fs.RiskLevel,
        ))
    }
    
    return scores
}
```

### 3.4 查询方法

```go
func (s *AssessmentScore) AssessmentID() AssessmentID { return s.assessmentID }
func (s *AssessmentScore) FactorCode() FactorCode { return s.factorCode }
func (s *AssessmentScore) RawScore() float64 { return s.rawScore }
func (s *AssessmentScore) RiskLevel() RiskLevel { return s.riskLevel }
func (s *AssessmentScore) CreatedAt() time.Time { return s.createdAt }
```

---

## 4. InterpretReport 聚合根

### 4.1 聚合根定位

InterpretReport 是对外展示的解读报告，1:1 绑定 Assessment。

**核心职责**：

1. 提供结构化的解读报告内容
2. 包含总分、风险等级、因子解读、结论、建议
3. 支持灵活的报告格式扩展

### 4.2 聚合根结构

```go
// InterpretReport 解读报告聚合根
type InterpretReport struct {
    // ID 与 AssessmentID 一致（1:1 关系）
    id AssessmentID
    
    // 量表信息
    scaleName string
    
    // 总体评估
    totalScore float64
    riskLevel  RiskLevel
    conclusion string
    
    // 因子解读
    dimensions []DimensionInterpret
    
    // 建议
    suggestions []string
    
    // 元数据
    createdAt time.Time
}

// DimensionInterpret 维度解读
type DimensionInterpret struct {
    factorCode  FactorCode
    factorName  string
    rawScore    float64
    riskLevel   RiskLevel
    description string
}
```

**设计要点**：

* InterpretReportID == AssessmentID（1:1 关系）
* 存储在 MongoDB：文档型存储，便于灵活扩展报告格式
* 是聚合根：可以独立查询和展示，不依赖 Assessment 的其他字段

### 4.3 构造函数与工厂

```go
// NewInterpretReport 创建解读报告
func NewInterpretReport(
    assessmentID AssessmentID,
    scaleName string,
    totalScore float64,
    riskLevel RiskLevel,
    conclusion string,
    dimensions []DimensionInterpret,
    suggestions []string,
) *InterpretReport {
    return &InterpretReport{
        id:          assessmentID,
        scaleName:   scaleName,
        totalScore:  totalScore,
        riskLevel:   riskLevel,
        conclusion:  conclusion,
        dimensions:  dimensions,
        suggestions: suggestions,
        createdAt:   time.Now(),
    }
}
```

### 4.4 ReportFactory（领域服务）

ReportFactory 负责把 scale.EvaluationResult 转换为 InterpretReport。

```go
// ReportFactory 解读报告工厂
type ReportFactory interface {
    Build(
        assessment *Assessment,
        medicalScale *scale.MedicalScale,
        result *scale.EvaluationResult,
    ) *InterpretReport
}

// DefaultReportFactory 默认实现
type DefaultReportFactory struct{}

func NewDefaultReportFactory() *DefaultReportFactory {
    return &DefaultReportFactory{}
}

func (f *DefaultReportFactory) Build(
    assessment *Assessment,
    medicalScale *scale.MedicalScale,
    result *scale.EvaluationResult,
) *InterpretReport {
    // 构建维度解读
    dimensions := make([]DimensionInterpret, 0, len(result.FactorScores))
    for _, fs := range result.FactorScores {
        // 从量表中查找因子信息
        factor, err := medicalScale.FindFactor(fs.FactorCode)
        if err != nil {
            continue
        }
        
        // 从量表中查找解读规则
        rule, err := medicalScale.FindInterpretRule(&fs.FactorCode, fs.RawScore)
        description := ""
        if err == nil && rule != nil {
            description = rule.Conclusion
        }
        
        dimensions = append(dimensions, DimensionInterpret{
            factorCode:  fs.FactorCode,
            factorName:  factor.Name(),
            rawScore:    fs.RawScore,
            riskLevel:   fs.RiskLevel,
            description: description,
        })
    }
    
    // 构建建议列表
    suggestions := []string{}
    if result.Suggestion != "" {
        suggestions = append(suggestions, result.Suggestion)
    }
    
    return NewInterpretReport(
        assessment.ID(),
        medicalScale.Name(),
        result.TotalScore,
        result.RiskLevel,
        result.Conclusion,
        dimensions,
        suggestions,
    )
}
```

### 4.5 查询方法

```go
func (r *InterpretReport) ID() AssessmentID { return r.id }
func (r *InterpretReport) ScaleName() string { return r.scaleName }
func (r *InterpretReport) TotalScore() float64 { return r.totalScore }
func (r *InterpretReport) RiskLevel() RiskLevel { return r.riskLevel }
func (r *InterpretReport) Conclusion() string { return r.conclusion }
func (r *InterpretReport) Dimensions() []DimensionInterpret { return r.dimensions }
func (r *InterpretReport) Suggestions() []string { return r.suggestions }
func (r *InterpretReport) CreatedAt() time.Time { return r.createdAt }
```

---

## 5. 状态机设计

### 5.1 状态转换图

```text
┌─────────┐
│ pending │  创建测评，答卷未提交
└────┬────┘
     │ Submit()
     ↓
┌───────────┐
│ submitted │  答卷已提交，等待评估
└─────┬─────┘
      │
      ├─ ApplyEvaluation() ──→ ┌─────────────┐
      │                        │ interpreted │  评估完成
      │                        └─────────────┘
      │
      └─ MarkAsFailed() ──────→ ┌────────┐
                                 │ failed │  评估失败
                                 └────────┘
```

### 5.2 状态转换规则

| 当前状态 | 允许的操作 | 目标状态 | 触发事件 |
|---------|----------|---------|---------|
| pending | Submit() | submitted | AssessmentSubmittedEvent |
| submitted | ApplyEvaluation() | interpreted | AssessmentInterpretedEvent |
| submitted | MarkAsFailed() | failed | AssessmentFailedEvent |
| interpreted | 无 | - | - |
| failed | 无 | - | - |

**关键约束**：

* pending → submitted：只能提交一次
* submitted → interpreted：只能成功评估一次
* submitted → failed：评估失败后不可恢复（需要重新创建测评）
* interpreted 和 failed 是终态，不能再迁移

### 5.3 纯问卷模式的状态机

对于纯问卷模式（MedicalScaleID == nil），状态机简化：

```text
┌─────────┐
│ pending │
└────┬────┘
     │ Submit()
     ↓
┌───────────┐
│ submitted │  终态（不需要评估）
└───────────┘
```

---

## 6. 领域事件

### 6.1 事件设计原则

* **事件命名**：过去式，描述已发生的事实（如 AssessmentSubmitted，而非 SubmitAssessment）
* **事件内容**：包含足够的信息，让消费者能处理业务逻辑，但不包含完整聚合
* **事件不可变**：一旦发布，事件内容不可修改
* **事件顺序**：按时间顺序发布，保证因果关系

### 6.2 AssessmentSubmittedEvent

```go
// AssessmentSubmittedEvent 测评已提交事件
type AssessmentSubmittedEvent struct {
    baseEvent
    
    assessmentID    AssessmentID
    testeeID        TesteeID
    questionnaireID QuestionnaireID
    answerSheetID   AnswerSheetID
    medicalScaleID  *MedicalScaleID
    submittedAt     time.Time
}

func NewAssessmentSubmittedEvent(
    assessmentID AssessmentID,
    testeeID TesteeID,
    questionnaireID QuestionnaireID,
    answerSheetID AnswerSheetID,
    medicalScaleID *MedicalScaleID,
    submittedAt time.Time,
) *AssessmentSubmittedEvent {
    return &AssessmentSubmittedEvent{
        baseEvent:       newBaseEvent("assessment.submitted"),
        assessmentID:    assessmentID,
        testeeID:        testeeID,
        questionnaireID: questionnaireID,
        answerSheetID:   answerSheetID,
        medicalScaleID:  medicalScaleID,
        submittedAt:     submittedAt,
    }
}

func (e *AssessmentSubmittedEvent) AssessmentID() AssessmentID { return e.assessmentID }
func (e *AssessmentSubmittedEvent) TesteeID() TesteeID { return e.testeeID }
func (e *AssessmentSubmittedEvent) QuestionnaireID() QuestionnaireID { return e.questionnaireID }
func (e *AssessmentSubmittedEvent) AnswerSheetID() AnswerSheetID { return e.answerSheetID }
func (e *AssessmentSubmittedEvent) MedicalScaleID() *MedicalScaleID { return e.medicalScaleID }
func (e *AssessmentSubmittedEvent) SubmittedAt() time.Time { return e.submittedAt }
```

**用途**：

* qs-worker 消费此事件，触发评估流程
* 通知服务消费此事件，发送"答卷已提交"通知

### 6.3 AssessmentInterpretedEvent

```go
// AssessmentInterpretedEvent 测评已解读事件
type AssessmentInterpretedEvent struct {
    baseEvent
    
    assessmentID   AssessmentID
    testeeID       TesteeID
    medicalScaleID MedicalScaleID
    totalScore     float64
    riskLevel      RiskLevel
    interpretedAt  time.Time
}

func NewAssessmentInterpretedEvent(
    assessmentID AssessmentID,
    testeeID TesteeID,
    medicalScaleID MedicalScaleID,
    totalScore float64,
    riskLevel RiskLevel,
    interpretedAt time.Time,
) *AssessmentInterpretedEvent {
    return &AssessmentInterpretedEvent{
        baseEvent:      newBaseEvent("assessment.interpreted"),
        assessmentID:   assessmentID,
        testeeID:       testeeID,
        medicalScaleID: medicalScaleID,
        totalScore:     totalScore,
        riskLevel:      riskLevel,
        interpretedAt:  interpretedAt,
    }
}

func (e *AssessmentInterpretedEvent) AssessmentID() AssessmentID { return e.assessmentID }
func (e *AssessmentInterpretedEvent) TesteeID() TesteeID { return e.testeeID }
func (e *AssessmentInterpretedEvent) MedicalScaleID() MedicalScaleID { return e.medicalScaleID }
func (e *AssessmentInterpretedEvent) TotalScore() float64 { return e.totalScore }
func (e *AssessmentInterpretedEvent) RiskLevel() RiskLevel { return e.riskLevel }
func (e *AssessmentInterpretedEvent) InterpretedAt() time.Time { return e.interpretedAt }
```

**用途**：

* 通知服务消费此事件，发送"报告已生成"通知
* 预警服务消费此事件，对高风险案例发送预警
* 统计服务消费此事件，更新实时统计数据

### 6.4 AssessmentFailedEvent

```go
// AssessmentFailedEvent 测评失败事件
type AssessmentFailedEvent struct {
    baseEvent
    
    assessmentID AssessmentID
    testeeID     TesteeID
    reason       string
    failedAt     time.Time
}

func NewAssessmentFailedEvent(
    assessmentID AssessmentID,
    testeeID TesteeID,
    reason string,
    failedAt time.Time,
) *AssessmentFailedEvent {
    return &AssessmentFailedEvent{
        baseEvent:    newBaseEvent("assessment.failed"),
        assessmentID: assessmentID,
        testeeID:     testeeID,
        reason:       reason,
        failedAt:     failedAt,
    }
}

func (e *AssessmentFailedEvent) AssessmentID() AssessmentID { return e.assessmentID }
func (e *AssessmentFailedEvent) TesteeID() TesteeID { return e.testeeID }
func (e *AssessmentFailedEvent) Reason() string { return e.reason }
func (e *AssessmentFailedEvent) FailedAt() time.Time { return e.failedAt }
```

**用途**：

* 日志服务记录失败原因
* 监控服务统计失败率
* 通知服务发送失败通知（可选）

---

## 7. Repository 接口

### 7.1 AssessmentRepository

```go
// AssessmentRepository 测评仓储接口
type AssessmentRepository interface {
    // 保存测评（新增或更新）
    Save(ctx context.Context, assessment *Assessment) error
    
    // 根据 ID 查找
    FindByID(ctx context.Context, id AssessmentID) (*Assessment, error)
    
    // 根据答卷 ID 查找
    FindByAnswerSheetID(ctx context.Context, answerSheetID AnswerSheetID) (*Assessment, error)
    
    // 查询受试者的测评列表（支持分页）
    FindByTesteeID(
        ctx context.Context,
        testeeID TesteeID,
        pagination Pagination,
    ) ([]*Assessment, int, error)
    
    // 查询受试者在某个量表下的测评列表
    FindByTesteeIDAndScaleID(
        ctx context.Context,
        testeeID TesteeID,
        scaleID MedicalScaleID,
        pagination Pagination,
    ) ([]*Assessment, int, error)
    
    // 查询计划下的测评列表
    FindByPlanID(
        ctx context.Context,
        planID string,
        pagination Pagination,
    ) ([]*Assessment, int, error)
    
    // 查询筛查项目下的测评列表
    FindByScreeningProjectID(
        ctx context.Context,
        screeningProjectID string,
        pagination Pagination,
    ) ([]*Assessment, int, error)
    
    // 统计：按状态统计数量
    CountByStatus(ctx context.Context, status AssessmentStatus) (int, error)
    
    // 统计：按受试者和状态统计
    CountByTesteeIDAndStatus(
        ctx context.Context,
        testeeID TesteeID,
        status AssessmentStatus,
    ) (int, error)
}
```

### 7.2 AssessmentScoreRepository

```go
// AssessmentScoreRepository 测评得分仓储接口
type AssessmentScoreRepository interface {
    // 批量保存得分
    SaveScores(ctx context.Context, scores []*AssessmentScore) error
    
    // 查询测评的所有得分
    FindByAssessmentID(ctx context.Context, assessmentID AssessmentID) ([]*AssessmentScore, error)
    
    // 查询受试者在某个因子上的历史得分（用于趋势分析）
    FindByTesteeIDAndFactorCode(
        ctx context.Context,
        testeeID TesteeID,
        factorCode FactorCode,
        limit int,
    ) ([]*AssessmentScore, error)
    
    // 查询受试者在某个量表下所有因子的最新得分
    FindLatestByTesteeIDAndScaleID(
        ctx context.Context,
        testeeID TesteeID,
        scaleID MedicalScaleID,
    ) ([]*AssessmentScore, error)
}
```

### 7.3 InterpretReportRepository

```go
// InterpretReportRepository 解读报告仓储接口
type InterpretReportRepository interface {
    // 保存报告
    Save(ctx context.Context, report *InterpretReport) error
    
    // 根据 ID（即 AssessmentID）查找
    FindByID(ctx context.Context, id AssessmentID) (*InterpretReport, error)
    
    // 批量查询（根据 AssessmentID 列表）
    FindByIDs(ctx context.Context, ids []AssessmentID) ([]*InterpretReport, error)
    
    // 查询受试者的报告列表
    FindByTesteeID(
        ctx context.Context,
        testeeID TesteeID,
        pagination Pagination,
    ) ([]*InterpretReport, int, error)
}
```

---

## 8. 与其他子域的协作

### 8.1 与 survey 子域的协作

assessment 引用 survey 的聚合 ID：

```go
// 引用关系
type Assessment struct {
    questionnaireID QuestionnaireID // 引用 survey.Questionnaire
    answerSheetID   AnswerSheetID   // 引用 survey.AnswerSheet
}
```

**协作场景**：

1. **创建测评**：需要先创建 Questionnaire 和 AnswerSheet
2. **校验答卷**：提交测评前需要调用 survey.AnswerSheetValidator
3. **评估计算**：评估时需要加载 Questionnaire 和 AnswerSheet 传给 scale.Evaluator

### 8.2 与 scale 子域的协作

assessment 引用 scale 的聚合 ID，并调用 Evaluator：

```go
// 引用关系
type Assessment struct {
    medicalScaleID *MedicalScaleID // 引用 scale.MedicalScale（可选）
}

// 调用关系（在应用层）
type AssessmentService struct {
    scaleRepo scale.MedicalScaleRepository
    evaluator scale.Evaluator
}

func (s *AssessmentService) EvaluateAssessment(
    ctx context.Context,
    assessmentID AssessmentID,
) error {
    // 1. 加载 Assessment
    assessment, _ := s.assessmentRepo.FindByID(ctx, assessmentID)
    
    // 2. 加载 MedicalScale（来自 scale 子域）
    medicalScale, _ := s.scaleRepo.FindByID(ctx, *assessment.MedicalScaleID())
    
    // 3. 加载 Questionnaire 和 AnswerSheet（来自 survey 子域）
    questionnaire, _ := s.questionnaireRepo.FindByID(ctx, assessment.QuestionnaireID())
    answerSheet, _ := s.answerSheetRepo.FindByID(ctx, assessment.AnswerSheetID())
    
    // 4. 调用 Evaluator（来自 scale 子域）
    result, err := s.evaluator.Evaluate(ctx, medicalScale, questionnaire, answerSheet)
    if err != nil {
        assessment.MarkAsFailed(err.Error())
        s.assessmentRepo.Save(ctx, assessment)
        return err
    }
    
    // 5. 应用评估结果
    assessment.ApplyEvaluation(result)
    s.assessmentRepo.Save(ctx, assessment)
    
    // 6. 保存 AssessmentScore
    scores := FromEvaluationResult(assessment.ID(), result)
    s.scoreRepo.SaveScores(ctx, scores)
    
    // 7. 生成 InterpretReport
    report := s.reportFactory.Build(assessment, medicalScale, result)
    s.reportRepo.Save(ctx, report)
    
    return nil
}
```

### 8.3 与 user 子域的协作

assessment 引用 user 的 Testee：

```go
// 引用关系
type Assessment struct {
    testeeID TesteeID // 引用 user.Testee
}
```

**协作场景**：

1. **创建测评**：需要指定 TesteeID
2. **查询历史**：根据 TesteeID 查询测评列表
3. **趋势分析**：根据 TesteeID 聚合得分趋势

### 8.4 与 plan/screening 子域的协作

assessment 引用 plan 或 screening 的 ID：

```go
// 引用关系
type Assessment struct {
    originType AssessmentOriginType // adhoc / plan / screening
    originID   *string              // PlanID / ScreeningProjectID
}
```

**协作场景**：

1. **从计划创建测评**：AssessmentTask 生成测评时指定 PlanID
2. **从筛查创建测评**：ScreeningProject 生成测评时指定 ScreeningProjectID
3. **统计完成率**：根据 OriginID 统计计划/筛查的完成情况

---

## 9. 典型用例流程

### 9.1 一次性量表测评（adhoc）

```text
1. 前端：用户选择量表 → 创建问卷和答卷入口
   ├─ 应用层：创建 Questionnaire（survey）
   ├─ 应用层：创建 AnswerSheet（survey）
   └─ 应用层：创建 Assessment（assessment，status=pending）

2. 前端：用户填写答卷 → 提交
   ├─ 应用层：更新 AnswerSheet.Items
   ├─ 应用层：校验 AnswerSheet（survey.Validator）
   └─ 应用层：提交 Assessment（assessment.Submit()）
       └─ 领域层：发布 AssessmentSubmittedEvent

3. qs-worker：消费 AssessmentSubmittedEvent
   ├─ 应用层：加载 MedicalScale、Questionnaire、AnswerSheet
   ├─ 应用层：调用 Evaluator.Evaluate()（scale）
   ├─ 应用层：应用评估结果（assessment.ApplyEvaluation()）
   │   └─ 领域层：发布 AssessmentInterpretedEvent
   ├─ 应用层：保存 AssessmentScore（assessment）
   └─ 应用层：生成并保存 InterpretReport（assessment）

4. 前端：轮询查询报告
   └─ 应用层：根据 AssessmentID 查询 InterpretReport
```

### 9.2 纯问卷模式（无量表）

```text
1. 前端：用户选择问卷 → 创建答卷入口
   ├─ 应用层：创建 Questionnaire（survey）
   ├─ 应用层：创建 AnswerSheet（survey）
   └─ 应用层：创建 Assessment（assessment，medicalScaleID=nil，status=pending）

2. 前端：用户填写答卷 → 提交
   ├─ 应用层：更新 AnswerSheet.Items
   ├─ 应用层：校验 AnswerSheet（survey.Validator）
   └─ 应用层：提交 Assessment（assessment.Submit()）
       ├─ 领域层：发布 AssessmentSubmittedEvent
       └─ 注意：由于 medicalScaleID=nil，不触发评估流程

3. 前端：查询答卷
   └─ 应用层：根据 AssessmentID 查询 AnswerSheet（无报告）
```

---

## 10. 存储设计

### 10.1 Assessment 存储（MySQL）

**表名**：`assessments`

| 字段 | 类型 | 说明 |
|------|------|------|
| id | VARCHAR(36) PK | AssessmentID |
| org_id | VARCHAR(36) | 组织 ID |
| testee_id | VARCHAR(36) | 受试者 ID |
| questionnaire_id | VARCHAR(36) | 问卷 ID |
| answer_sheet_id | VARCHAR(36) | 答卷 ID |
| medical_scale_id | VARCHAR(36) NULL | 量表 ID |
| origin_type | VARCHAR(20) | 来源类型 |
| origin_id | VARCHAR(36) NULL | 来源 ID |
| status | VARCHAR(20) | 状态 |
| total_score | DECIMAL(10,2) NULL | 总分 |
| risk_level | VARCHAR(20) NULL | 风险等级 |
| created_at | TIMESTAMP | 创建时间 |
| submitted_at | TIMESTAMP NULL | 提交时间 |
| interpreted_at | TIMESTAMP NULL | 解读时间 |
| failed_at | TIMESTAMP NULL | 失败时间 |
| failure_reason | TEXT NULL | 失败原因 |

**索引**：

* PRIMARY KEY (id)
* INDEX idx_testee_id (testee_id)
* INDEX idx_answer_sheet_id (answer_sheet_id)
* INDEX idx_medical_scale_id (medical_scale_id)
* INDEX idx_origin (origin_type, origin_id)
* INDEX idx_status (status)
* INDEX idx_created_at (created_at)

### 10.2 AssessmentScore 存储（MySQL）

**表名**：`assessment_scores`

| 字段 | 类型 | 说明 |
|------|------|------|
| assessment_id | VARCHAR(36) PK | 测评 ID |
| factor_code | VARCHAR(50) PK | 因子编码 |
| raw_score | DECIMAL(10,2) | 原始分 |
| risk_level | VARCHAR(20) | 风险等级 |
| created_at | TIMESTAMP | 创建时间 |

**索引**：

* PRIMARY KEY (assessment_id, factor_code)
* INDEX idx_factor_code (factor_code)

**支持查询**：

* 按 AssessmentID 查询所有因子得分
* 按 TesteeID + FactorCode 查询趋势（需要 JOIN assessments 表）

### 10.3 InterpretReport 存储（MongoDB）

**集合名**：`interpret_reports`

```json
{
    "_id": "assessment-id-xxx",  // 与 AssessmentID 一致
    "scaleName": "抑郁自评量表（SDS）",
    "totalScore": 56.0,
    "riskLevel": "medium",
    "conclusion": "您的抑郁水平为中度...",
    "dimensions": [
        {
            "factorCode": "depression",
            "factorName": "抑郁维度",
            "rawScore": 28.0,
            "riskLevel": "medium",
            "description": "该维度得分较高，建议关注..."
        }
    ],
    "suggestions": [
        "建议保持规律作息",
        "可尝试运动放松"
    ],
    "createdAt": "2025-11-20T10:30:00Z"
}
```

**索引**：

* PRIMARY KEY (_id)
* INDEX (createdAt)

---

## 11. 扩展性设计

### 11.1 支持多种业务来源

通过 `originType` 和 `originID` 支持多种业务场景：

* **adhoc**：一次性测评
* **plan**：测评计划
* **screening**：入校筛查
* **未来扩展**：体检套餐、健康档案等

新增业务来源只需：

1. 增加 `AssessmentOriginType` 枚举值
2. 新增对应的创建测评快捷方法
3. 不影响现有代码

### 11.2 支持自定义报告格式

通过 MongoDB 存储 InterpretReport，支持灵活扩展报告格式：

```go
// 未来可以增加字段而不影响现有代码
type InterpretReport struct {
    // 现有字段...
    
    // 新增字段（可选）
    charts      []ChartData      // 图表数据
    attachments []Attachment     // 附件（PDF、图片等）
    customFields map[string]interface{} // 自定义字段
}
```

### 11.3 支持评估重试机制

如果评估失败，可以通过创建新的 Assessment 重试：

```go
// 应用层服务
func (s *AssessmentService) RetryAssessment(
    ctx context.Context,
    failedAssessmentID AssessmentID,
) (*Assessment, error) {
    // 1. 加载失败的 Assessment
    oldAssessment, _ := s.assessmentRepo.FindByID(ctx, failedAssessmentID)
    if oldAssessment.Status() != AssessmentStatusFailed {
        return nil, errors.New("assessment is not failed")
    }
    
    // 2. 创建新的 Assessment（复用相同的参数）
    newAssessment := NewAssessment(
        oldAssessment.OrgID(),
        oldAssessment.TesteeID(),
        oldAssessment.QuestionnaireID(),
        oldAssessment.AnswerSheetID(),
        oldAssessment.MedicalScaleID(),
        oldAssessment.OriginType(),
        oldAssessment.OriginID(),
    )
    
    // 3. 直接提交（跳过 pending 状态）
    newAssessment.Submit()
    s.assessmentRepo.Save(ctx, newAssessment)
    
    return newAssessment, nil
}
```

---

## 12. 目录结构

```text
internal/domain/assessment/
├── assessment.go              // Assessment 聚合根
├── assessment_score.go        // AssessmentScore 实体
├── interpret_report.go        // InterpretReport 聚合根
├── report_factory.go          // ReportFactory 领域服务
├── types.go                   // 类型定义（ID、枚举等）
├── errors.go                  // 领域错误
│
├── events.go                  // 事件基类
├── event_submitted.go         // AssessmentSubmittedEvent
├── event_interpreted.go       // AssessmentInterpretedEvent
├── event_failed.go            // AssessmentFailedEvent
│
└── repository.go              // Repository 接口定义
```

---

## 13. 总结

本文档详细阐述了 assessment 子域的设计，核心要点包括：

### 13.1 三个核心聚合

1. **Assessment 聚合根**：
   * 代表一次测评行为的完整记录
   * 管理测评生命周期（pending → submitted → interpreted/failed）
   * 桥接 survey、scale、user、plan/screening 子域

2. **AssessmentScore 实体**：
   * 记录因子级别的详细得分
   * 存储在 MySQL，便于统计查询和趋势分析

3. **InterpretReport 聚合根**：
   * 对外展示的解读报告
   * 存储在 MongoDB，支持灵活的报告格式扩展

### 13.2 状态机与领域事件

* **状态机**：pending → submitted → interpreted/failed
* **领域事件**：AssessmentSubmittedEvent、AssessmentInterpretedEvent、AssessmentFailedEvent
* **事件驱动**：通过事件解耦提交和评估流程，支持异步处理

### 13.3 与其他子域的协作

* **survey**：引用 QuestionnaireID 和 AnswerSheetID
* **scale**：引用 MedicalScaleID，调用 Evaluator
* **user**：引用 TesteeID
* **plan/screening**：通过 originType 和 originID 关联

### 13.4 扩展性设计

* 支持多种业务来源（adhoc、plan、screening、未来扩展）
* 支持纯问卷模式（medicalScaleID = nil）
* 支持自定义报告格式（MongoDB 灵活存储）
* 支持评估重试机制

### 13.5 设计原则

* **聚合边界清晰**：三个聚合各司其职，不越界
* **单向依赖**：依赖其他子域，不被其他子域依赖
* **事件驱动**：通过领域事件实现异步解耦
* **业务桥接**：作为依赖链末端，组合各子域能力

---

## 附录：与相关文档的关系

* **《11-01-问卷&量表 BC 领域模型总览-v2.md》**：定义了 assessment 子域在整个 BC 中的定位
* **《11-02-qs-apiserver 领域层代码结构设计-v2.md》**：定义了 assessment 子域的目录结构
* **《11-04-Survey 子域设计-v2.md》**：定义了 assessment 依赖的 Questionnaire 和 AnswerSheet
* **《11-05-Scale 子域设计-v2.md》**：定义了 assessment 调用的 MedicalScale 和 Evaluator
* **《12-03-评估工作流与 qs-worker 设计-v2.md》**：描述了 qs-worker 如何消费 AssessmentSubmittedEvent 完成评估
