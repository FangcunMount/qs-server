package evaluation

import (
	"context"
	"strings"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

type assessmentReadModel struct {
	mysql.BaseRepository[*AssessmentPO]
}

func NewAssessmentReadModel(db *gorm.DB, opts ...mysql.BaseRepositoryOptions) evaluationreadmodel.AssessmentReader {
	return &assessmentReadModel{
		BaseRepository: mysql.NewBaseRepository[*AssessmentPO](db, opts...),
	}
}

func (r *assessmentReadModel) GetAssessment(ctx context.Context, id uint64) (*evaluationreadmodel.AssessmentRow, error) {
	var po AssessmentPO
	err := r.WithContext(ctx).
		Where("id = ? AND deleted_at IS NULL", id).
		First(&po).Error
	if err != nil {
		if cberrors.Is(err, gorm.ErrRecordNotFound) {
			return nil, cberrors.WithCode(code.ErrAssessmentNotFound, "assessment not found")
		}
		return nil, err
	}
	row := assessmentPOToReadRow(&po)
	return &row, nil
}

func (r *assessmentReadModel) GetAssessmentByAnswerSheetID(ctx context.Context, answerSheetID uint64) (*evaluationreadmodel.AssessmentRow, error) {
	var po AssessmentPO
	err := r.WithContext(ctx).
		Where("answer_sheet_id = ? AND deleted_at IS NULL", answerSheetID).
		First(&po).Error
	if err != nil {
		if cberrors.Is(err, gorm.ErrRecordNotFound) {
			return nil, cberrors.WithCode(code.ErrAssessmentNotFound, "assessment not found")
		}
		return nil, err
	}
	row := assessmentPOToReadRow(&po)
	return &row, nil
}

func (r *assessmentReadModel) ListAssessments(
	ctx context.Context,
	filter evaluationreadmodel.AssessmentFilter,
	page evaluationreadmodel.PageRequest,
) ([]evaluationreadmodel.AssessmentRow, int64, error) {
	query := r.WithContext(ctx).
		Model(&AssessmentPO{}).
		Where("deleted_at IS NULL")

	if filter.RestrictToAccessScope {
		if len(filter.AccessibleTesteeIDs) == 0 {
			return []evaluationreadmodel.AssessmentRow{}, 0, nil
		}
	}
	query = applyAssessmentReadModelFilter(query, filter)

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var pos []*AssessmentPO
	if err := query.
		Order("id DESC").
		Offset(page.Offset()).
		Limit(page.Limit()).
		Find(&pos).Error; err != nil {
		return nil, 0, err
	}

	rows := make([]evaluationreadmodel.AssessmentRow, 0, len(pos))
	for _, po := range pos {
		rows = append(rows, assessmentPOToReadRow(po))
	}
	return rows, total, nil
}

func applyAssessmentReadModelFilter(query *gorm.DB, filter evaluationreadmodel.AssessmentFilter) *gorm.DB {
	if filter.OrgID != 0 {
		query = query.Where("org_id = ?", filter.OrgID)
	}
	if filter.TesteeID != nil {
		query = query.Where("testee_id = ?", *filter.TesteeID)
	}
	if filter.RestrictToAccessScope {
		query = query.Where("testee_id IN ?", filter.AccessibleTesteeIDs)
	}
	if len(filter.Statuses) > 0 {
		query = query.Where("status IN ?", filter.Statuses)
	}
	if filter.ScaleCode != "" {
		query = query.Where("medical_scale_code = ?", filter.ScaleCode)
	}
	if filter.RiskLevel != "" {
		query = query.Where("risk_level = ?", strings.ToLower(filter.RiskLevel))
	}
	if filter.DateFrom != nil {
		query = query.Where("created_at >= ?", *filter.DateFrom)
	}
	if filter.DateTo != nil {
		query = query.Where("created_at < ?", *filter.DateTo)
	}
	return query
}

func assessmentPOToReadRow(po *AssessmentPO) evaluationreadmodel.AssessmentRow {
	if po == nil {
		return evaluationreadmodel.AssessmentRow{}
	}
	return evaluationreadmodel.AssessmentRow{
		ID:                   po.ID.Uint64(),
		OrgID:                po.OrgID,
		TesteeID:             po.TesteeID,
		QuestionnaireCode:    po.QuestionnaireCode,
		QuestionnaireVersion: po.QuestionnaireVersion,
		AnswerSheetID:        po.AnswerSheetID,
		MedicalScaleID:       po.MedicalScaleID,
		MedicalScaleCode:     po.MedicalScaleCode,
		MedicalScaleName:     po.MedicalScaleName,
		OriginType:           po.OriginType,
		OriginID:             po.OriginID,
		Status:               po.Status,
		TotalScore:           po.TotalScore,
		RiskLevel:            po.RiskLevel,
		SubmittedAt:          po.SubmittedAt,
		InterpretedAt:        po.InterpretedAt,
		FailedAt:             po.FailedAt,
		FailureReason:        po.FailureReason,
	}
}

type scoreReadModel struct {
	mysql.BaseRepository[*AssessmentScorePO]
}

func NewScoreReadModel(db *gorm.DB, opts ...mysql.BaseRepositoryOptions) evaluationreadmodel.ScoreReader {
	return &scoreReadModel{
		BaseRepository: mysql.NewBaseRepository[*AssessmentScorePO](db, opts...),
	}
}

func (r *scoreReadModel) GetScoreByAssessmentID(ctx context.Context, assessmentID uint64) (*evaluationreadmodel.ScoreRow, error) {
	var pos []*AssessmentScorePO
	err := r.WithContext(ctx).
		Where("assessment_id = ? AND deleted_at IS NULL", assessmentID).
		Order("is_total_score DESC, factor_code ASC").
		Find(&pos).Error
	if err != nil {
		return nil, err
	}
	if len(pos) == 0 {
		return nil, cberrors.WithCode(code.ErrAssessmentScoreNotFound, "score not found")
	}
	row := scorePOsToReadRow(pos)
	return &row, nil
}

func (r *scoreReadModel) ListFactorTrend(ctx context.Context, filter evaluationreadmodel.FactorTrendFilter) ([]evaluationreadmodel.ScoreRow, error) {
	query := r.WithContext(ctx).
		Where("testee_id = ? AND factor_code = ? AND deleted_at IS NULL", filter.TesteeID, filter.FactorCode).
		Order("id DESC")
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}

	var pos []*AssessmentScorePO
	if err := query.Find(&pos).Error; err != nil {
		return nil, err
	}

	rows := make([]evaluationreadmodel.ScoreRow, 0, len(pos))
	for _, po := range pos {
		rows = append(rows, scorePOsToReadRow([]*AssessmentScorePO{po}))
	}
	return rows, nil
}

func scorePOsToReadRow(pos []*AssessmentScorePO) evaluationreadmodel.ScoreRow {
	row := evaluationreadmodel.ScoreRow{}
	if len(pos) == 0 {
		return row
	}
	first := pos[0]
	row.AssessmentID = first.AssessmentID
	row.MedicalScaleID = &first.MedicalScaleID
	row.MedicalScaleCode = &first.MedicalScaleCode
	row.RiskLevel = first.RiskLevel
	row.TotalScore = first.RawScore
	row.FactorScores = make([]evaluationreadmodel.ScoreFactorRow, 0, len(pos))
	for _, po := range pos {
		factor := evaluationreadmodel.ScoreFactorRow{
			FactorCode:   po.FactorCode,
			FactorName:   po.FactorName,
			RawScore:     po.RawScore,
			RiskLevel:    po.RiskLevel,
			Conclusion:   po.Conclusion,
			Suggestion:   po.Suggestion,
			IsTotalScore: po.IsTotalScore,
		}
		row.FactorScores = append(row.FactorScores, factor)
		if po.IsTotalScore {
			row.TotalScore = po.RawScore
			row.RiskLevel = po.RiskLevel
		}
	}
	return row
}
