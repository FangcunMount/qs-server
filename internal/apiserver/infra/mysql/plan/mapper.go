package plan

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainPlan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// ==================== PlanMapper ====================

// PlanMapper 计划映射器
type PlanMapper struct{}

// NewPlanMapper 创建计划映射器
func NewPlanMapper() *PlanMapper {
	return &PlanMapper{}
}

// ToPO 将领域对象转换为持久化对象
func (m *PlanMapper) ToPO(domain *domainPlan.AssessmentPlan) *AssessmentPlanPO {
	if domain == nil {
		return nil
	}

	po := &AssessmentPlanPO{
		OrgID:        domain.GetOrgID(),
		ScaleCode:    domain.GetScaleCode(),
		ScheduleType: string(domain.GetScheduleType()),
		Interval:     domain.GetInterval(),
		TotalTimes:   domain.GetTotalTimes(),
		Status:       string(domain.GetStatus()),
	}

	// 设置ID（如果已存在）
	if !domain.GetID().IsZero() {
		po.ID = domain.GetID()
	}

	// 转换固定日期列表（time.Time -> string）
	fixedDates := domain.GetFixedDates()
	if len(fixedDates) > 0 {
		dateStrings := make([]string, len(fixedDates))
		for i, date := range fixedDates {
			dateStrings[i] = date.Format("2006-01-02")
		}
		po.FixedDates = StringSlice(dateStrings)
	}

	// 转换相对周次列表
	relativeWeeks := domain.GetRelativeWeeks()
	if len(relativeWeeks) > 0 {
		po.RelativeWeeks = IntSlice(relativeWeeks)
	}

	return po
}

// ToDomain 将持久化对象转换为领域对象
func (m *PlanMapper) ToDomain(po *AssessmentPlanPO) *domainPlan.AssessmentPlan {
	if po == nil {
		return nil
	}

	// 转换固定日期列表（string -> time.Time）
	var fixedDates []time.Time
	if len(po.FixedDates) > 0 {
		fixedDates = make([]time.Time, 0, len(po.FixedDates))
		for _, dateStr := range po.FixedDates {
			if date, err := time.Parse("2006-01-02", dateStr); err == nil {
				fixedDates = append(fixedDates, date)
			}
		}
	}

	// 转换相对周次列表
	var relativeWeeks []int
	if len(po.RelativeWeeks) > 0 {
		relativeWeeks = []int(po.RelativeWeeks)
	}

	// 构建选项
	var opts []domainPlan.PlanOption
	if len(fixedDates) > 0 {
		opts = append(opts, domainPlan.WithFixedDates(fixedDates))
	}
	if len(relativeWeeks) > 0 {
		opts = append(opts, domainPlan.WithRelativeWeeks(relativeWeeks))
	}

	// 创建领域对象
	plan, err := domainPlan.NewAssessmentPlan(
		po.OrgID,
		po.ScaleCode,
		domainPlan.PlanScheduleType(po.ScheduleType),
		po.Interval,
		po.TotalTimes,
		opts...,
	)
	if err != nil {
		// 如果创建失败，返回 nil（这种情况不应该发生，因为数据来自数据库）
		return nil
	}

	// 恢复ID和状态（使用 RestoreFromRepository）
	plan.RestoreFromRepository(
		meta.ID(po.ID),
		domainPlan.PlanStatus(po.Status),
	)

	return plan
}

// ToDomainList 批量转换
func (m *PlanMapper) ToDomainList(pos []*AssessmentPlanPO) []*domainPlan.AssessmentPlan {
	if len(pos) == 0 {
		return nil
	}
	domains := make([]*domainPlan.AssessmentPlan, 0, len(pos))
	for _, po := range pos {
		if domain := m.ToDomain(po); domain != nil {
			domains = append(domains, domain)
		}
	}
	return domains
}

// SyncID 同步ID（从PO到领域对象）
func (m *PlanMapper) SyncID(po *AssessmentPlanPO, domain *domainPlan.AssessmentPlan) {
	// 使用 RestoreFromRepository 同步 ID（但保持现有状态）
	// 或者直接调用 setID（但这是包内方法，不能跨包访问）
	// 实际上，SyncID 只在 Save 时使用，此时 domain 已经有 ID 了
	// 这里只是为了确保 ID 同步，如果 domain 没有 ID，则设置
	if domain.GetID().IsZero() {
		domain.RestoreFromRepository(meta.ID(po.ID), domain.GetStatus())
	}
}

// ==================== TaskMapper ====================

// TaskMapper 任务映射器
type TaskMapper struct{}

// NewTaskMapper 创建任务映射器
func NewTaskMapper() *TaskMapper {
	return &TaskMapper{}
}

// ToPO 将领域对象转换为持久化对象
func (m *TaskMapper) ToPO(domain *domainPlan.AssessmentTask) *AssessmentTaskPO {
	if domain == nil {
		return nil
	}

	po := &AssessmentTaskPO{
		PlanID:     domain.GetPlanID().Uint64(),
		Seq:        domain.GetSeq(),
		OrgID:      domain.GetOrgID(),
		TesteeID:   domain.GetTesteeID().Uint64(),
		ScaleCode:  domain.GetScaleCode(),
		PlannedAt:  domain.GetPlannedAt(),
		Status:     string(domain.GetStatus()),
		EntryToken: domain.GetEntryToken(),
		EntryURL:   domain.GetEntryURL(),
	}

	// 设置ID（如果已存在）
	if !domain.GetID().IsZero() {
		po.ID = domain.GetID()
	}

	// 可选字段
	if openAt := domain.GetOpenAt(); openAt != nil {
		po.OpenAt = openAt
	}
	if expireAt := domain.GetExpireAt(); expireAt != nil {
		po.ExpireAt = expireAt
	}
	if completedAt := domain.GetCompletedAt(); completedAt != nil {
		po.CompletedAt = completedAt
	}
	if assessmentID := domain.GetAssessmentID(); assessmentID != nil {
		assessmentIDUint64 := assessmentID.Uint64()
		po.AssessmentID = &assessmentIDUint64
	}

	return po
}

// ToDomain 将持久化对象转换为领域对象
func (m *TaskMapper) ToDomain(po *AssessmentTaskPO) *domainPlan.AssessmentTask {
	if po == nil {
		return nil
	}

	// 创建领域对象
	task := domainPlan.NewAssessmentTask(
		meta.FromUint64(po.PlanID),
		po.Seq,
		po.OrgID,
		testee.ID(meta.FromUint64(po.TesteeID)),
		po.ScaleCode,
		po.PlannedAt,
	)

	// 恢复可选字段
	var assessmentIDPtr *assessment.ID
	if po.AssessmentID != nil {
		assessmentID := assessment.ID(meta.FromUint64(*po.AssessmentID))
		assessmentIDPtr = &assessmentID
	}

	// 恢复ID和状态（使用 RestoreFromRepository）
	task.RestoreFromRepository(
		meta.ID(po.ID),
		domainPlan.TaskStatus(po.Status),
		po.OpenAt,
		po.ExpireAt,
		po.CompletedAt,
		assessmentIDPtr,
		po.EntryToken,
		po.EntryURL,
	)

	return task
}

// ToDomainList 批量转换
func (m *TaskMapper) ToDomainList(pos []*AssessmentTaskPO) []*domainPlan.AssessmentTask {
	if len(pos) == 0 {
		return nil
	}
	domains := make([]*domainPlan.AssessmentTask, 0, len(pos))
	for _, po := range pos {
		if domain := m.ToDomain(po); domain != nil {
			domains = append(domains, domain)
		}
	}
	return domains
}

// SyncID 同步ID（从PO到领域对象）
func (m *TaskMapper) SyncID(po *AssessmentTaskPO, domain *domainPlan.AssessmentTask) {
	// SyncID 只在 Save 时使用，此时 domain 已经有 ID 了
	// 这里只是为了确保 ID 同步，如果 domain 没有 ID，则设置
	if domain.GetID().IsZero() {
		var assessmentIDPtr *assessment.ID
		if po.AssessmentID != nil {
			assessmentID := assessment.ID(meta.FromUint64(*po.AssessmentID))
			assessmentIDPtr = &assessmentID
		}
		domain.RestoreFromRepository(
			meta.ID(po.ID),
			domain.GetStatus(),
			po.OpenAt,
			po.ExpireAt,
			po.CompletedAt,
			assessmentIDPtr,
			po.EntryToken,
			po.EntryURL,
		)
	}
}
