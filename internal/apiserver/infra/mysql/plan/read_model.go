package plan

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/port/planreadmodel"
	"gorm.io/gorm"
)

// readModel implements plan read-side queries without widening domain repositories.
type readModel struct {
	db *gorm.DB
}

// NewReadModel creates the MySQL-backed plan read model adapter.
func NewReadModel(db *gorm.DB) interface {
	planreadmodel.PlanReader
	planreadmodel.TaskReader
} {
	return &readModel{db: db}
}

func (m *readModel) GetPlan(ctx context.Context, orgID int64, planID uint64) (*planreadmodel.PlanRow, error) {
	var po AssessmentPlanPO
	query := m.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", planID)
	if orgID > 0 {
		query = query.Where("org_id = ?", orgID)
	}
	if err := query.First(&po).Error; err != nil {
		return nil, err
	}
	row := planRowFromPO(&po)
	return &row, nil
}

func (m *readModel) ListPlans(ctx context.Context, filter planreadmodel.PlanFilter, page planreadmodel.PageRequest) (planreadmodel.PlanPage, error) {
	var pos []*AssessmentPlanPO
	var total int64

	query := buildPlanListQuery(m.db.WithContext(ctx), filter)
	if err := query.Model(&AssessmentPlanPO{}).Count(&total).Error; err != nil {
		return planreadmodel.PlanPage{}, err
	}
	if page.Page > 0 && page.PageSize > 0 {
		query = query.Offset((page.Page - 1) * page.PageSize).Limit(page.PageSize)
	}
	if err := query.Order("id DESC").Find(&pos).Error; err != nil {
		return planreadmodel.PlanPage{}, err
	}
	return planreadmodel.PlanPage{
		Items:    planRowsFromPOs(pos),
		Total:    total,
		Page:     page.Page,
		PageSize: page.PageSize,
	}, nil
}

func (m *readModel) ListPlansByTesteeID(ctx context.Context, testeeID uint64) ([]planreadmodel.PlanRow, error) {
	var pos []*AssessmentPlanPO
	if err := m.db.WithContext(ctx).
		Table("assessment_plan").
		Select("DISTINCT assessment_plan.*").
		Joins("INNER JOIN assessment_task ON assessment_plan.id = assessment_task.plan_id").
		Where("assessment_task.testee_id = ? AND assessment_plan.deleted_at IS NULL AND assessment_task.deleted_at IS NULL", testeeID).
		Find(&pos).Error; err != nil {
		return nil, err
	}
	return planRowsFromPOs(pos), nil
}

func (m *readModel) GetTask(ctx context.Context, orgID int64, taskID uint64) (*planreadmodel.TaskRow, error) {
	var po AssessmentTaskPO
	query := m.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", taskID)
	if orgID > 0 {
		query = query.Where("org_id = ?", orgID)
	}
	if err := query.First(&po).Error; err != nil {
		return nil, err
	}
	row := taskRowFromPO(&po)
	return &row, nil
}

func (m *readModel) ListTasks(ctx context.Context, filter planreadmodel.TaskFilter, page planreadmodel.PageRequest) (planreadmodel.TaskPage, error) {
	if filter.RestrictToAccessScope && len(filter.AccessibleTesteeIDs) == 0 {
		return planreadmodel.TaskPage{Items: []planreadmodel.TaskRow{}, Page: page.Page, PageSize: page.PageSize}, nil
	}

	var pos []*AssessmentTaskPO
	var total int64
	query := buildTaskListQuery(m.db.WithContext(ctx), filter)
	if err := query.Model(&AssessmentTaskPO{}).Count(&total).Error; err != nil {
		return planreadmodel.TaskPage{}, err
	}
	if page.Page > 0 && page.PageSize > 0 {
		query = query.Offset((page.Page - 1) * page.PageSize).Limit(page.PageSize)
	}
	if err := query.Order("planned_at DESC").Find(&pos).Error; err != nil {
		return planreadmodel.TaskPage{}, err
	}
	return planreadmodel.TaskPage{
		Items:    taskRowsFromPOs(pos),
		Total:    total,
		Page:     page.Page,
		PageSize: page.PageSize,
	}, nil
}

func (m *readModel) ListTaskWindow(ctx context.Context, filter planreadmodel.TaskWindowFilter, page planreadmodel.PageRequest) (planreadmodel.TaskWindow, error) {
	if page.Page <= 0 {
		page.Page = 1
	}
	if page.PageSize <= 0 {
		page.PageSize = 10
	}

	query := buildTaskWindowQuery(m.db.WithContext(ctx), filter)

	var pos []*AssessmentTaskPO
	limit := page.PageSize + 1
	if err := query.
		Order("planned_at ASC").
		Order("id ASC").
		Offset((page.Page - 1) * page.PageSize).
		Limit(limit).
		Find(&pos).Error; err != nil {
		return planreadmodel.TaskWindow{}, err
	}
	hasMore := len(pos) > page.PageSize
	if hasMore {
		pos = pos[:page.PageSize]
	}
	return planreadmodel.TaskWindow{
		Items:    taskRowsFromPOs(pos),
		Page:     page.Page,
		PageSize: page.PageSize,
		HasMore:  hasMore,
	}, nil
}

func buildPlanListQuery(db *gorm.DB, filter planreadmodel.PlanFilter) *gorm.DB {
	query := db.Where("deleted_at IS NULL")
	if filter.OrgID > 0 {
		query = query.Where("org_id = ?", filter.OrgID)
	}
	if filter.ScaleCode != "" {
		query = query.Where("scale_code = ?", filter.ScaleCode)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	return query
}

func buildTaskListQuery(db *gorm.DB, filter planreadmodel.TaskFilter) *gorm.DB {
	query := db.Where("org_id = ? AND deleted_at IS NULL", filter.OrgID)
	if filter.PlanID != nil {
		query = query.Where("plan_id = ?", *filter.PlanID)
	}
	if filter.RestrictToAccessScope {
		query = query.Where("testee_id IN ?", filter.AccessibleTesteeIDs)
	} else if filter.TesteeID != nil {
		query = query.Where("testee_id = ?", *filter.TesteeID)
	}
	if filter.Status != nil {
		query = query.Where("status = ?", *filter.Status)
	}
	return query
}

func buildTaskWindowQuery(db *gorm.DB, filter planreadmodel.TaskWindowFilter) *gorm.DB {
	query := db.Where("org_id = ? AND plan_id = ? AND deleted_at IS NULL", filter.OrgID, filter.PlanID)
	if len(filter.TesteeIDs) > 0 {
		query = query.Where("testee_id IN ?", filter.TesteeIDs)
	}
	if filter.Status != nil {
		query = query.Where("status = ?", *filter.Status)
	}
	if filter.PlannedBefore != nil {
		query = query.Where("planned_at <= ?", *filter.PlannedBefore)
	}
	return query
}

func (m *readModel) ListTasksByPlanID(ctx context.Context, planID uint64) ([]planreadmodel.TaskRow, error) {
	var pos []*AssessmentTaskPO
	if err := m.db.WithContext(ctx).
		Where("plan_id = ? AND deleted_at IS NULL", planID).
		Order("seq ASC").
		Find(&pos).Error; err != nil {
		return nil, err
	}
	return taskRowsFromPOs(pos), nil
}

func (m *readModel) ListTasksByPlanIDAndTesteeIDs(ctx context.Context, planID uint64, testeeIDs []uint64) ([]planreadmodel.TaskRow, error) {
	if len(testeeIDs) == 0 {
		return []planreadmodel.TaskRow{}, nil
	}
	var pos []*AssessmentTaskPO
	if err := m.db.WithContext(ctx).
		Where("plan_id = ? AND testee_id IN ? AND deleted_at IS NULL", planID, testeeIDs).
		Order("seq ASC").
		Find(&pos).Error; err != nil {
		return nil, err
	}
	return taskRowsFromPOs(pos), nil
}

func (m *readModel) ListTasksByTesteeID(ctx context.Context, testeeID uint64) ([]planreadmodel.TaskRow, error) {
	var pos []*AssessmentTaskPO
	if err := m.db.WithContext(ctx).
		Where("testee_id = ? AND deleted_at IS NULL", testeeID).
		Order("planned_at ASC").
		Find(&pos).Error; err != nil {
		return nil, err
	}
	return taskRowsFromPOs(pos), nil
}

func (m *readModel) ListTasksByTesteeIDAndPlanID(ctx context.Context, testeeID uint64, planID uint64) ([]planreadmodel.TaskRow, error) {
	var pos []*AssessmentTaskPO
	if err := m.db.WithContext(ctx).
		Where("testee_id = ? AND plan_id = ? AND deleted_at IS NULL", testeeID, planID).
		Order("seq ASC").
		Find(&pos).Error; err != nil {
		return nil, err
	}
	return taskRowsFromPOs(pos), nil
}

func planRowFromPO(po *AssessmentPlanPO) planreadmodel.PlanRow {
	if po == nil {
		return planreadmodel.PlanRow{}
	}
	return planreadmodel.PlanRow{
		ID:            po.ID.Uint64(),
		OrgID:         po.OrgID,
		ScaleCode:     po.ScaleCode,
		ScheduleType:  po.ScheduleType,
		TriggerTime:   po.TriggerTime,
		Interval:      po.Interval,
		TotalTimes:    po.TotalTimes,
		FixedDates:    append([]string(nil), po.FixedDates...),
		RelativeWeeks: append([]int(nil), po.RelativeWeeks...),
		Status:        po.Status,
	}
}

func planRowsFromPOs(pos []*AssessmentPlanPO) []planreadmodel.PlanRow {
	if len(pos) == 0 {
		return []planreadmodel.PlanRow{}
	}
	rows := make([]planreadmodel.PlanRow, 0, len(pos))
	for _, po := range pos {
		rows = append(rows, planRowFromPO(po))
	}
	return rows
}

func taskRowFromPO(po *AssessmentTaskPO) planreadmodel.TaskRow {
	if po == nil {
		return planreadmodel.TaskRow{}
	}
	return planreadmodel.TaskRow{
		ID:           po.ID.Uint64(),
		PlanID:       po.PlanID,
		Seq:          po.Seq,
		OrgID:        po.OrgID,
		TesteeID:     po.TesteeID,
		ScaleCode:    po.ScaleCode,
		PlannedAt:    po.PlannedAt,
		OpenAt:       po.OpenAt,
		ExpireAt:     po.ExpireAt,
		CompletedAt:  po.CompletedAt,
		Status:       po.Status,
		AssessmentID: po.AssessmentID,
		EntryToken:   po.EntryToken,
		EntryURL:     po.EntryURL,
	}
}

func taskRowsFromPOs(pos []*AssessmentTaskPO) []planreadmodel.TaskRow {
	if len(pos) == 0 {
		return []planreadmodel.TaskRow{}
	}
	rows := make([]planreadmodel.TaskRow, 0, len(pos))
	for _, po := range pos {
		rows = append(rows, taskRowFromPO(po))
	}
	return rows
}
