package evaluation

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"github.com/FangcunMount/qs-server/internal/pkg/middleware"
	"gorm.io/gorm"
)

// scoreRepository 测评得分仓储实现
type scoreRepository struct {
	mysql.BaseRepository[*AssessmentScorePO]
	mapper *ScoreMapper
}

// NewScoreRepository 创建得分仓储
func NewScoreRepository(db *gorm.DB, opts ...mysql.BaseRepositoryOptions) assessment.ScoreRepository {
	repo := &scoreRepository{
		BaseRepository: mysql.NewBaseRepository[*AssessmentScorePO](db, opts...),
		mapper:         NewScoreMapper(),
	}
	// 设置错误转换器
	repo.SetErrorTranslator(translateScoreError)
	return repo
}

// ==================== 批量保存 ====================

// SaveScores 批量保存得分
// 注意：需要传入辅助信息来扁平化存储
func (r *scoreRepository) SaveScores(_ context.Context, scores []*assessment.AssessmentScore) error {
	if len(scores) == 0 {
		return nil
	}

	// 这个方法需要外部传入 testeeID 和 scaleInfo
	// 由于当前领域模型中 AssessmentScore 不包含这些信息，
	// 需要从 Assessment 中获取或通过其他方式传递
	// 暂时使用占位实现，后续需要优化

	return errors.WithCode(code.ErrAssessmentScoreSaveFailed, "SaveScores requires additional context, use SaveScoresWithContext instead")
}

// SaveScoresWithContext 带上下文保存得分（包含受试者和量表信息）
func (r *scoreRepository) SaveScoresWithContext(ctx context.Context, assessmentDomain *assessment.Assessment, score *assessment.AssessmentScore) error {
	if score == nil || assessmentDomain == nil {
		return nil
	}

	// 获取辅助信息
	testeeID := assessmentDomain.TesteeID().Uint64()
	var scaleID uint64
	var scaleCode string

	if scaleRef := assessmentDomain.MedicalScaleRef(); scaleRef != nil {
		scaleID = scaleRef.ID().Uint64()
		scaleCode = scaleRef.Code().String()
	}

	// 转换为 PO 列表
	pos := r.mapper.ToPOs(score, testeeID, scaleID, scaleCode)
	if len(pos) == 0 {
		return nil
	}

	// 确保每个 PO 都调用 BeforeCreate 生成 ID
	for _, po := range pos {
		if err := po.BeforeCreate(nil); err != nil {
			return err
		}
	}

	userID := middleware.GetUserIDFromContext(ctx)
	if userID > 0 {
		auditID := meta.FromUint64(userID)
		for _, po := range pos {
			po.SetCreatedBy(auditID)
			po.SetUpdatedBy(auditID)
		}
	}

	// 批量创建
	return r.WithContext(ctx).Create(&pos).Error
}

// ==================== 删除 ====================

// DeleteByAssessmentID 删除测评的所有得分
func (r *scoreRepository) DeleteByAssessmentID(ctx context.Context, assessmentID assessment.ID) error {
	return r.WithContext(ctx).
		Where("assessment_id = ?", assessmentID.Uint64()).
		Delete(&AssessmentScorePO{}).Error
}

// translateScoreError 将数据库错误转换为领域错误
func translateScoreError(err error) error {
	if err == nil {
		return nil
	}

	// 处理唯一约束冲突
	if mysql.IsDuplicateError(err) {
		return errors.WithCode(code.ErrAssessmentScoreSaveFailed, "score already exists")
	}

	// 处理记录不存在
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.WithCode(code.ErrAssessmentScoreNotFound, "score not found")
	}

	return err
}
