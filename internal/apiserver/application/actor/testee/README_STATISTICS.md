# TesteeStatisticsService 实现说明

## 概述

`TesteeStatisticsService` 是受试者统计服务，提供受试者的测评数据统计和分析能力。

## 服务结构

```go
type statisticsService struct {
    testeeRepo     domain.Repository          // 受试者仓储
    assessmentRepo assessment.Repository      // 测评仓储
    scoreRepo      assessment.ScoreRepository // 得分仓储
    reportRepo     report.ReportRepository    // 报告仓储
}
```

## 核心方法

### 1. GetScaleAnalysis - 量表趋势分析

获取受试者在各个量表上的历史得分变化，用于绘制趋势图表和分析干预效果。

**场景**：管理员或数据分析系统查看受试者的量表趋势

**实现逻辑**：

1. 验证受试者是否存在
2. 查询受试者的所有测评记录
3. 过滤出已完成的测评（StatusInterpreted）
4. 按量表 ID 分组
5. 对每个量表：
   - 获取测评的基本信息（时间、总分、风险等级）
   - 获取解读报告中的结果描述
   - 获取所有因子得分（排除总分因子）
6. 按时间升序排序每个量表的测评记录
7. 返回结构化的分析结果

**返回数据结构**：

```go
type ScaleAnalysisResult struct {
    TesteeID uint64
    Scales   []ScaleTrendAnalysis  // 按量表分组的趋势分析
}

type ScaleTrendAnalysis struct {
    ScaleID   uint64
    ScaleCode string
    ScaleName string
    Tests     []TestRecordData  // 按时间升序排列
}

type TestRecordData struct {
    AssessmentID uint64
    TestDate     time.Time
    TotalScore   float64
    RiskLevel    string
    Result       string
    Factors      []FactorScoreData
}
```

### 2. GetPeriodicStats - 周期性测评统计

获取受试者参与的周期性测评项目统计，用于监控长期干预计划的执行情况。

**场景**：管理员查看受试者在周期性项目中的完成进度

**实现逻辑**：

1. 验证受试者是否存在
2. 查询受试者的所有测评记录
3. 筛选出来源为测评计划（OriginPlan）的测评
4. 按 planID 分组
5. 对每个项目：
   - 按时间排序测评记录
   - 统计完成情况（已完成周数、总周数、完成率）
   - 判断项目是否活跃（有未完成任务）
   - 构建每周任务状态列表
   - 计算当前应完成的周次
6. 返回结构化的统计结果

**返回数据结构**：

```go
type PeriodicStatsResult struct {
    TesteeID       uint64
    Projects       []PeriodicProjectStats
    TotalProjects  int  // 项目总数
    ActiveProjects int  // 进行中的项目数
}

type PeriodicProjectStats struct {
    ProjectID      uint64
    ProjectName    string
    ScaleName      string
    TotalWeeks     int
    CompletedWeeks int
    CompletionRate float64  // 完成率 0-100
    CurrentWeek    int      // 当前应完成的周次
    Tasks          []PeriodicTask
    StartDate      *time.Time
    EndDate        *time.Time
}

type PeriodicTask struct {
    Week         int
    Status       string  // completed/pending/overdue
    CompletedAt  *time.Time
    DueDate      *time.Time
    AssessmentID *uint64
}
```

## 依赖关系

- **domain.Repository**: 查询受试者信息
- **assessment.Repository**: 查询测评记录
- **assessment.ScoreRepository**: 查询因子得分
- **report.ReportRepository**: 查询解读报告

## 使用示例

```go
// 创建服务实例
statsService := testee.NewStatisticsService(
    testeeRepo,
    assessmentRepo,
    scoreRepo,
    reportRepo,
)

// 获取量表趋势分析
scaleAnalysis, err := statsService.GetScaleAnalysis(ctx, testeeID)
if err != nil {
    // 处理错误
}

// 获取周期性测评统计
periodicStats, err := statsService.GetPeriodicStats(ctx, testeeID)
if err != nil {
    // 处理错误
}
```

## 注意事项

1. **性能考虑**：当前使用分页参数 1000 来获取全量数据，适合数据量不大的情况。如果数据量很大，需要优化查询策略。

2. **TODO 项**：
   - `PeriodicProjectStats.ProjectID`: 当前设置为 0，需要从 plan 领域获取真实的项目 ID
   - T 分和百分位字段当前未实现，预留在 `FactorScoreData` 中

3. **数据完整性**：
   - 只统计已完成（StatusInterpreted）的测评
   - 只处理关联了量表的测评（`HasMedicalScale()`）
   - 测评失败的记录在周期性统计中标记为 "overdue"

4. **错误处理**：
   - 受试者不存在返回 `code.ErrUserNotFound`
   - 其他错误用 `errors.Wrap` 包装并返回

## 扩展建议

1. **缓存优化**：对于频繁访问的统计数据可以考虑添加缓存
2. **异步计算**：对于复杂的统计分析可以考虑异步计算并存储结果
3. **更多维度**：可以扩展更多统计维度，如按时间段统计、按风险等级统计等
4. **趋势分析**：可以添加更高级的趋势分析算法，如移动平均、线性回归等
