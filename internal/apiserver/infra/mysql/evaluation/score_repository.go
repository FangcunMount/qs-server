package evaluation

import (
	"context"

	"github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
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
func NewScoreRepository(db *gorm.DB) assessment.ScoreRepository {
	repo := &scoreRepository{
		BaseRepository: mysql.NewBaseRepository[*AssessmentScorePO](db),
		mapper:         NewScoreMapper(),
	}
	// 设置错误转换器
	repo.SetErrorTranslator(translateScoreError)
	return repo
}

// ==================== 批量保存 ====================

// SaveScores 批量保存得分
// 注意：需要传入辅助信息来扁平化存储
func (r *scoreRepository) SaveScores(ctx context.Context, scores []*assessment.AssessmentScore) error {
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
		if err := po.BeforeCreate(); err != nil {
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

// ==================== 基础查询 ====================

// FindByAssessmentID 查询测评的所有得分
func (r *scoreRepository) FindByAssessmentID(ctx context.Context, assessmentID assessment.ID) ([]*assessment.AssessmentScore, error) {
	var pos []*AssessmentScorePO
	err := r.WithContext(ctx).
		Where("assessment_id = ? AND deleted_at IS NULL", assessmentID.Uint64()).
		Order("is_total_score DESC, factor_code ASC"). // 总分优先
		Find(&pos).Error

	if err != nil {
		return nil, err
	}

	// 按 AssessmentID 聚合并转换
	return r.mapper.ToDomainList(pos), nil
}

// ==================== 趋势分析查询 ====================

// FindByTesteeIDAndFactorCode 查询受试者在某个因子上的历史得分（用于趋势分析）
func (r *scoreRepository) FindByTesteeIDAndFactorCode(ctx context.Context, testeeID testee.ID, factorCode assessment.FactorCode, limit int) ([]*assessment.AssessmentScore, error) {
	var pos []*AssessmentScorePO
	query := r.WithContext(ctx).
		Where("testee_id = ? AND factor_code = ? AND deleted_at IS NULL",
			uint64(testeeID), factorCode.String()).
		Order("created_at DESC")

	if limit > 0 {
		query = query.Limit(limit)
	}

	err := query.Find(&pos).Error
	if err != nil {
		return nil, err
	}

	// 由于是按因子查询，每行对应一个 AssessmentScore
	// 但由于 ToDomainList 会聚合同一 Assessment 的记录，这里需要特殊处理
	// 每个 PO 单独转换为一个只包含该因子的 AssessmentScore
	return r.toSingleFactorDomainList(pos), nil
}

// FindLatestByTesteeIDAndScaleID 查询受试者在某个量表下所有因子的最新得分
func (r *scoreRepository) FindLatestByTesteeIDAndScaleID(ctx context.Context, testeeID testee.ID, scaleRef assessment.MedicalScaleRef) ([]*assessment.AssessmentScore, error) {
	// 首先找到最新的 AssessmentID
	var latestAssessmentID uint64
	err := r.WithContext(ctx).
		Model(&AssessmentScorePO{}).
		Select("assessment_id").
		Where("testee_id = ? AND medical_scale_id = ? AND deleted_at IS NULL",
			uint64(testeeID), scaleRef.ID().Uint64()).
		Order("created_at DESC").
		Limit(1).
		Scan(&latestAssessmentID).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	if latestAssessmentID == 0 {
		return nil, nil
	}

	// 查询该 Assessment 的所有因子得分
	var pos []*AssessmentScorePO
	err = r.WithContext(ctx).
		Where("assessment_id = ? AND deleted_at IS NULL", latestAssessmentID).
		Order("is_total_score DESC, factor_code ASC").
		Find(&pos).Error

	if err != nil {
		return nil, err
	}

	return r.mapper.ToDomainList(pos), nil
}

// ==================== 删除 ====================

// DeleteByAssessmentID 删除测评的所有得分
func (r *scoreRepository) DeleteByAssessmentID(ctx context.Context, assessmentID assessment.ID) error {
	return r.WithContext(ctx).
		Where("assessment_id = ?", assessmentID.Uint64()).
		Delete(&AssessmentScorePO{}).Error
}

// ==================== 辅助方法 ====================

// toSingleFactorDomainList 将每个 PO 转换为只包含单个因子的 AssessmentScore
// 用于趋势分析查询
func (r *scoreRepository) toSingleFactorDomainList(pos []*AssessmentScorePO) []*assessment.AssessmentScore {
	if len(pos) == 0 {
		return nil
	}

	result := make([]*assessment.AssessmentScore, 0, len(pos))
	for _, po := range pos {
		// 构建单因子得分
		factorScore := assessment.NewFactorScore(
			assessment.FactorCode(po.FactorCode),
			po.FactorName,
			po.RawScore,
			assessment.RiskLevel(po.RiskLevel),
			po.IsTotalScore,
		)

		// 创建只包含单个因子的 AssessmentScore
		score := assessment.ReconstructAssessmentScore(
			assessment.ID(po.AssessmentID),
			po.RawScore,
			assessment.RiskLevel(po.RiskLevel),
			[]assessment.FactorScore{factorScore},
		)
		result = append(result, score)
	}

	return result
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
