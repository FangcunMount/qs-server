package evaluation

import (
	"context"
	"strings"
	"time"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	mysqlEventOutbox "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventoutbox"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"github.com/FangcunMount/qs-server/pkg/event"
	"gorm.io/gorm"
)

// assessmentRepository 测评仓储实现
type assessmentRepository struct {
	mysql.BaseRepository[*AssessmentPO]
	mapper      *AssessmentMapper
	outboxStore *mysqlEventOutbox.Store
}

// NewAssessmentRepository 创建测评仓储
func NewAssessmentRepository(db *gorm.DB, opts ...mysql.BaseRepositoryOptions) assessment.Repository {
	return NewAssessmentRepositoryWithTopicResolver(db, nil, opts...)
}

func NewAssessmentRepositoryWithTopicResolver(db *gorm.DB, resolver eventcatalog.TopicResolver, opts ...mysql.BaseRepositoryOptions) assessment.Repository {
	repo := &assessmentRepository{
		BaseRepository: mysql.NewBaseRepository[*AssessmentPO](db, opts...),
		mapper:         NewAssessmentMapper(),
		outboxStore:    mysqlEventOutbox.NewStoreWithTopicResolver(db, resolver),
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
		if err := po.BeforeCreate(nil); err != nil {
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

// SaveWithEvents 保存测评并将聚合上的事件落到 MySQL outbox。
// Deprecated: application use cases should use UoW + outbox stager explicitly.
func (r *assessmentRepository) SaveWithEvents(ctx context.Context, a *assessment.Assessment) error {
	return r.SaveWithAdditionalEvents(ctx, a, nil)
}

// SaveWithAdditionalEvents 保存测评并在同一事务里暂存聚合事件与补充事件。
// Deprecated: application use cases should use UoW + outbox stager explicitly.
func (r *assessmentRepository) SaveWithAdditionalEvents(ctx context.Context, a *assessment.Assessment, additional []event.DomainEvent) error {
	if a == nil {
		return nil
	}

	err := r.BaseRepository.DB().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txCtx := mysql.WithTx(ctx, tx)
		if err := r.Save(txCtx, a); err != nil {
			return err
		}
		eventsToStage := make([]event.DomainEvent, 0, len(a.Events())+len(additional))
		eventsToStage = append(eventsToStage, a.Events()...)
		eventsToStage = append(eventsToStage, additional...)
		if len(eventsToStage) == 0 {
			return nil
		}
		return r.outboxStore.Stage(txCtx, eventsToStage...)
	})
	if err != nil {
		return err
	}

	a.ClearEvents()
	return nil
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
		Where("testee_id = ? AND deleted_at IS NULL", testeeID.Uint64())

	// 统计总数
	if err := query.Model(&AssessmentPO{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	err := query.
		Order("id DESC").
		Offset(pagination.Offset()).
		Limit(pagination.Limit()).
		Find(&pos).Error

	if err != nil {
		return nil, 0, err
	}

	return r.mapper.ToDomainList(pos), total, nil
}

// FindByTesteeIDWithFilters 查询受试者的测评列表（支持分页和筛选）
func (r *assessmentRepository) FindByTesteeIDWithFilters(
	ctx context.Context,
	testeeID testee.ID,
	status string,
	scaleCode string,
	riskLevel string,
	dateFrom *time.Time,
	dateTo *time.Time,
	pagination assessment.Pagination,
) ([]*assessment.Assessment, int64, error) {
	var pos []*AssessmentPO
	var total int64

	query := r.WithContext(ctx).
		Where("testee_id = ? AND deleted_at IS NULL", testeeID.Uint64())

	query = applyAssessmentStatusFilter(query, status)
	if scaleCode != "" {
		query = query.Where("medical_scale_code = ?", scaleCode)
	}
	if riskLevel != "" {
		query = query.Where("risk_level = ?", strings.ToLower(riskLevel))
	}
	if dateFrom != nil {
		query = query.Where("created_at >= ?", *dateFrom)
	}
	if dateTo != nil {
		query = query.Where("created_at < ?", *dateTo)
	}

	if err := query.Model(&AssessmentPO{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.
		Order("id DESC").
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
			testeeID.Uint64(), scaleRef.ID().Uint64())

	// 统计总数
	if err := query.Model(&AssessmentPO{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	err := query.
		Order("id DESC").
		Offset(pagination.Offset()).
		Limit(pagination.Limit()).
		Find(&pos).Error

	if err != nil {
		return nil, 0, err
	}

	return r.mapper.ToDomainList(pos), total, nil
}

func applyAssessmentStatusFilter(query *gorm.DB, rawStatus string) *gorm.DB {
	switch strings.ToLower(strings.TrimSpace(rawStatus)) {
	case "":
		return query
	case "pending":
		return query.Where("status IN ?", []string{
			assessment.StatusPending.String(),
			assessment.StatusSubmitted.String(),
		})
	case "done":
		return query.Where("status = ?", assessment.StatusInterpreted.String())
	default:
		return query.Where("status = ?", rawStatus)
	}
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
		Order("id DESC").
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
			testeeID.Uint64(), status.String()).
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
		convertedID, err := metaIDToUint64(id)
		if err != nil {
			return nil, err
		}
		idList[i] = convertedID
	}

	var pos []*AssessmentPO
	err := r.WithContext(ctx).
		Where("id IN ? AND deleted_at IS NULL", idList).
		Order("id DESC").
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
		Order("id ASC"). // 按创建时间升序，优先处理老的
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
		Order("id DESC").
		Offset(pagination.Offset()).
		Limit(pagination.Limit()).
		Find(&pos).Error

	if err != nil {
		return nil, 0, err
	}

	return r.mapper.ToDomainList(pos), total, nil
}

// FindByOrgIDAndTesteeIDs 按组织和受试者集合查询测评列表。
func (r *assessmentRepository) FindByOrgIDAndTesteeIDs(
	ctx context.Context,
	orgID int64,
	testeeIDs []testee.ID,
	status *assessment.Status,
	pagination assessment.Pagination,
) ([]*assessment.Assessment, int64, error) {
	if len(testeeIDs) == 0 {
		return []*assessment.Assessment{}, 0, nil
	}

	var pos []*AssessmentPO
	var total int64
	rawIDs := make([]uint64, 0, len(testeeIDs))
	for _, id := range testeeIDs {
		convertedID, err := metaIDToUint64(id)
		if err != nil {
			return nil, 0, err
		}
		rawIDs = append(rawIDs, convertedID)
	}

	query := r.WithContext(ctx).
		Where("org_id = ? AND testee_id IN ? AND deleted_at IS NULL", orgID, rawIDs)

	if status != nil {
		query = query.Where("status = ?", status.String())
	}

	if err := query.Model(&AssessmentPO{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.
		Order("id DESC").
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
