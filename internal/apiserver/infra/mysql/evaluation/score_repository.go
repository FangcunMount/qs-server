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

// SaveProjectionFromOutcome persists an Assessment score query projection with
// its immutable EvaluationOutcome provenance.
func (r *scoreRepository) SaveProjectionFromOutcome(ctx context.Context, outcomeID meta.ID, assessmentDomain *assessment.Assessment, score *assessment.ScaleScoreProjection) error {
	if score == nil || assessmentDomain == nil {
		return nil
	}
	if outcomeID.IsZero() {
		return errors.WithCode(code.ErrAssessmentScoreSaveFailed, "evaluation outcome id is required for score projection")
	}

	testeeID := assessmentDomain.TesteeID().Uint64()

	// 转换为 PO 列表
	pos := r.mapper.ToPOs(score, testeeID, outcomeID)
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
