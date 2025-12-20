package plan

import (
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
	ScaleID       string   // 量表ID
	ScheduleType  string   // 周期类型
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
	ScaleID      string  // 量表ID
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
	PlanID string        // 计划ID
	Tasks  []*TaskResult // 生成的任务列表
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
		ScaleID:       p.GetScaleID().String(),
		ScheduleType:  string(p.GetScheduleType()),
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
		ScaleID:    t.GetScaleID().String(),
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
		return time.Time{}, nil
	}
	// 尝试多种格式
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
		"2006-01-02",
	}
	for _, format := range formats {
		if t, err := time.Parse(format, timeStr); err == nil {
			return t, nil
		}
	}
	return time.Time{}, nil
}

// parseDate 解析日期字符串
func parseDate(dateStr string) (time.Time, error) {
	if dateStr == "" {
		return time.Time{}, nil
	}
	return time.Parse("2006-01-02", dateStr)
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
		return meta.ID(0), nil
	}
	return testee.ID(id), nil
}

// toScaleID 转换为量表ID
func toScaleID(s string) (meta.ID, error) {
	return meta.ParseID(s)
}

// toPlanID 转换为计划ID
func toPlanID(s string) (plan.AssessmentPlanID, error) {
	return plan.ParseAssessmentPlanID(s)
}

// toTaskID 转换为任务ID
func toTaskID(s string) (plan.AssessmentTaskID, error) {
	return plan.ParseAssessmentTaskID(s)
}
