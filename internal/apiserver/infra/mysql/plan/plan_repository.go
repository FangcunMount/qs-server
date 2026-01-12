package plan

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	domainPlan "github.com/FangcunMount/qs-server/internal/apiserver/domain/plan"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

// planRepository 计划仓储实现
type planRepository struct {
	mysql.BaseRepository[*AssessmentPlanPO]
	mapper *PlanMapper
}

// NewPlanRepository 创建计划仓储
func NewPlanRepository(db *gorm.DB) domainPlan.AssessmentPlanRepository {
	repo := &planRepository{
		BaseRepository: mysql.NewBaseRepository[*AssessmentPlanPO](db),
		mapper:         NewPlanMapper(),
	}
	// 设置错误转换器
	repo.SetErrorTranslator(translatePlanError)
	return repo
}

// FindByID 根据ID查询计划
func (r *planRepository) FindByID(ctx context.Context, id domainPlan.AssessmentPlanID) (*domainPlan.AssessmentPlan, error) {
	po, err := r.BaseRepository.FindByID(ctx, id.Uint64())
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrPageNotFound, "plan not found")
		}
		return nil, err
	}

	return r.mapper.ToDomain(po), nil
}

// FindByScaleCode 查询某个量表的所有计划
func (r *planRepository) FindByScaleCode(ctx context.Context, scaleCode string) ([]*domainPlan.AssessmentPlan, error) {
	var pos []*AssessmentPlanPO
	err := r.WithContext(ctx).
		Where("scale_code = ? AND deleted_at IS NULL", scaleCode).
		Find(&pos).Error

	if err != nil {
		return nil, err
	}

	return r.mapper.ToDomainList(pos), nil
}

// FindActivePlans 查询所有活跃的计划
func (r *planRepository) FindActivePlans(ctx context.Context) ([]*domainPlan.AssessmentPlan, error) {
	var pos []*AssessmentPlanPO
	err := r.WithContext(ctx).
		Where("status = ? AND deleted_at IS NULL", domainPlan.PlanStatusActive.String()).
		Find(&pos).Error

	if err != nil {
		return nil, err
	}

	return r.mapper.ToDomainList(pos), nil
}

// FindByTesteeID 查询某个受试者参与的所有计划
// 实现方式：通过 JOIN Task 表查询，返回去重后的 Plan 列表
func (r *planRepository) FindByTesteeID(ctx context.Context, testeeID testee.ID) ([]*domainPlan.AssessmentPlan, error) {
	var pos []*AssessmentPlanPO

	// 使用 JOIN 查询，通过 Task 表关联 Plan 表
	// 明确选择 assessment_plan 的所有字段，并使用 DISTINCT 去重
	err := r.WithContext(ctx).
		Table("assessment_plan").
		Select("DISTINCT assessment_plan.*").
		Joins("INNER JOIN assessment_task ON assessment_plan.id = assessment_task.plan_id").
		Where("assessment_task.testee_id = ? AND assessment_plan.deleted_at IS NULL AND assessment_task.deleted_at IS NULL", testeeID.Uint64()).
		Find(&pos).Error

	if err != nil {
		return nil, err
	}

	return r.mapper.ToDomainList(pos), nil
}

// FindList 分页查询计划列表（支持条件筛选）
func (r *planRepository) FindList(ctx context.Context, orgID int64, scaleCode string, status string, page, pageSize int) ([]*domainPlan.AssessmentPlan, int64, error) {
	var pos []*AssessmentPlanPO
	var total int64

	// 构建查询条件
	db := r.WithContext(ctx).Where("deleted_at IS NULL")

	// 添加筛选条件
	if orgID > 0 {
		db = db.Where("org_id = ?", orgID)
	}
	if scaleCode != "" {
		db = db.Where("scale_code = ?", scaleCode)
	}
	if status != "" {
		db = db.Where("status = ?", status)
	}

	// 获取总数
	if err := db.Model(&AssessmentPlanPO{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	if page > 0 && pageSize > 0 {
		offset := (page - 1) * pageSize
		db = db.Offset(offset).Limit(pageSize)
	}

	// 按创建时间倒序
	db = db.Order("id DESC")

	// 执行查询
	if err := db.Find(&pos).Error; err != nil {
		return nil, 0, err
	}

	return r.mapper.ToDomainList(pos), total, nil
}

// Save 保存计划（新增或更新）
func (r *planRepository) Save(ctx context.Context, plan *domainPlan.AssessmentPlan) error {
	po := r.mapper.ToPO(plan)

	// 判断是新增还是更新
	if plan.GetID().IsZero() {
		// ID 为零，确保 BeforeCreate 被调用以生成 ID
		if err := po.BeforeCreate(); err != nil {
			return err
		}
		// 直接创建
		return r.CreateAndSync(ctx, po, func(po *AssessmentPlanPO) {
			r.mapper.SyncID(po, plan)
		})
	}

	// ID 不为零，先检查记录是否存在
	exists, err := r.ExistsByID(ctx, plan.GetID().Uint64())
	if err != nil {
		return err
	}

	if !exists {
		// 记录不存在，确保 BeforeCreate 被调用（虽然已有 ID，但需要设置版本号）
		if err := po.BeforeCreate(); err != nil {
			return err
		}
		// 执行 INSERT（使用指定的 ID）
		return r.CreateAndSync(ctx, po, func(po *AssessmentPlanPO) {
			r.mapper.SyncID(po, plan)
		})
	}

	// 记录存在，执行 UPDATE
	return r.UpdateAndSync(ctx, po, func(po *AssessmentPlanPO) {
		r.mapper.SyncID(po, plan)
	})
}

// translatePlanError 将数据库错误转换为领域错误
func translatePlanError(err error) error {
	if err == nil {
		return nil
	}

	// 处理唯一约束冲突
	if mysql.IsDuplicateError(err) {
		return errors.WithCode(code.ErrInvalidArgument, "plan already exists")
	}

	// 处理记录不存在
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.WithCode(code.ErrPageNotFound, "plan not found")
	}

	return err
}
