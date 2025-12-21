package plan

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
)

// TaskGenerator 任务生成器
// 负责根据计划的周期策略生成测评任务
type TaskGenerator struct{}

// NewTaskGenerator 创建任务生成器
func NewTaskGenerator() *TaskGenerator {
	return &TaskGenerator{}
}

// GenerateTasks 根据计划生成所有任务（一次性生成全部）
//
// 适用场景：测评次数不多（≤ 50 次）
// 生成时机：受试者加入计划时立即生成所有 Task
//
// 参数：
//   - plan: 测评计划模板（包含周期策略）
//   - testeeID: 受试者ID
//   - startDate: 基准日期，所有相对时间都基于此日期计算
func (g *TaskGenerator) GenerateTasks(plan *AssessmentPlan, testeeID testee.ID, startDate time.Time) []*AssessmentTask {
	var tasks []*AssessmentTask

	switch plan.GetScheduleType() {
	case PlanScheduleByWeek:
		// 每 N 周一次
		for i := 0; i < plan.GetTotalTimes(); i++ {
			plannedAt := startDate.AddDate(0, 0, i*plan.GetInterval()*7)
			task := NewAssessmentTask(
				plan.GetID(),
				i+1,
				plan.GetOrgID(),
				testeeID,
				plan.GetScaleCode(),
				plannedAt,
			)
			tasks = append(tasks, task)
		}

	case PlanScheduleByDay:
		// 每 N 天一次
		for i := 0; i < plan.GetTotalTimes(); i++ {
			plannedAt := startDate.AddDate(0, 0, i*plan.GetInterval())
			task := NewAssessmentTask(
				plan.GetID(),
				i+1,
				plan.GetOrgID(),
				testeeID,
				plan.GetScaleCode(),
				plannedAt,
			)
			tasks = append(tasks, task)
		}

	case PlanScheduleCustom:
		// 相对周次（如 [2,4,8,12,18]）
		// 每个周次都是相对于 startDate 的偏移
		relativeWeeks := plan.GetRelativeWeeks()
		for i, week := range relativeWeeks {
			plannedAt := startDate.AddDate(0, 0, week*7)
			task := NewAssessmentTask(
				plan.GetID(),
				i+1,
				plan.GetOrgID(),
				testeeID,
				plan.GetScaleCode(),
				plannedAt,
			)
			tasks = append(tasks, task)
		}

	case PlanScheduleFixedDate:
		// 固定日期列表
		fixedDates := plan.GetFixedDates()
		for i, date := range fixedDates {
			task := NewAssessmentTask(
				plan.GetID(),
				i+1,
				plan.GetOrgID(),
				testeeID,
				plan.GetScaleCode(),
				date,
			)
			tasks = append(tasks, task)
		}
	}

	return tasks
}

// GenerateTasksUntil 生成直到指定日期之前的任务（用于定时生成场景）
//
// 适用场景：计划跨度很大（数年）或次数很多
// 生成时机：定时任务扫描计划，生成未来 N 天内的 Task
//
// 参数：
//   - plan: 测评计划模板（包含周期策略）
//   - testeeID: 受试者ID
//   - startDate: 基准日期，所有相对时间都基于此日期计算
//   - endDate: 截止日期，只生成此日期之前的任务
func (g *TaskGenerator) GenerateTasksUntil(plan *AssessmentPlan, testeeID testee.ID, startDate time.Time, endDate time.Time) []*AssessmentTask {
	var tasks []*AssessmentTask
	seq := 1

	switch plan.GetScheduleType() {
	case PlanScheduleByWeek:
		// 每 N 周一次
		currentDate := startDate
		for currentDate.Before(endDate) && seq <= plan.GetTotalTimes() {
			task := NewAssessmentTask(
				plan.GetID(),
				seq,
				plan.GetOrgID(),
				testeeID,
				plan.GetScaleCode(),
				currentDate,
			)
			tasks = append(tasks, task)
			currentDate = currentDate.AddDate(0, 0, plan.GetInterval()*7)
			seq++
		}

	case PlanScheduleByDay:
		// 每 N 天一次
		currentDate := startDate
		for currentDate.Before(endDate) && seq <= plan.GetTotalTimes() {
			task := NewAssessmentTask(
				plan.GetID(),
				seq,
				plan.GetOrgID(),
				testeeID,
				plan.GetScaleCode(),
				currentDate,
			)
			tasks = append(tasks, task)
			currentDate = currentDate.AddDate(0, 0, plan.GetInterval())
			seq++
		}

	case PlanScheduleCustom:
		// 相对周次（如 [2,4,8,12,18]）
		// 每个周次都是相对于 startDate 的偏移
		relativeWeeks := plan.GetRelativeWeeks()
		for _, week := range relativeWeeks {
			plannedAt := startDate.AddDate(0, 0, week*7)
			if plannedAt.Before(endDate) || plannedAt.Equal(endDate) {
				task := NewAssessmentTask(
					plan.GetID(),
					seq,
					plan.GetOrgID(),
					testeeID,
					plan.GetScaleCode(),
					plannedAt,
				)
				tasks = append(tasks, task)
				seq++
			}
		}

	case PlanScheduleFixedDate:
		// 固定日期列表
		fixedDates := plan.GetFixedDates()
		for _, date := range fixedDates {
			if date.Before(endDate) || date.Equal(endDate) {
				task := NewAssessmentTask(
					plan.GetID(),
					seq,
					plan.GetOrgID(),
					testeeID,
					plan.GetScaleCode(),
					date,
				)
				tasks = append(tasks, task)
				seq++
			}
		}
	}

	return tasks
}

// FilterNewTasks 过滤出新的任务（排除已存在的任务）
func FilterNewTasks(newTasks []*AssessmentTask, existingTasks []*AssessmentTask) []*AssessmentTask {
	if len(existingTasks) == 0 {
		return newTasks
	}

	// 构建已存在任务的映射（按 planID + seq）
	existingMap := make(map[string]bool)
	for _, task := range existingTasks {
		key := task.GetPlanID().String() + "_" + string(rune(task.GetSeq()))
		existingMap[key] = true
	}

	// 过滤新任务
	var result []*AssessmentTask
	for _, task := range newTasks {
		key := task.GetPlanID().String() + "_" + string(rune(task.GetSeq()))
		if !existingMap[key] {
			result = append(result, task)
		}
	}

	return result
}

// Helper function for creating tasks with testee and scale code
// This is a convenience function that can be used when you have the code directly
func GenerateTasksWithIDs(
	planID AssessmentPlanID,
	orgID int64,
	testeeID testee.ID,
	scaleCode string,
	scheduleType PlanScheduleType,
	interval int,
	totalTimes int,
	startDate time.Time,
	fixedDates []time.Time,
	customWeeks []int,
) []*AssessmentTask {
	// Create a temporary plan-like structure for generation
	// Note: This is a helper function, actual plan creation should go through NewAssessmentPlan
	var tasks []*AssessmentTask

	switch scheduleType {
	case PlanScheduleByWeek:
		for i := 0; i < totalTimes; i++ {
			plannedAt := startDate.AddDate(0, 0, i*interval*7)
			task := NewAssessmentTask(planID, i+1, orgID, testeeID, scaleCode, plannedAt)
			tasks = append(tasks, task)
		}

	case PlanScheduleByDay:
		for i := 0; i < totalTimes; i++ {
			plannedAt := startDate.AddDate(0, 0, i*interval)
			task := NewAssessmentTask(planID, i+1, orgID, testeeID, scaleCode, plannedAt)
			tasks = append(tasks, task)
		}

	case PlanScheduleCustom:
		for i, week := range customWeeks {
			// 相对周次：相对于 startDate 的偏移
			plannedAt := startDate.AddDate(0, 0, week*7)
			task := NewAssessmentTask(planID, i+1, orgID, testeeID, scaleCode, plannedAt)
			tasks = append(tasks, task)
		}

	case PlanScheduleFixedDate:
		for i, date := range fixedDates {
			task := NewAssessmentTask(planID, i+1, orgID, testeeID, scaleCode, date)
			tasks = append(tasks, task)
		}
	}

	return tasks
}
