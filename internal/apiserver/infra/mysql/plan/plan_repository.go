package plan

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
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
func NewPlanRepository(db *gorm.DB, opts ...mysql.BaseRepositoryOptions) domainPlan.AssessmentPlanRepository {
	repo := &planRepository{
		BaseRepository: mysql.NewBaseRepository[*AssessmentPlanPO](db, opts...),
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

// Save 保存计划（新增或更新）
func (r *planRepository) Save(ctx context.Context, plan *domainPlan.AssessmentPlan) error {
	po := r.mapper.ToPO(plan)
	return saveMappedEntity(
		ctx,
		plan,
		po,
		func() error { return po.BeforeCreate(nil) },
		r.ExistsByID,
		r.createAndSyncPlan,
		r.updateAndSyncPlan,
	)
}

func (r *planRepository) createAndSyncPlan(ctx context.Context, po *AssessmentPlanPO, plan *domainPlan.AssessmentPlan) error {
	return r.CreateAndSync(ctx, po, func(saved *AssessmentPlanPO) {
		syncPlanPO(saved, plan, r.mapper)
	})
}

func (r *planRepository) updateAndSyncPlan(ctx context.Context, po *AssessmentPlanPO, plan *domainPlan.AssessmentPlan) error {
	return r.UpdateAndSync(ctx, po, func(saved *AssessmentPlanPO) {
		syncPlanPO(saved, plan, r.mapper)
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
