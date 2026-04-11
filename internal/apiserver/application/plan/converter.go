package plan

import (
	"fmt"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// ============= Result 定义 =============
// Results 用于应用服务层的输出结果

// PlanResult 计划结果
type PlanResult struct {
	ID            string   // 计划ID
	OrgID         int64    // 机构ID
	ScaleCode     string   // 量表编码
	ScheduleType  string   // 周期类型
	TriggerTime   string   // 触发时间
	Interval      int      // 间隔
	TotalTimes    int      // 总次数
	FixedDates    []string // 固定日期列表
	RelativeWeeks []int    // 相对周次列表
	Status        string   // 状态
}

// TaskResult 任务结果
type TaskResult struct {
	ID           string  // 任务ID
	PlanID       string  // 计划ID
	Seq          int     // 序号
	OrgID        int64   // 机构ID
	TesteeID     string  // 受试者ID
	ScaleCode    string  // 量表编码
	PlannedAt    string  // 计划时间点
	OpenAt       *string // 开放时间
	ExpireAt     *string // 截止时间
	CompletedAt  *string // 完成时间
	Status       string  // 状态
	AssessmentID *string // 关联的测评ID
	EntryToken   string  // 入口令牌
	EntryURL     string  // 入口URL
}

// EnrollmentResult 加入计划结果
type EnrollmentResult struct {
	PlanID           string        // 计划ID
	Tasks            []*TaskResult // 生成/复用后的任务列表
	Idempotent       bool          // 是否命中幂等路径
	CreatedTaskCount int           // 本次新建任务数量
}

// PlanMutationResult 计划写操作结果。
type PlanMutationResult struct {
	PlanID            string // 计划ID
	AffectedTaskCount int    // 本次联动影响的任务数量
}

// EnrollmentTerminationResult 终止参与结果。
type EnrollmentTerminationResult struct {
	PlanID            string // 计划ID
	TesteeID          string // 受试者ID
	AffectedTaskCount int    // 本次取消的任务数量
}

// TaskMutationResult 任务写操作结果。
type TaskMutationResult struct {
	TaskID            string // 任务ID
	PlanID            string // 计划ID
	AffectedTaskCount int    // 受影响记录数
}

// TaskScheduleResult 调度结果。
type TaskScheduleResult struct {
	Tasks []*TaskResult     // 本轮新开放的任务
	Stats TaskScheduleStats // 调度统计
}

// PlanListResult 计划列表结果
type PlanListResult struct {
	Items    []*PlanResult // 计划列表
	Total    int64         // 总数
	Page     int           // 当前页码
	PageSize int           // 每页数量
}

// TaskListResult 任务列表结果
type TaskListResult struct {
	Items    []*TaskResult // 任务列表
	Total    int64         // 总数
	Page     int           // 当前页码
	PageSize int           // 每页数量
}

// TaskWindowResult 任务窗口结果。
// 使用 has_more 表达是否存在下一页，避免 process 路径依赖 COUNT(*)。
type TaskWindowResult struct {
	Items    []*TaskResult // 当前窗口内的任务
	Page     int           // 当前页码
	PageSize int           // 当前页大小
	HasMore  bool          // 是否还有下一页
}

// ============= 转换函数 =============

// toPlanResult 将领域对象转换为结果对象
func toPlanResult(p *plan.AssessmentPlan) *PlanResult {
	if p == nil {
		return nil
	}

	fixedDates := make([]string, 0, len(p.GetFixedDates()))
	for _, date := range p.GetFixedDates() {
		fixedDates = append(fixedDates, date.Format("2006-01-02"))
	}

	return &PlanResult{
		ID:            p.GetID().String(),
		OrgID:         p.GetOrgID(),
		ScaleCode:     p.GetScaleCode(),
		ScheduleType:  string(p.GetScheduleType()),
		TriggerTime:   p.GetTriggerTime(),
		Interval:      p.GetInterval(),
		TotalTimes:    p.GetTotalTimes(),
		FixedDates:    fixedDates,
		RelativeWeeks: p.GetRelativeWeeks(),
		Status:        string(p.GetStatus()),
	}
}

// toTaskResult 将领域对象转换为结果对象
func toTaskResult(t *plan.AssessmentTask) *TaskResult {
	if t == nil {
		return nil
	}

	result := &TaskResult{
		ID:         t.GetID().String(),
		PlanID:     t.GetPlanID().String(),
		Seq:        t.GetSeq(),
		OrgID:      t.GetOrgID(),
		TesteeID:   t.GetTesteeID().String(),
		ScaleCode:  t.GetScaleCode(),
		PlannedAt:  t.GetPlannedAt().Format("2006-01-02 15:04:05"),
		Status:     string(t.GetStatus()),
		EntryToken: t.GetEntryToken(),
		EntryURL:   t.GetEntryURL(),
	}

	if openAt := t.GetOpenAt(); openAt != nil {
		openAtStr := openAt.Format("2006-01-02 15:04:05")
		result.OpenAt = &openAtStr
	}
	if expireAt := t.GetExpireAt(); expireAt != nil {
		expireAtStr := expireAt.Format("2006-01-02 15:04:05")
		result.ExpireAt = &expireAtStr
	}
	if completedAt := t.GetCompletedAt(); completedAt != nil {
		completedAtStr := completedAt.Format("2006-01-02 15:04:05")
		result.CompletedAt = &completedAtStr
	}
	if assessmentID := t.GetAssessmentID(); assessmentID != nil {
		assessmentIDStr := assessmentID.String()
		result.AssessmentID = &assessmentIDStr
	}

	return result
}

// toTaskResults 批量转换任务结果
func toTaskResults(tasks []*plan.AssessmentTask) []*TaskResult {
	if len(tasks) == 0 {
		return nil
	}
	results := make([]*TaskResult, 0, len(tasks))
	for _, task := range tasks {
		results = append(results, toTaskResult(task))
	}
	return results
}

// parseTime 解析时间字符串
func parseTime(timeStr string) (time.Time, error) {
	if timeStr == "" {
		return time.Now(), nil
	}
	// 尝试多种格式
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
		"2006-01-02",
	}
	for _, format := range formats {
		if t, err := time.ParseInLocation(format, timeStr, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported time format: %s", timeStr)
}

// parseDate 解析日期字符串
func parseDate(dateStr string) (time.Time, error) {
	if dateStr == "" {
		return time.Time{}, nil
	}
	return time.ParseInLocation("2006-01-02", dateStr, time.Local)
}

// toPlanScheduleType 转换为计划周期类型
func toPlanScheduleType(s string) plan.PlanScheduleType {
	switch s {
	case "by_week":
		return plan.PlanScheduleByWeek
	case "by_day":
		return plan.PlanScheduleByDay
	case "custom":
		return plan.PlanScheduleCustom
	case "fixed_date":
		return plan.PlanScheduleFixedDate
	default:
		return plan.PlanScheduleByWeek
	}
}

// toTesteeID 转换为受试者ID
func toTesteeID(s string) (testee.ID, error) {
	id, err := meta.ParseID(s)
	if err != nil {
		return meta.ID(0), err
	}
	return testee.ID(id), nil
}

// toPlanID 转换为计划ID
func toPlanID(s string) (plan.AssessmentPlanID, error) {
	return plan.ParseAssessmentPlanID(s)
}

// toTaskID 转换为任务ID
func toTaskID(s string) (plan.AssessmentTaskID, error) {
	return plan.ParseAssessmentTaskID(s)
}
