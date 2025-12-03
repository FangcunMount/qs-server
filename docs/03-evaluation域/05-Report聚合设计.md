# 11-06-05 Report 聚合设计

> **版本**：V2.0  
> **最后更新**：2025-11-29  
> **状态**：✅ 已实现并验证

---

## 📋 文档导航

**当前位置**：Report 聚合设计（你在这里）  
**前置阅读**：[11-06-04 Interpretation解读策略设计](./11-06-04-Interpretation解读策略设计.md)  
**后续阅读**：[11-06-06 应用服务层设计](./11-06-06-应用服务层设计.md)

---

## 🎯 核心设计思想（30秒速览）

> **如果只有30秒，你需要知道这些：**

```text
┌────────────────────────────────────────────────────────────────────────────┐
│                                                                            │
│   Report = 测评报告 = "把解读结果变成可视化文档"                           │
│                                                                            │
│   ┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐      │
│   │   解读结果       │ ──▶ │   ReportBuilder │ ──▶ │ InterpretReport │      │
│   │ [风险等级+结论]  │     │    (构建器)      │     │   (聚合根)       │      │
│   └─────────────────┘     └─────────────────┘     └────────┬────────┘      │
│                                                            │               │
│                                              ┌─────────────┴─────────────┐ │
│                                              ▼                           ▼ │
│                                       ┌────────────┐              ┌────────┐│
│                                       │ Dimensions │              │Exporter││
│                                       │ (维度解读)  │              │(导出器) ││
│                                       └────────────┘              └────────┘│
│                                                                            │
├────────────────────────────────────────────────────────────────────────────┤
│  核心设计模式：                                                            │
│    ✓ 聚合模式 - InterpretReport 管理维度和建议                            │
│    ✓ 建造者模式 - ReportBuilder 构建复杂报告                              │
│    ✓ 策略模式 - ReportExporter 支持多格式导出                             │
│    ✓ NoSQL存储 - MongoDB 适应灵活文档结构                                 │
└────────────────────────────────────────────────────────────────────────────┘
```

---

## 一、为什么需要 Report 聚合？（问题域）

### 1.1 业务场景：小明的测评报告

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│  场景：小明完成测评，需要一份可阅读、可打印的报告                          │
│                                                                             │
│  输入（来自 Interpretation）：                                              │
│    总分：68分，风险等级：中度                                              │
│    因子解读：                                                               │
│      - 精神情感症状：28分，高风险，"存在明显情感困扰"                      │
│      - 躯体症状：22分，中风险，"轻微躯体不适"                              │
│      - ...                                                                  │
│                                                                             │
│  问题：原始数据对用户不友好！                                               │
│    ❌ JSON 数据无法直接阅读                                                │
│    ❌ 没有结构化的报告格式                                                 │
│    ❌ 无法打印或分享                                                       │
│    ❌ 缺少个性化建议                                                       │
│                                                                             │
│  期望输出（一份完整的测评报告）：                                           │
│    ┌──────────────────────────────────────────────────────────────────┐    │
│    │                     心理健康测评报告                              │    │
│    │  ─────────────────────────────────────────────────────────────   │    │
│    │  量表：抑郁自评量表(SDS)                                         │    │
│    │  总分：68分    风险等级：🟠 中度风险                              │    │
│    │                                                                  │    │
│    │  【总体结论】                                                    │    │
│    │  测评结果显示存在中度抑郁倾向，建议进一步关注...                 │    │
│    │                                                                  │    │
│    │  【维度分析】                                                    │    │
│    │  ┌──────────────┬──────┬──────┬───────────────────────┐         │    │
│    │  │    维度      │ 得分 │ 等级 │         解读          │         │    │
│    │  ├──────────────┼──────┼──────┼───────────────────────┤         │    │
│    │  │ 精神情感症状 │  28  │ 🔴高 │ 存在明显情感困扰      │         │    │
│    │  │ 躯体症状     │  22  │ 🟡中 │ 轻微躯体不适          │         │    │
│    │  │ ...          │ ...  │ ...  │ ...                   │         │    │
│    │  └──────────────┴──────┴──────┴───────────────────────┘         │    │
│    │                                                                  │    │
│    │  【建议措施】                                                    │    │
│    │  1. 建议与心理咨询师预约面谈                                    │    │
│    │  2. 保持规律作息，适当运动                                      │    │
│    │  3. ...                                                         │    │
│    └──────────────────────────────────────────────────────────────────┘    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 1.2 问题拆解与解决方案

| 问题 | 挑战 | Report 的解决方案 |
|------|------|------------------|
| **结构化呈现** | 数据需要组织成可读格式 | **聚合根模式** - InterpretReport 统一管理 |
| **维度化展示** | 多因子需要分类展示 | **DimensionInterpret** - 维度值对象 |
| **个性化建议** | 根据风险生成建议 | **SuggestionGenerator** - 策略模式 |
| **多格式导出** | PDF/HTML/JSON 等 | **ReportExporter** - 策略模式 |
| **灵活存储** | 报告结构可能变化 | **MongoDB** - 文档型数据库 |

### 1.3 职责边界

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                        Report 聚合边界                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ✅ 我负责的：                        ❌ 我不关心的：                       │
│    • 组织报告结构（维度、建议）         • 分数如何计算（Calculation）       │
│    • 构建报告内容                       • 结果如何解读（Interpretation）    │
│    • 导出多种格式（PDF/HTML）           • 测评状态管理（Assessment）        │
│    • 持久化到 MongoDB                   • 得分如何存储（Score 存 MySQL）   │
│                                                                             │
│  与 Assessment 的关系：                                                     │
│    Assessment : InterpretReport = 1 : 1                                    │
│    Report.ID = Assessment.ID（相同标识）                                   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 二、在评估流程中的位置（是什么？）

### 2.1 评估流程全景图

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                          评估流程全景图                                     │
│                                                                             │
│   Phase 1: 数据准备                                                         │
│   ─────────────────────────────────────────────────────────────────────────│
│     AnswerSheet + MedicalScale ────▶ 准备就绪                              │
│                                                                             │
│                              ▼                                              │
│   Phase 2: 分数计算 (Calculation)                                          │
│   ─────────────────────────────────────────────────────────────────────────│
│     答案 + 计算策略 ────▶ 因子得分列表                                     │
│                                                                             │
│                              ▼                                              │
│   Phase 3: 结果解读 (Interpretation)                                       │
│   ─────────────────────────────────────────────────────────────────────────│
│     因子得分 + 解读规则 ────▶ 风险等级 + 结论                              │
│                                                                             │
│                              ▼                                              │
│   Phase 4: 报告生成 ◀─────── 【你在这里：Report】                          │
│   ─────────────────────────────────────────────────────────────────────────│
│                                                                             │
│     ┌─────────────────────────────────────────────────────────────────┐    │
│     │  ReportBuilder                                                  │    │
│     │    ├─ 接收: EvaluationResult (总分、风险、因子解读)            │    │
│     │    ├─ 构建: InterpretReport (聚合根)                           │    │
│     │    │        ├─ 维度解读列表 (DimensionInterpret[])             │    │
│     │    │        └─ 建议列表 (Suggestions[])                        │    │
│     │    └─ 存储: MongoDB                                            │    │
│     └─────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│                              ▼                                              │
│   Phase 5: 报告导出 (可选)                                                 │
│   ─────────────────────────────────────────────────────────────────────────│
│     InterpretReport ────▶ ReportExporter ────▶ PDF / HTML / JSON           │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 Report 内部结构

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                          report/（聚合）                                    │
│                                                                             │
│   ┌─────────────────────────────────────────────────────────────────────┐  │
│   │  report.go                          聚合根：InterpretReport          │  │
│   │  ┌───────────────────────────────────────────────────────────────┐  │  │
│   │  │  InterpretReport {                                            │  │  │
│   │  │      id           ID              // 与 AssessmentID 一致     │  │  │
│   │  │      scaleName    string          // 量表名称                 │  │  │
│   │  │      scaleCode    string          // 量表编码                 │  │  │
│   │  │      totalScore   float64         // 总分                     │  │  │
│   │  │      riskLevel    RiskLevel       // 风险等级                 │  │  │
│   │  │      conclusion   string          // 总体结论                 │  │  │
│   │  │      dimensions   []Dimension...  // 维度解读列表             │  │  │
│   │  │      suggestions  []string        // 建议列表                 │  │  │
│   │  │  }                                                            │  │  │
│   │  └───────────────────────────────────────────────────────────────┘  │  │
│   └─────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
│   ┌────────────────┬────────────────┬────────────────┬────────────────┐   │
│   │  dimension.go  │  suggestion.go │   builder.go   │  exporter.go   │   │
│   │  维度值对象    │   建议生成器   │   报告构建器   │   报告导出器   │   │
│   └────────────────┴────────────────┴────────────────┴────────────────┘   │
│                                                                             │
│   ┌────────────────────────────────────────────────────────────────────┐   │
│   │  types.go                              类型定义                    │   │
│   │  repository.go                         仓储接口                    │   │
│   └────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 三、聚合根设计（InterpretReport）

### 3.1 聚合结构图

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                      InterpretReport 聚合结构                               │
│                                                                             │
│   ┌─────────────────────────────────────────────────────────────────────┐  │
│   │                    InterpretReport (聚合根)                         │  │
│   │  ┌───────────────────────────────────────────────────────────────┐  │  │
│   │  │  id: 12345                                                    │  │  │
│   │  │  scaleName: "抑郁自评量表(SDS)"                               │  │  │
│   │  │  totalScore: 68.0                                             │  │  │
│   │  │  riskLevel: "medium"                                          │  │  │
│   │  │  conclusion: "测评结果显示存在中度抑郁倾向..."               │  │  │
│   │  └───────────────────────────────────────────────────────────────┘  │  │
│   │                              │                                       │  │
│   │              ┌───────────────┴───────────────┐                       │  │
│   │              ▼                               ▼                       │  │
│   │  ┌─────────────────────────┐   ┌─────────────────────────┐         │  │
│   │  │ dimensions (值对象列表) │   │ suggestions (字符串列表) │         │  │
│   │  │ ┌─────────────────────┐ │   │ ┌─────────────────────┐ │         │  │
│   │  │ │ DimensionInterpret  │ │   │ │ "建议咨询心理专家"  │ │         │  │
│   │  │ │ factorCode: "F1"    │ │   │ │ "保持规律作息"      │ │         │  │
│   │  │ │ rawScore: 28        │ │   │ │ "适当户外运动"      │ │         │  │
│   │  │ │ riskLevel: "high"   │ │   │ └─────────────────────┘ │         │  │
│   │  │ │ description: "..."  │ │   └─────────────────────────┘         │  │
│   │  │ └─────────────────────┘ │                                        │  │
│   │  │ ┌─────────────────────┐ │                                        │  │
│   │  │ │ DimensionInterpret  │ │                                        │  │
│   │  │ │ factorCode: "F2"    │ │                                        │  │
│   │  │ │ ...                 │ │                                        │  │
│   │  │ └─────────────────────┘ │                                        │  │
│   │  └─────────────────────────┘                                        │  │
│   └─────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
│   存储位置：MongoDB（文档型，适应灵活结构）                                 │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 3.2 核心代码

```go
// 代码位置：internal/apiserver/domain/evaluation/report/report.go

// InterpretReport 解读报告聚合根
type InterpretReport struct {
    id          ID                    // 与 AssessmentID 一致
    scaleName   string                // 量表名称
    scaleCode   string                // 量表编码
    totalScore  float64               // 总分
    riskLevel   RiskLevel             // 风险等级
    conclusion  string                // 总体结论
    dimensions  []DimensionInterpret  // 维度解读列表
    suggestions []string              // 建议列表
    createdAt   time.Time
    updatedAt   *time.Time
}

// NewInterpretReport 创建解读报告
func NewInterpretReport(
    id ID,
    scaleName, scaleCode string,
    totalScore float64,
    riskLevel RiskLevel,
    conclusion string,
    dimensions []DimensionInterpret,
    suggestions []string,
) *InterpretReport {
    return &InterpretReport{
        id: id, scaleName: scaleName, scaleCode: scaleCode,
        totalScore: totalScore, riskLevel: riskLevel,
        conclusion: conclusion, dimensions: dimensions,
        suggestions: suggestions, createdAt: time.Now(),
    }
}
```

> 📎 **完整代码**：[report.go](../../../internal/apiserver/domain/evaluation/report/report.go)

### 3.3 DimensionInterpret 值对象

```go
// DimensionInterpret 维度解读值对象
type DimensionInterpret struct {
    factorCode  FactorCode  // 因子编码
    factorName  string      // 因子名称
    rawScore    float64     // 原始得分
    riskLevel   RiskLevel   // 风险等级
    description string      // 解读描述
}

// IsHighRisk 是否高风险
func (d DimensionInterpret) IsHighRisk() bool {
    return d.riskLevel == RiskLevelHigh || d.riskLevel == RiskLevelSevere
}
```

> 📎 **完整代码**：[dimension.go](../../../internal/apiserver/domain/evaluation/report/dimension.go)

---

## 四、建造者模式（ReportBuilder）

### 4.1 构建流程

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                        ReportBuilder 构建流程                               │
│                                                                             │
│   输入：                                                                    │
│     ├─ Assessment (测评实体)                                               │
│     ├─ MedicalScale (量表实体)                                             │
│     └─ EvaluationResult (评估结果)                                         │
│                                                                             │
│   ┌─────────────────────────────────────────────────────────────────────┐  │
│   │  ReportBuilder.Build()                                              │  │
│   │                                                                     │  │
│   │  Step 1: 提取基础信息                                               │  │
│   │    ├─ reportID ← assessment.ID                                     │  │
│   │    ├─ scaleName ← medicalScale.Title                               │  │
│   │    └─ scaleCode ← medicalScale.Code                                │  │
│   │                                                                     │  │
│   │  Step 2: 构建总体结论                                               │  │
│   │    └─ conclusion ← buildConclusion(result.RiskLevel)               │  │
│   │                                                                     │  │
│   │  Step 3: 构建维度解读                                               │  │
│   │    └─ for each FactorScore in result:                              │  │
│   │          dimensions.append(DimensionInterpret{...})                │  │
│   │                                                                     │  │
│   │  Step 4: 生成建议                                                   │  │
│   │    └─ suggestions ← buildSuggestions(result)                       │  │
│   │                                                                     │  │
│   │  Step 5: 组装报告                                                   │  │
│   │    └─ return NewInterpretReport(...)                               │  │
│   │                                                                     │  │
│   └─────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
│   输出：*InterpretReport                                                    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 4.2 核心代码

```go
// 代码位置：internal/apiserver/domain/evaluation/report/builder.go

// ReportBuilder 报告构建器接口
type ReportBuilder interface {
    Build(
        assess *assessment.Assessment,
        medicalScale *scale.MedicalScale,
        evaluationResult *assessment.EvaluationResult,
    ) (*InterpretReport, error)
}

// DefaultReportBuilder 默认实现
type DefaultReportBuilder struct{}

func (b *DefaultReportBuilder) Build(
    assess *assessment.Assessment,
    medicalScale *scale.MedicalScale,
    result *assessment.EvaluationResult,
) (*InterpretReport, error) {
    
    // 1. 转换 ID
    reportID := ID(assess.ID())
    
    // 2. 构建总体结论
    conclusion := b.buildConclusion(medicalScale, result)
    
    // 3. 构建维度解读
    dimensions := b.buildDimensions(medicalScale, result)
    
    // 4. 生成建议
    suggestions := b.buildInitialSuggestions(result)
    
    // 5. 创建报告
    return NewInterpretReport(
        reportID,
        medicalScale.GetTitle(),
        medicalScale.GetCode().String(),
        result.TotalScore,
        RiskLevel(result.RiskLevel),
        conclusion,
        dimensions,
        suggestions,
    ), nil
}
```

> 📎 **完整代码**：[builder.go](../../../internal/apiserver/domain/evaluation/report/builder.go)

---

## 五、策略模式（ReportExporter）

### 5.1 导出器设计

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                        ReportExporter 策略模式                              │
│                                                                             │
│              ┌─────────────────────────┐                                    │
│              │    <<interface>>        │                                    │
│              │    ReportExporter       │                                    │
│              ├─────────────────────────┤                                    │
│              │ +Export(report, format) │                                    │
│              │ +SupportedFormats()     │                                    │
│              └───────────┬─────────────┘                                    │
│                          │                                                  │
│          ┌───────────────┼───────────────┐                                  │
│          │               │               │                                  │
│          ▼               ▼               ▼                                  │
│   ┌────────────┐  ┌────────────┐  ┌────────────┐                           │
│   │PDFExporter │  │HTMLExporter│  │JSONExporter│                           │
│   ├────────────┤  ├────────────┤  ├────────────┤                           │
│   │ 生成PDF    │  │ 生成HTML   │  │ 生成JSON   │                           │
│   │ (模板渲染) │  │ (模板渲染) │  │ (序列化)   │                           │
│   └────────────┘  └────────────┘  └────────────┘                           │
│                                                                             │
│   导出格式：                                                                │
│     ┌──────────┬─────────────────────────────────────────────────────────┐ │
│     │   格式   │                     用途                                │ │
│     ├──────────┼─────────────────────────────────────────────────────────┤ │
│     │   PDF    │ 正式报告，打印存档                                      │ │
│     │   HTML   │ 在线预览，邮件发送                                      │ │
│     │   JSON   │ API 返回，前端渲染                                      │ │
│     └──────────┴─────────────────────────────────────────────────────────┘ │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 5.2 接口定义

```go
// ReportExporter 报告导出器接口
type ReportExporter interface {
    // Export 导出报告
    Export(ctx context.Context, report *InterpretReport, 
           format ExportFormat, options ExportOptions) (io.Reader, error)
    
    // SupportedFormats 支持的格式列表
    SupportedFormats() []ExportFormat
}

// ExportFormat 导出格式
type ExportFormat string

const (
    ExportFormatPDF  ExportFormat = "pdf"
    ExportFormatHTML ExportFormat = "html"
    ExportFormatJSON ExportFormat = "json"
)

// ExportOptions 导出选项
type ExportOptions struct {
    TemplateID          string            // 自定义模板
    IncludeSuggestions  bool              // 包含建议
    IncludeDimensions   bool              // 包含维度
    IncludeCharts       bool              // 包含图表
    Metadata            map[string]string // 自定义元数据
}
```

> 📎 **完整代码**：[exporter.go](../../../internal/apiserver/domain/evaluation/report/exporter.go)

---

## 六、建议生成器（SuggestionGenerator）

### 6.1 设计思路

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                      SuggestionGenerator 策略链                             │
│                                                                             │
│   ┌─────────────────────────────────────────────────────────────────────┐  │
│   │  RuleBasedSuggestionGenerator                                       │  │
│   │                                                                     │  │
│   │  strategies = [                                                     │  │
│   │    HighRiskStrategy,      // 高风险专属建议                         │  │
│   │    DepressionStrategy,    // 抑郁相关建议                           │  │
│   │    AnxietyStrategy,       // 焦虑相关建议                           │  │
│   │    GeneralStrategy,       // 通用健康建议                           │  │
│   │  ]                                                                  │  │
│   │                                                                     │  │
│   │  for each strategy:                                                 │  │
│   │    if strategy.CanHandle(report):                                  │  │
│   │      suggestions += strategy.GenerateSuggestions(report)           │  │
│   │                                                                     │  │
│   └─────────────────────────────────────────────────────────────────────┘  │
│                                                                             │
│   示例输出：                                                                │
│     高风险因子"精神情感症状"触发：                                         │
│       → "建议尽快预约心理咨询师进行面谈"                                   │
│       → "如有自伤想法，请立即拨打心理援助热线"                             │
│     中风险触发：                                                           │
│       → "建议保持规律作息"                                                 │
│       → "适当进行户外运动"                                                 │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 6.2 核心代码

```go
// SuggestionGenerator 建议生成器接口
type SuggestionGenerator interface {
    Generate(ctx context.Context, report *InterpretReport) ([]string, error)
}

// SuggestionStrategy 建议策略接口
type SuggestionStrategy interface {
    Name() string
    CanHandle(report *InterpretReport) bool
    GenerateSuggestions(ctx context.Context, report *InterpretReport) ([]string, error)
}

// RuleBasedSuggestionGenerator 基于规则的建议生成器
type RuleBasedSuggestionGenerator struct {
    strategies []SuggestionStrategy
}

func (g *RuleBasedSuggestionGenerator) Generate(ctx context.Context, report *InterpretReport) ([]string, error) {
    var allSuggestions []string
    
    for _, strategy := range g.strategies {
        if strategy.CanHandle(report) {
            suggestions, _ := strategy.GenerateSuggestions(ctx, report)
            allSuggestions = append(allSuggestions, suggestions...)
        }
    }
    return allSuggestions, nil
}
```

> 📎 **完整代码**：[suggestion.go](../../../internal/apiserver/domain/evaluation/report/suggestion.go)

---

## 七、存储设计（MySQL vs MongoDB）

### 7.1 双存储策略

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                          评估数据存储策略                                   │
│                                                                             │
│   ┌────────────────────────────────────┬────────────────────────────────┐  │
│   │           MySQL                    │           MongoDB              │  │
│   ├────────────────────────────────────┼────────────────────────────────┤  │
│   │                                    │                                │  │
│   │  AssessmentScore                   │  InterpretReport               │  │
│   │  ┌───────────────────────┐         │  ┌───────────────────────┐    │  │
│   │  │ assessment_id (FK)    │         │  │ _id (= assessment_id) │    │  │
│   │  │ factor_code           │         │  │ scale_name            │    │  │
│   │  │ raw_score             │         │  │ total_score           │    │  │
│   │  │ risk_level            │         │  │ risk_level            │    │  │
│   │  │ is_total_score        │         │  │ conclusion            │    │  │
│   │  └───────────────────────┘         │  │ dimensions: [...]     │    │  │
│   │                                    │  │ suggestions: [...]    │    │  │
│   │  适合场景：                        │  └───────────────────────┘    │  │
│   │  ✓ SQL 聚合查询                    │                                │  │
│   │  ✓ 趋势分析 (GROUP BY)             │  适合场景：                    │  │
│   │  ✓ 跨测评统计                      │  ✓ 灵活的文档结构              │  │
│   │                                    │  ✓ 嵌套数据（维度、建议）      │  │
│   │                                    │  ✓ 频繁读取完整报告            │  │
│   │                                    │                                │  │
│   └────────────────────────────────────┴────────────────────────────────┘  │
│                                                                             │
│   设计原理：                                                                │
│     • Score 存 MySQL：支持 "小明最近10次测评的抑郁因子趋势" 这类查询       │
│     • Report 存 MongoDB：一次读取完整报告，无需多表 JOIN                   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 7.2 MongoDB 文档结构

```json
{
  "_id": "12345",
  "scale_name": "抑郁自评量表(SDS)",
  "scale_code": "SDS",
  "total_score": 68.0,
  "risk_level": "medium",
  "conclusion": "测评结果显示存在中度抑郁倾向...",
  "dimensions": [
    {
      "factor_code": "F1",
      "factor_name": "精神情感症状",
      "raw_score": 28,
      "risk_level": "high",
      "description": "存在明显情感困扰，建议咨询"
    },
    {
      "factor_code": "F2",
      "factor_name": "躯体症状",
      "raw_score": 22,
      "risk_level": "medium",
      "description": "轻微躯体不适，注意休息"
    }
  ],
  "suggestions": [
    "建议与心理咨询师预约面谈",
    "保持规律作息，适当运动",
    "避免长时间独处"
  ],
  "created_at": "2025-01-29T10:30:00Z"
}
```

---

## 八、运行时调用流程

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                        报告生成时序图                                       │
│                                                                             │
│  EvaluationService    ReportBuilder    SuggestionGen    ReportRepo         │
│        │                   │                │               │               │
│        │  1. Build()       │                │               │               │
│        │──────────────────▶│                │               │               │
│        │                   │                │               │               │
│        │  2. 构建维度解读  │                │               │               │
│        │                   │────────┐       │               │               │
│        │                   │        │       │               │               │
│        │                   │◀───────┘       │               │               │
│        │                   │                │               │               │
│        │  3. 生成建议      │                │               │               │
│        │                   │───────────────▶│               │               │
│        │                   │                │               │               │
│        │                   │◀───────────────│               │               │
│        │                   │   suggestions  │               │               │
│        │                   │                │               │               │
│        │  4. 返回报告      │                │               │               │
│        │◀──────────────────│                │               │               │
│        │  InterpretReport  │                │               │               │
│        │                   │                │               │               │
│        │  5. 持久化到 MongoDB                               │               │
│        │────────────────────────────────────────────────────▶               │
│        │                                                    │               │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 九、设计思想总结

### 9.1 核心设计决策

| 设计决策 | 选择 | 理由 |
|---------|------|------|
| **聚合根** | InterpretReport | 统一管理维度和建议的生命周期 |
| **建造者模式** | ReportBuilder | 封装复杂报告的构建过程 |
| **策略模式** | Exporter/Suggestion | 支持多格式导出、多策略建议 |
| **MongoDB 存储** | 文档型存储 | 适应灵活的报告结构，一次读取 |
| **ID 一致性** | Report.ID = Assessment.ID | 简化关联，避免额外外键 |

### 9.2 与其他模块的协作

```text
┌─────────────────────────────────────────────────────────────────────────────┐
│                        Report 协作关系图                                    │
│                                                                             │
│            ┌──────────────────┐                                             │
│            │  Interpretation  │                                             │
│            │   (解读结果)      │                                             │
│            └────────┬─────────┘                                             │
│                     │                                                       │
│                     ▼                                                       │
│            ┌──────────────────┐                                             │
│            │  ReportBuilder   │                                             │
│            │   (构建报告)      │                                             │
│            └────────┬─────────┘                                             │
│                     │                                                       │
│                     ▼                                                       │
│            ┌──────────────────┐                                             │
│            │ InterpretReport  │                                             │
│            │   (聚合根)        │                                             │
│            └────────┬─────────┘                                             │
│                     │                                                       │
│         ┌───────────┴───────────┐                                           │
│         ▼                       ▼                                           │
│  ┌────────────────┐     ┌────────────────┐                                  │
│  │ MongoDB 存储   │     │ ReportExporter │                                  │
│  │  (持久化)      │     │  (导出PDF等)   │                                  │
│  └────────────────┘     └────────────────┘                                  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 十、代码索引

| 文件 | 职责 | 路径 |
|------|------|------|
| `report.go` | 聚合根：InterpretReport | `internal/apiserver/domain/evaluation/report/` |
| `dimension.go` | 值对象：DimensionInterpret | 同上 |
| `builder.go` | 领域服务：ReportBuilder | 同上 |
| `exporter.go` | 领域服务：ReportExporter | 同上 |
| `suggestion.go` | 领域服务：SuggestionGenerator | 同上 |
| `types.go` | 类型定义 | 同上 |
| `repository.go` | 仓储接口 | 同上 |

---

## 📖 延伸阅读

- **前一篇**：[11-06-04 Interpretation解读策略设计](./11-06-04-Interpretation解读策略设计.md) - 理解解读结果从何而来
- **后一篇**：[11-06-06 应用服务层设计](./11-06-06-应用服务层设计.md) - 学习如何编排整个评估流程
- **设计模式**：建造者模式、策略模式、聚合模式
