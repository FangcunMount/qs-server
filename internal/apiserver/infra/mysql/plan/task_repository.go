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
)

// taskRepository 任务仓储实现
type taskRepository struct {
	mysql.BaseRepository[*AssessmentTaskPO]
	mapper *TaskMapper
}

// NewTaskRepository 创建任务仓储
func NewTaskRepository(db *gorm.DB) domainPlan.AssessmentTaskRepository {
	repo := &taskRepository{
		BaseRepository: mysql.NewBaseRepository[*AssessmentTaskPO](db),
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

// FindPendingTasks 查询待推送的任务（计划时间 <= before）
func (r *taskRepository) FindPendingTasks(ctx context.Context, before time.Time) ([]*domainPlan.AssessmentTask, error) {
	var pos []*AssessmentTaskPO
	err := r.WithContext(ctx).
		Where("status = ? AND planned_at <= ? AND deleted_at IS NULL",
						domainPlan.TaskStatusPending.String(), before).
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

// Save 保存任务（新增或更新）
func (r *taskRepository) Save(ctx context.Context, task *domainPlan.AssessmentTask) error {
	po := r.mapper.ToPO(task)

	// 判断是新增还是更新
	if task.GetID().IsZero() {
		// ID 为零，直接创建
		return r.CreateAndSync(ctx, po, func(po *AssessmentTaskPO) {
			r.mapper.SyncID(po, task)
		})
	}

	// ID 不为零，先检查记录是否存在
	exists, err := r.ExistsByID(ctx, task.GetID().Uint64())
	if err != nil {
		return err
	}

	if !exists {
		// 记录不存在，执行 INSERT（使用指定的 ID）
		return r.CreateAndSync(ctx, po, func(po *AssessmentTaskPO) {
			r.mapper.SyncID(po, task)
		})
	}

	// 记录存在，执行 UPDATE
	return r.UpdateAndSync(ctx, po, func(po *AssessmentTaskPO) {
		r.mapper.SyncID(po, task)
	})
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
