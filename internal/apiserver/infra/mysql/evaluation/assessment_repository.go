package evaluation

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

// assessmentRepository 测评仓储实现
type assessmentRepository struct {
	mysql.BaseRepository[*AssessmentPO]
	mapper *AssessmentMapper
}

// NewAssessmentRepository 创建测评仓储
func NewAssessmentRepository(db *gorm.DB) assessment.Repository {
	repo := &assessmentRepository{
		BaseRepository: mysql.NewBaseRepository[*AssessmentPO](db),
		mapper:         NewAssessmentMapper(),
	}
	// 设置错误转换器
	repo.SetErrorTranslator(translateAssessmentError)
	return repo
}

// ==================== 基础 CRUD ====================

// Save 保存测评（新增或更新）
func (r *assessmentRepository) Save(ctx context.Context, a *assessment.Assessment) error {
	po := r.mapper.ToPO(a)

	// 判断是新增还是更新
	if a.ID().IsZero() {
		// 确保 BeforeCreate 被调用以生成 ID
		if err := po.BeforeCreate(); err != nil {
			return err
		}
		return r.CreateAndSync(ctx, po, func(po *AssessmentPO) {
			r.mapper.SyncID(po, a)
		})
	}

	return r.UpdateAndSync(ctx, po, func(po *AssessmentPO) {
		r.mapper.SyncID(po, a)
	})
}

// FindByID 根据ID查找
func (r *assessmentRepository) FindByID(ctx context.Context, id assessment.ID) (*assessment.Assessment, error) {
	po, err := r.BaseRepository.FindByID(ctx, id.Uint64())
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrAssessmentNotFound, "assessment not found")
		}
		return nil, err
	}

	return r.mapper.ToDomain(po), nil
}

// Delete 删除测评
func (r *assessmentRepository) Delete(ctx context.Context, id assessment.ID) error {
	return r.DeleteByID(ctx, id.Uint64())
}

// ==================== 按关联查询 ====================

// FindByAnswerSheetID 根据答卷ID查找
func (r *assessmentRepository) FindByAnswerSheetID(ctx context.Context, answerSheetRef assessment.AnswerSheetRef) (*assessment.Assessment, error) {
	var po AssessmentPO
	err := r.WithContext(ctx).
		Where("answer_sheet_id = ? AND deleted_at IS NULL", answerSheetRef.ID().Uint64()).
		First(&po).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.WithCode(code.ErrAssessmentNotFound, "assessment not found")
		}
		return nil, err
	}

	return r.mapper.ToDomain(&po), nil
}

// FindByTesteeID 查询受试者的测评列表（支持分页）
func (r *assessmentRepository) FindByTesteeID(ctx context.Context, testeeID testee.ID, pagination assessment.Pagination) ([]*assessment.Assessment, int64, error) {
	var pos []*AssessmentPO
	var total int64

	query := r.WithContext(ctx).
		Where("testee_id = ? AND deleted_at IS NULL", uint64(testeeID))

	// 统计总数
	if err := query.Model(&AssessmentPO{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	err := query.
		Order("created_at DESC").
		Offset(pagination.Offset()).
		Limit(pagination.Limit()).
		Find(&pos).Error

	if err != nil {
		return nil, 0, err
	}

	return r.mapper.ToDomainList(pos), total, nil
}

// FindByTesteeIDAndScaleID 查询受试者在某个量表下的测评列表
func (r *assessmentRepository) FindByTesteeIDAndScaleID(ctx context.Context, testeeID testee.ID, scaleRef assessment.MedicalScaleRef, pagination assessment.Pagination) ([]*assessment.Assessment, int64, error) {
	var pos []*AssessmentPO
	var total int64

	query := r.WithContext(ctx).
		Where("testee_id = ? AND medical_scale_id = ? AND deleted_at IS NULL",
			uint64(testeeID), scaleRef.ID().Uint64())

	// 统计总数
	if err := query.Model(&AssessmentPO{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	err := query.
		Order("created_at DESC").
		Offset(pagination.Offset()).
		Limit(pagination.Limit()).
		Find(&pos).Error

	if err != nil {
		return nil, 0, err
	}

	return r.mapper.ToDomainList(pos), total, nil
}

// ==================== 按业务来源查询 ====================

// FindByPlanID 查询计划下的测评列表
func (r *assessmentRepository) FindByPlanID(ctx context.Context, planID string, pagination assessment.Pagination) ([]*assessment.Assessment, int64, error) {
	var pos []*AssessmentPO
	var total int64

	query := r.WithContext(ctx).
		Where("origin_type = ? AND origin_id = ? AND deleted_at IS NULL",
			assessment.OriginPlan, planID)

	// 统计总数
	if err := query.Model(&AssessmentPO{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	err := query.
		Order("created_at DESC").
		Offset(pagination.Offset()).
		Limit(pagination.Limit()).
		Find(&pos).Error

	if err != nil {
		return nil, 0, err
	}

	return r.mapper.ToDomainList(pos), total, nil
}

// FindByScreeningProjectID 查询筛查项目下的测评列表
func (r *assessmentRepository) FindByScreeningProjectID(ctx context.Context, screeningProjectID string, pagination assessment.Pagination) ([]*assessment.Assessment, int64, error) {
	var pos []*AssessmentPO
	var total int64

	query := r.WithContext(ctx).
		Where("origin_type = ? AND origin_id = ? AND deleted_at IS NULL",
			assessment.OriginScreening, screeningProjectID)

	// 统计总数
	if err := query.Model(&AssessmentPO{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	err := query.
		Order("created_at DESC").
		Offset(pagination.Offset()).
		Limit(pagination.Limit()).
		Find(&pos).Error

	if err != nil {
		return nil, 0, err
	}

	return r.mapper.ToDomainList(pos), total, nil
}

// ==================== 统计查询 ====================

// CountByStatus 按状态统计数量
func (r *assessmentRepository) CountByStatus(ctx context.Context, status assessment.Status) (int64, error) {
	var count int64
	err := r.WithContext(ctx).
		Model(&AssessmentPO{}).
		Where("status = ? AND deleted_at IS NULL", status.String()).
		Count(&count).Error

	return count, err
}

// CountByTesteeIDAndStatus 按受试者和状态统计
func (r *assessmentRepository) CountByTesteeIDAndStatus(ctx context.Context, testeeID testee.ID, status assessment.Status) (int64, error) {
	var count int64
	err := r.WithContext(ctx).
		Model(&AssessmentPO{}).
		Where("testee_id = ? AND status = ? AND deleted_at IS NULL",
			uint64(testeeID), status.String()).
		Count(&count).Error

	return count, err
}

// CountByOrgIDAndStatus 按组织和状态统计
func (r *assessmentRepository) CountByOrgIDAndStatus(ctx context.Context, orgID int64, status assessment.Status) (int64, error) {
	var count int64
	err := r.WithContext(ctx).
		Model(&AssessmentPO{}).
		Where("org_id = ? AND status = ? AND deleted_at IS NULL", orgID, status.String()).
		Count(&count).Error

	return count, err
}

// ==================== 批量查询 ====================

// FindByIDs 批量查询（根据ID列表）
func (r *assessmentRepository) FindByIDs(ctx context.Context, ids []assessment.ID) ([]*assessment.Assessment, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	// 转换ID列表
	idList := make([]uint64, len(ids))
	for i, id := range ids {
		idList[i] = id.Uint64()
	}

	var pos []*AssessmentPO
	err := r.WithContext(ctx).
		Where("id IN ? AND deleted_at IS NULL", idList).
		Order("created_at DESC").
		Find(&pos).Error

	if err != nil {
		return nil, err
	}

	return r.mapper.ToDomainList(pos), nil
}

// FindPendingSubmission 查找待提交的测评
func (r *assessmentRepository) FindPendingSubmission(ctx context.Context, pagination assessment.Pagination) ([]*assessment.Assessment, int64, error) {
	var pos []*AssessmentPO
	var total int64

	query := r.WithContext(ctx).
		Where("status = ? AND deleted_at IS NULL", assessment.StatusPending.String())

	// 统计总数
	if err := query.Model(&AssessmentPO{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	err := query.
		Order("created_at ASC"). // 按创建时间升序，优先处理老的
		Offset(pagination.Offset()).
		Limit(pagination.Limit()).
		Find(&pos).Error

	if err != nil {
		return nil, 0, err
	}

	return r.mapper.ToDomainList(pos), total, nil
}

// FindByOrgID 按组织ID查询测评列表（支持分页和条件筛选）
func (r *assessmentRepository) FindByOrgID(ctx context.Context, orgID int64, status *assessment.Status, pagination assessment.Pagination) ([]*assessment.Assessment, int64, error) {
	var pos []*AssessmentPO
	var total int64

	query := r.WithContext(ctx).
		Where("org_id = ? AND deleted_at IS NULL", orgID)

	// 状态筛选
	if status != nil {
		query = query.Where("status = ?", status.String())
	}

	// 统计总数
	if err := query.Model(&AssessmentPO{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	err := query.
		Order("created_at DESC").
		Offset(pagination.Offset()).
		Limit(pagination.Limit()).
		Find(&pos).Error

	if err != nil {
		return nil, 0, err
	}

	return r.mapper.ToDomainList(pos), total, nil
}

// ==================== 辅助方法 ====================

// translateAssessmentError 将数据库错误转换为领域错误
func translateAssessmentError(err error) error {
	if err == nil {
		return nil
	}

	// 处理唯一约束冲突
	if mysql.IsDuplicateError(err) {
		return errors.WithCode(code.ErrAssessmentDuplicate, "assessment already exists")
	}

	// 处理记录不存在
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.WithCode(code.ErrAssessmentNotFound, "assessment not found")
	}

	return err
}
