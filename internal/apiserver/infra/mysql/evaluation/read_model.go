package evaluation

import (
	"context"
	"fmt"
	"strings"
	"time"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
	"github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"gorm.io/gorm"
)

type assessmentReadModel struct {
	mysql.BaseRepository[*AssessmentPO]
}

const latestRiskRowsQuery = `
SELECT
	ranked.id AS assessment_id,
	ranked.org_id,
	ranked.testee_id,
	ranked.risk_level,
	COALESCE(ranked.interpreted_at, ranked.updated_at, ranked.created_at) AS occurred_at
FROM (
	SELECT
		assessment.*,
		ROW_NUMBER() OVER (
			PARTITION BY assessment.testee_id
			ORDER BY COALESCE(assessment.interpreted_at, assessment.updated_at, assessment.created_at) DESC, assessment.id DESC
		) AS row_num
	FROM assessment
	WHERE assessment.org_id = ?
		AND assessment.testee_id IN ?
		AND assessment.status = ?
		AND assessment.risk_level IS NOT NULL
		AND assessment.risk_level <> ''
		AND assessment.deleted_at IS NULL
) ranked
WHERE ranked.row_num = 1
ORDER BY occurred_at DESC, assessment_id DESC
`

const latestRiskQueueSelectSQL = `
SELECT
	ranked.id AS assessment_id,
	ranked.org_id,
	ranked.testee_id,
	ranked.risk_level,
	COALESCE(ranked.interpreted_at, ranked.updated_at, ranked.created_at) AS occurred_at
FROM (
	SELECT
		assessment.*,
		ROW_NUMBER() OVER (
			PARTITION BY assessment.testee_id
			ORDER BY COALESCE(assessment.interpreted_at, assessment.updated_at, assessment.created_at) DESC, assessment.id DESC
		) AS row_num
	FROM assessment
	WHERE assessment.org_id = ?
		%s
		AND assessment.status = ?
		AND assessment.risk_level IS NOT NULL
		AND assessment.risk_level <> ''
		AND assessment.deleted_at IS NULL
) ranked
WHERE ranked.row_num = 1
	AND LOWER(ranked.risk_level) IN ?
`

func NewAssessmentReadModel(db *gorm.DB, opts ...mysql.BaseRepositoryOptions) interface {
	evaluationreadmodel.AssessmentReader
	evaluationreadmodel.LatestRiskReader
} {
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

func (r *assessmentReadModel) ListLatestRisksByTesteeIDs(
	ctx context.Context,
	filter evaluationreadmodel.LatestRiskFilter,
) ([]evaluationreadmodel.LatestRiskRow, error) {
	if len(filter.TesteeIDs) == 0 {
		return []evaluationreadmodel.LatestRiskRow{}, nil
	}

	var rows []latestRiskPO
	err := r.WithContext(ctx).
		Raw(latestRiskRowsQuery, filter.OrgID, uniqueUint64(filter.TesteeIDs), "interpreted").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	return latestRiskRowsFromPOs(rows), nil
}

func (r *assessmentReadModel) ListLatestRiskQueue(
	ctx context.Context,
	filter evaluationreadmodel.LatestRiskQueueFilter,
	page evaluationreadmodel.PageRequest,
) (evaluationreadmodel.LatestRiskPage, error) {
	if filter.RestrictToTesteeIDs && len(filter.TesteeIDs) == 0 {
		return evaluationreadmodel.LatestRiskPage{
			Items:    []evaluationreadmodel.LatestRiskRow{},
			Page:     normalizedLatestRiskPage(page.Page),
			PageSize: page.Limit(),
		}, nil
	}

	args := latestRiskQueueArgs(filter)
	var total int64
	if err := r.WithContext(ctx).
		Raw(latestRiskQueueCountQuery(filter.RestrictToTesteeIDs), args...).
		Scan(&total).Error; err != nil {
		return evaluationreadmodel.LatestRiskPage{}, err
	}

	rowArgs := append(args, page.Limit(), page.Offset())
	var rows []latestRiskPO
	if err := r.WithContext(ctx).
		Raw(latestRiskQueueRowsQuery(filter.RestrictToTesteeIDs), rowArgs...).
		Scan(&rows).Error; err != nil {
		return evaluationreadmodel.LatestRiskPage{}, err
	}

	return evaluationreadmodel.LatestRiskPage{
		Items:    latestRiskRowsFromPOs(rows),
		Total:    total,
		Page:     normalizedLatestRiskPage(page.Page),
		PageSize: page.Limit(),
	}, nil
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

type latestRiskPO struct {
	AssessmentID uint64    `gorm:"column:assessment_id"`
	OrgID        int64     `gorm:"column:org_id"`
	TesteeID     uint64    `gorm:"column:testee_id"`
	RiskLevel    string    `gorm:"column:risk_level"`
	OccurredAt   time.Time `gorm:"column:occurred_at"`
}

func latestRiskQueueRowsQuery(restrictToTesteeIDs bool) string {
	return latestRiskQueueSelect(restrictToTesteeIDs) + `
ORDER BY occurred_at DESC, assessment_id DESC
LIMIT ? OFFSET ?
`
}

func latestRiskQueueCountQuery(restrictToTesteeIDs bool) string {
	return `SELECT COUNT(*) FROM (` + latestRiskQueueSelect(restrictToTesteeIDs) + `) latest_risk_queue`
}

func latestRiskQueueSelect(restrictToTesteeIDs bool) string {
	testeePredicate := ""
	if restrictToTesteeIDs {
		testeePredicate = "AND assessment.testee_id IN ?"
	}
	return fmt.Sprintf(latestRiskQueueSelectSQL, testeePredicate)
}

func latestRiskQueueArgs(filter evaluationreadmodel.LatestRiskQueueFilter) []interface{} {
	riskLevels := normalizeRiskLevels(filter.RiskLevels)
	args := []interface{}{filter.OrgID}
	if filter.RestrictToTesteeIDs {
		args = append(args, uniqueUint64(filter.TesteeIDs))
	}
	args = append(args, "interpreted", riskLevels)
	return args
}

func normalizeRiskLevels(values []string) []string {
	if len(values) == 0 {
		return []string{"high", "severe"}
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	if len(result) == 0 {
		return []string{"high", "severe"}
	}
	return result
}

func latestRiskRowsFromPOs(rows []latestRiskPO) []evaluationreadmodel.LatestRiskRow {
	result := make([]evaluationreadmodel.LatestRiskRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, evaluationreadmodel.LatestRiskRow{
			AssessmentID: row.AssessmentID,
			OrgID:        row.OrgID,
			TesteeID:     row.TesteeID,
			RiskLevel:    strings.ToLower(row.RiskLevel),
			OccurredAt:   row.OccurredAt,
		})
	}
	return result
}

func normalizedLatestRiskPage(page int) int {
	if page < 1 {
		return 1
	}
	return page
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
	query := buildFactorTrendQuery(r.WithContext(ctx), filter)

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

func buildFactorTrendQuery(db *gorm.DB, filter evaluationreadmodel.FactorTrendFilter) *gorm.DB {
	query := db.
		Where("testee_id = ? AND factor_code = ? AND deleted_at IS NULL", filter.TesteeID, filter.FactorCode).
		Order("id DESC")
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	}
	return query
}

func uniqueUint64(items []uint64) []uint64 {
	if len(items) == 0 {
		return []uint64{}
	}
	seen := make(map[uint64]struct{}, len(items))
	result := make([]uint64, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
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
