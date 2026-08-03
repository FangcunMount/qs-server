package plan

import (
	"context"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainPlan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// taskRepository 任务仓储实现
type taskRepository struct {
	mysql.BaseRepository[*AssessmentTaskPO]
	mapper *TaskMapper
}

// NewTaskRepository 创建任务仓储
func NewTaskRepository(db *gorm.DB, opts ...mysql.BaseRepositoryOptions) domainPlan.AssessmentTaskRepository {
	repo := &taskRepository{
		BaseRepository: mysql.NewBaseRepository[*AssessmentTaskPO](db, opts...),
		mapper:         NewTaskMapper(),
	}
	// 设置错误转换器
	repo.SetErrorTranslator(translateTaskError)
	return repo
}

// FindByID 根据ID查询任务
func (r *taskRepository) FindByID(ctx context.Context, id domainPlan.AssessmentTaskID) (*domainPlan.AssessmentTask, error) {
	po, err := r.BaseRepository.FindByID(ctx, id.Uint64())
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrPageNotFound, "task not found")
		}
		return nil, err
	}

	return r.mapper.ToDomain(po), nil
}

// FindByIDForUpdate serializes historical task transitions inside the caller's
// active transaction. It is exposed as an optional application capability and
// intentionally does not widen the domain repository contract.
func (r *taskRepository) FindByIDForUpdate(ctx context.Context, id domainPlan.AssessmentTaskID) (*domainPlan.AssessmentTask, error) {
	var po AssessmentTaskPO
	err := r.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id.Uint64()).First(&po).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrPageNotFound, "task not found")
		}
		return nil, err
	}
	return r.mapper.ToDomain(&po), nil
}

// FindByPlanID 查询某个计划的所有任务
func (r *taskRepository) FindByPlanID(ctx context.Context, planID domainPlan.AssessmentPlanID) ([]*domainPlan.AssessmentTask, error) {
	var pos []*AssessmentTaskPO
	err := r.WithContext(ctx).
		Where("plan_id = ? AND deleted_at IS NULL", planID.Uint64()).
		Order("seq ASC"). // 按序号排序
		Find(&pos).Error

	if err != nil {
		return nil, err
	}

	return r.mapper.ToDomainList(pos), nil
}

func (r *taskRepository) FindByPlanIDForUpdate(ctx context.Context, planID domainPlan.AssessmentPlanID) ([]*domainPlan.AssessmentTask, error) {
	var pos []*AssessmentTaskPO
	if err := r.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("plan_id = ? AND deleted_at IS NULL", planID.Uint64()).
		Order("id ASC").Find(&pos).Error; err != nil {
		return nil, err
	}
	return r.mapper.ToDomainList(pos), nil
}

// FindByPlanIDAndTesteeIDs 查询某个计划下指定受试者集合的任务。
func (r *taskRepository) FindByPlanIDAndTesteeIDs(ctx context.Context, planID domainPlan.AssessmentPlanID, testeeIDs []testee.ID) ([]*domainPlan.AssessmentTask, error) {
	if len(testeeIDs) == 0 {
		return []*domainPlan.AssessmentTask{}, nil
	}

	rawIDs := make([]uint64, 0, len(testeeIDs))
	for _, id := range testeeIDs {
		rawIDs = append(rawIDs, id.Uint64())
	}

	var pos []*AssessmentTaskPO
	err := r.WithContext(ctx).
		Where("plan_id = ? AND testee_id IN ? AND deleted_at IS NULL", planID.Uint64(), rawIDs).
		Order("seq ASC").
		Find(&pos).Error
	if err != nil {
		return nil, err
	}

	return r.mapper.ToDomainList(pos), nil
}

// FindByTesteeID 查询某个受试者的所有任务
func (r *taskRepository) FindByTesteeID(ctx context.Context, testeeID testee.ID) ([]*domainPlan.AssessmentTask, error) {
	var pos []*AssessmentTaskPO
	err := r.WithContext(ctx).
		Where("testee_id = ? AND deleted_at IS NULL", testeeID.Uint64()).
		Order("planned_at ASC"). // 按计划时间升序
		Find(&pos).Error

	if err != nil {
		return nil, err
	}

	return r.mapper.ToDomainList(pos), nil
}

// FindByTesteeIDAndPlanID 查询某个受试者在某个计划下的所有任务
func (r *taskRepository) FindByTesteeIDAndPlanID(ctx context.Context, testeeID testee.ID, planID domainPlan.AssessmentPlanID) ([]*domainPlan.AssessmentTask, error) {
	var pos []*AssessmentTaskPO
	err := r.WithContext(ctx).
		Where("testee_id = ? AND plan_id = ? AND deleted_at IS NULL", testeeID.Uint64(), planID.Uint64()).
		Order("seq ASC"). // 按序号排序
		Find(&pos).Error

	if err != nil {
		return nil, err
	}

	return r.mapper.ToDomainList(pos), nil
}

func (r *taskRepository) FindByEnrollmentID(ctx context.Context, enrollmentID domainPlan.PlanEnrollmentID) ([]*domainPlan.AssessmentTask, error) {
	var pos []*AssessmentTaskPO
	err := r.WithContext(ctx).
		Where("enrollment_id = ? AND deleted_at IS NULL", enrollmentID.Uint64()).
		Order("seq ASC").Find(&pos).Error
	if err != nil {
		return nil, err
	}
	return r.mapper.ToDomainList(pos), nil
}

// FindPendingTasks 查询待推送的任务（计划时间 <= before）
func (r *taskRepository) FindPendingTasks(ctx context.Context, orgID int64, before time.Time) ([]*domainPlan.AssessmentTask, error) {
	var pos []*AssessmentTaskPO
	err := r.WithContext(ctx).
		Where("org_id = ? AND status = ? AND planned_at <= ? AND deleted_at IS NULL",
						orgID, domainPlan.TaskStatusPending.String(), before).
		Order("planned_at ASC"). // 按计划时间升序，优先处理早的
		Find(&pos).Error

	if err != nil {
		return nil, err
	}

	return r.mapper.ToDomainList(pos), nil
}

// FindExpiredTasks 查询已过期的任务（状态为 opened，截止时间 <= now）
func (r *taskRepository) FindExpiredTasks(ctx context.Context) ([]*domainPlan.AssessmentTask, error) {
	now := time.Now()
	var pos []*AssessmentTaskPO
	err := r.WithContext(ctx).
		Where("status = ? AND expire_at IS NOT NULL AND expire_at <= ? AND deleted_at IS NULL",
					domainPlan.TaskStatusOpened.String(), now).
		Order("expire_at ASC"). // 按过期时间升序
		Find(&pos).Error

	if err != nil {
		return nil, err
	}

	return r.mapper.ToDomainList(pos), nil
}

func (r *taskRepository) FindOpenEligibleTaskPage(ctx context.Context, orgID int64, plannedAfter, plannedThrough, cursorAt time.Time, cursorID uint64, limit int) ([]*domainPlan.AssessmentTask, error) {
	if limit <= 0 {
		limit = 200
	}
	var pos []*AssessmentTaskPO
	query := r.WithContext(ctx).
		Where("org_id = ? AND status = ? AND planned_at > ? AND planned_at <= ? AND deleted_at IS NULL", orgID, domainPlan.TaskStatusPending.String(), plannedAfter, plannedThrough)
	if !cursorAt.IsZero() {
		query = query.Where("(planned_at > ?) OR (planned_at = ? AND id > ?)", cursorAt, cursorAt, cursorID)
	}
	if err := query.Order("planned_at ASC").Order("id ASC").Limit(limit).Find(&pos).Error; err != nil {
		return nil, err
	}
	return r.mapper.ToDomainList(pos), nil
}

func (r *taskRepository) FindMissedPendingTaskPage(ctx context.Context, orgID int64, plannedThrough, cursorAt time.Time, cursorID uint64, limit int) ([]*domainPlan.AssessmentTask, error) {
	if limit <= 0 {
		limit = 200
	}
	var pos []*AssessmentTaskPO
	query := r.WithContext(ctx).
		Where("org_id = ? AND status = ? AND planned_at <= ? AND deleted_at IS NULL", orgID, domainPlan.TaskStatusPending.String(), plannedThrough)
	if !cursorAt.IsZero() {
		query = query.Where("(planned_at > ?) OR (planned_at = ? AND id > ?)", cursorAt, cursorAt, cursorID)
	}
	if err := query.Order("planned_at ASC").Order("id ASC").Limit(limit).Find(&pos).Error; err != nil {
		return nil, err
	}
	return r.mapper.ToDomainList(pos), nil
}

func (r *taskRepository) FindEntryExpiredTaskPage(ctx context.Context, orgID int64, expireThrough, cursorAt time.Time, cursorID uint64, limit int) ([]*domainPlan.AssessmentTask, error) {
	if limit <= 0 {
		limit = 200
	}
	var pos []*AssessmentTaskPO
	query := r.WithContext(ctx).
		Where("org_id = ? AND status = ? AND expire_at IS NOT NULL AND expire_at <= ? AND deleted_at IS NULL", orgID, domainPlan.TaskStatusOpened.String(), expireThrough)
	if !cursorAt.IsZero() {
		query = query.Where("(expire_at > ?) OR (expire_at = ? AND id > ?)", cursorAt, cursorAt, cursorID)
	}
	if err := query.Order("expire_at ASC").Order("id ASC").Limit(limit).Find(&pos).Error; err != nil {
		return nil, err
	}
	return r.mapper.ToDomainList(pos), nil
}

func (r *taskRepository) FindScopedOpenEligibleTaskPage(ctx context.Context, orgID int64, planID domainPlan.AssessmentPlanID, testeeIDs []testee.ID, plannedAfter, plannedThrough, cursorAt time.Time, cursorID uint64, limit int) ([]*domainPlan.AssessmentTask, error) {
	query := r.scopedPendingSchedulerQuery(ctx, orgID, planID, testeeIDs).
		Where("planned_at > ? AND planned_at <= ?", plannedAfter, plannedThrough)
	return r.findPendingSchedulerPage(query, cursorAt, cursorID, limit)
}

func (r *taskRepository) FindScopedMissedPendingTaskPage(ctx context.Context, orgID int64, planID domainPlan.AssessmentPlanID, testeeIDs []testee.ID, plannedThrough, cursorAt time.Time, cursorID uint64, limit int) ([]*domainPlan.AssessmentTask, error) {
	query := r.scopedPendingSchedulerQuery(ctx, orgID, planID, testeeIDs).
		Where("planned_at <= ?", plannedThrough)
	return r.findPendingSchedulerPage(query, cursorAt, cursorID, limit)
}

func (r *taskRepository) scopedPendingSchedulerQuery(ctx context.Context, orgID int64, planID domainPlan.AssessmentPlanID, testeeIDs []testee.ID) *gorm.DB {
	query := r.WithContext(ctx).Where("org_id = ? AND plan_id = ? AND status = ? AND deleted_at IS NULL", orgID, planID.Uint64(), domainPlan.TaskStatusPending.String())
	if len(testeeIDs) == 0 {
		return query
	}
	rawIDs := make([]uint64, 0, len(testeeIDs))
	for _, id := range testeeIDs {
		rawIDs = append(rawIDs, id.Uint64())
	}
	return query.Where("testee_id IN ?", rawIDs)
}

func (r *taskRepository) findPendingSchedulerPage(query *gorm.DB, cursorAt time.Time, cursorID uint64, limit int) ([]*domainPlan.AssessmentTask, error) {
	if limit <= 0 {
		limit = 200
	}
	if !cursorAt.IsZero() {
		query = query.Where("(planned_at > ?) OR (planned_at = ? AND id > ?)", cursorAt, cursorAt, cursorID)
	}
	var pos []*AssessmentTaskPO
	if err := query.Order("planned_at ASC").Order("id ASC").Limit(limit).Find(&pos).Error; err != nil {
		return nil, err
	}
	return r.mapper.ToDomainList(pos), nil
}

// Save 保存任务（新增或更新）
func (r *taskRepository) Save(ctx context.Context, task *domainPlan.AssessmentTask) error {
	po := r.mapper.ToPO(task)
	return saveMappedEntity(
		ctx,
		task,
		po,
		func() error { return po.BeforeCreate(nil) },
		r.ExistsByID,
		r.createAndSyncTask,
		r.updateAndSyncTask,
	)
}

func (r *taskRepository) createAndSyncTask(ctx context.Context, po *AssessmentTaskPO, task *domainPlan.AssessmentTask) error {
	return r.CreateAndSync(ctx, po, func(saved *AssessmentTaskPO) {
		syncTaskPO(saved, task, r.mapper)
	})
}

func (r *taskRepository) updateAndSyncTask(ctx context.Context, po *AssessmentTaskPO, task *domainPlan.AssessmentTask) error {
	return r.UpdateAndSync(ctx, po, func(saved *AssessmentTaskPO) {
		syncTaskPO(saved, task, r.mapper)
	})
}

// SaveRescheduled persists a complete schedule reset with an expected-revision
// CAS. A generic struct update cannot be used here because GORM omits nil and
// zero fields, which would leave the previous schedule's terminal timestamps,
// assessment link, or entry credentials behind.
func (r *taskRepository) SaveRescheduled(ctx context.Context, task *domainPlan.AssessmentTask, expectedRevision uint32) error {
	po := r.mapper.ToPO(task)
	updates := map[string]any{
		"planned_at":          po.PlannedAt,
		"due_at":              po.DueAt,
		"schedule_revision":   po.ScheduleRevision,
		"schedule_defined_at": po.ScheduleDefinedAt,
		"status":              po.Status,
		"open_at":             po.OpenAt,
		"expire_at":           po.ExpireAt,
		"completed_at":        po.CompletedAt,
		"expired_at":          po.ExpiredAt,
		"canceled_at":         po.CanceledAt,
		"expiration_reason":   po.ExpirationReason,
		"assessment_id":       po.AssessmentID,
		"entry_token":         po.EntryToken,
		"entry_url":           po.EntryURL,
		"updated_at":          time.Now().UTC(),
		"version":             gorm.Expr("version + 1"),
	}
	result := r.WithContext(ctx).Model(&AssessmentTaskPO{}).
		Where("id = ? AND org_id = ? AND schedule_revision = ? AND deleted_at IS NULL", task.GetID().Uint64(), task.GetOrgID(), expectedRevision).
		Updates(updates)
	if result.Error != nil {
		return translateTaskError(result.Error)
	}
	if result.RowsAffected != 1 {
		return errors.WithCode(code.ErrConflict, "task schedule revision conflict: task=%d expected=%d", task.GetID().Uint64(), expectedRevision)
	}
	return nil
}

// SaveBatch 批量保存任务
func (r *taskRepository) SaveBatch(ctx context.Context, tasks []*domainPlan.AssessmentTask) error {
	if len(tasks) == 0 {
		return nil
	}

	// 转换为PO列表
	pos := make([]*AssessmentTaskPO, 0, len(tasks))
	for _, task := range tasks {
		pos = append(pos, r.mapper.ToPO(task))
	}

	// 确保每个 PO 都调用 BeforeCreate 生成 ID
	for _, po := range pos {
		if err := po.BeforeCreate(nil); err != nil {
			return err
		}
	}

	// 批量插入
	err := r.WithContext(ctx).CreateInBatches(pos, 100).Error
	if err != nil {
		return err
	}

	// 同步ID
	for i, po := range pos {
		r.mapper.SyncID(po, tasks[i])
	}

	return nil
}

// translateTaskError 将数据库错误转换为领域错误
func translateTaskError(err error) error {
	if err == nil {
		return nil
	}

	// 处理唯一约束冲突
	if mysql.IsDuplicateError(err) {
		return errors.WithCode(code.ErrInvalidArgument, "task already exists")
	}

	// 处理记录不存在
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.WithCode(code.ErrPageNotFound, "task not found")
	}

	return err
}
