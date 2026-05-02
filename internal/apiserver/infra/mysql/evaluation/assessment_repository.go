package evaluation

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/eventcatalog"
	"gorm.io/gorm"
)

// assessmentRepository 测评仓储实现
type assessmentRepository struct {
	mysql.BaseRepository[*AssessmentPO]
	mapper *AssessmentMapper
}

// NewAssessmentRepository 创建测评仓储
func NewAssessmentRepository(db *gorm.DB, opts ...mysql.BaseRepositoryOptions) assessment.Repository {
	return NewAssessmentRepositoryWithTopicResolver(db, nil, opts...)
}

func NewAssessmentRepositoryWithTopicResolver(db *gorm.DB, resolver eventcatalog.TopicResolver, opts ...mysql.BaseRepositoryOptions) assessment.Repository {
	repo := &assessmentRepository{
		BaseRepository: mysql.NewBaseRepository[*AssessmentPO](db, opts...),
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
