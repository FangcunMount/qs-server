package evaluation

import (
	"context"
	"fmt"
	"strings"
	"time"

	cberrors "github.com/FangcunMount/component-base/pkg/errors"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/workbenchreadmodel"
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
	COALESCE(ranked.evaluated_at, ranked.updated_at, ranked.created_at) AS occurred_at
FROM (
	SELECT
		assessment.*,
		ROW_NUMBER() OVER (
			PARTITION BY assessment.testee_id
			ORDER BY COALESCE(assessment.evaluated_at, assessment.updated_at, assessment.created_at) DESC, assessment.id DESC
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

const latestRiskQueueCoreSQL = `
FROM (
	SELECT
		assessment.testee_id,
		MAX(assessment.id) AS latest_id
	FROM assessment FORCE INDEX (idx_assessment_workbench_latest_id_risk_by_testee)
	WHERE assessment.org_id = ?
		%s
		AND assessment.status = ?
		AND assessment.deleted_at IS NULL
		AND assessment.risk_level IS NOT NULL
		AND assessment.risk_level <> ''
	GROUP BY assessment.testee_id
) latest
JOIN assessment a ON a.id = latest.latest_id
WHERE a.org_id = ?
	AND a.status = ?
	AND a.deleted_at IS NULL
	AND a.risk_level IN ?
	`

const latestRiskQueueSelectSQL = `
SELECT
	a.id AS assessment_id,
	a.org_id,
	a.testee_id,
	a.risk_level,
	COALESCE(a.evaluated_at, a.updated_at, a.created_at) AS occurred_at
` + latestRiskQueueCoreSQL

const latestRiskQueueCountSQL = `
SELECT COUNT(*)
` + latestRiskQueueCoreSQL

const latestRiskQueueRestrictedTesteePredicate = `
		AND assessment.testee_id IN ?`

const latestRiskQueueUnrestrictedTesteePredicate = ``

func NewAssessmentReadModel(db *gorm.DB, opts ...mysql.BaseRepositoryOptions) interface {
	evaluationreadmodel.AssessmentReader
	workbenchreadmodel.LatestRiskReader
	ListSubmittedAssessmentIDsAfter(context.Context, uint64, int) ([]uint64, error)
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

// ListSubmittedAssessmentIDsAfter implements the Evaluation scheduler's
// stable keyset scan without exposing maintenance pagination to actor queries.
func (r *assessmentReadModel) ListSubmittedAssessmentIDsAfter(ctx context.Context, afterID uint64, limit int) ([]uint64, error) {
	if limit <= 0 {
		return []uint64{}, nil
	}
	ids := make([]uint64, 0, limit)
	err := r.WithContext(ctx).
		Model(&AssessmentPO{}).
		Where("status = ? AND id > ? AND deleted_at IS NULL", "submitted", afterID).
		Order("id ASC").
		Limit(limit).
		Pluck("id", &ids).Error
	return ids, err
}

func (r *assessmentReadModel) ListLatestRisksByTesteeIDs(
	ctx context.Context,
	filter workbenchreadmodel.LatestRiskFilter,
) ([]workbenchreadmodel.LatestRiskRow, error) {
	if len(filter.TesteeIDs) == 0 {
		return []workbenchreadmodel.LatestRiskRow{}, nil
	}

	var rows []latestRiskPO
	err := r.WithContext(ctx).
		Raw(latestRiskRowsQuery, filter.OrgID, uniqueUint64(filter.TesteeIDs), "evaluated").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	return latestRiskRowsFromPOs(rows), nil
}

func (r *assessmentReadModel) ListLatestRiskQueue(
	ctx context.Context,
	filter workbenchreadmodel.LatestRiskQueueFilter,
	page workbenchreadmodel.PageRequest,
) (workbenchreadmodel.LatestRiskPage, error) {
	if filter.RestrictToTesteeIDs && len(filter.TesteeIDs) == 0 {
		return workbenchreadmodel.LatestRiskPage{
			Items:    []workbenchreadmodel.LatestRiskRow{},
			Page:     normalizedLatestRiskPage(page.Page),
			PageSize: page.Limit(),
		}, nil
	}

	args := latestRiskQueueArgs(filter)
	var total int64
	if err := r.WithContext(ctx).
		Raw(latestRiskQueueCountQuery(filter.RestrictToTesteeIDs), args...).
		Scan(&total).Error; err != nil {
		return workbenchreadmodel.LatestRiskPage{}, err
	}

	rowArgs := append(args, page.Limit(), page.Offset())
	var rows []latestRiskPO
	if err := r.WithContext(ctx).
		Raw(latestRiskQueueRowsQuery(filter.RestrictToTesteeIDs), rowArgs...).
		Scan(&rows).Error; err != nil {
		return workbenchreadmodel.LatestRiskPage{}, err
	}

	return workbenchreadmodel.LatestRiskPage{
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
		query = query.Where("evaluation_model_kind = ? AND evaluation_model_code = ?", "scale", filter.ScaleCode)
	}
	if filter.ModelKind != "" {
		query = query.Where("evaluation_model_kind = ?", filter.ModelKind)
	}
	if len(filter.ModelKinds) > 0 {
		query = query.Where("evaluation_model_kind IN ?", filter.ModelKinds)
	}
	if filter.ModelAlgorithm != "" {
		query = query.Where("evaluation_model_algorithm = ?", filter.ModelAlgorithm)
	}
	if filter.ModelCode != "" {
		query = query.Where("evaluation_model_code = ?", filter.ModelCode)
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
	return fmt.Sprintf(latestRiskQueueCountSQL, latestRiskQueueTesteePredicate(restrictToTesteeIDs))
}

func latestRiskQueueSelect(restrictToTesteeIDs bool) string {
	return fmt.Sprintf(latestRiskQueueSelectSQL, latestRiskQueueTesteePredicate(restrictToTesteeIDs))
}

func latestRiskQueueTesteePredicate(restrictToTesteeIDs bool) string {
	if restrictToTesteeIDs {
		return latestRiskQueueRestrictedTesteePredicate
	}
	return latestRiskQueueUnrestrictedTesteePredicate
}

func latestRiskQueueArgs(filter workbenchreadmodel.LatestRiskQueueFilter) []interface{} {
	riskLevels := normalizeRiskLevels(filter.RiskLevels)
	args := []interface{}{filter.OrgID}
	if filter.RestrictToTesteeIDs {
		args = append(args, uniqueUint64(filter.TesteeIDs))
	}
	args = append(args, "evaluated", filter.OrgID, "evaluated", riskLevels)
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

func latestRiskRowsFromPOs(rows []latestRiskPO) []workbenchreadmodel.LatestRiskRow {
	result := make([]workbenchreadmodel.LatestRiskRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, workbenchreadmodel.LatestRiskRow{
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
		ID:                       po.ID.Uint64(),
		OrgID:                    po.OrgID,
		TesteeID:                 po.TesteeID,
		QuestionnaireCode:        po.QuestionnaireCode,
		QuestionnaireVersion:     po.QuestionnaireVersion,
		AnswerSheetID:            po.AnswerSheetID,
		EvaluationModelKind:      po.EvaluationModelKind,
		EvaluationModelSubKind:   po.EvaluationModelSubKind,
		EvaluationModelAlgorithm: po.EvaluationModelAlgorithm,
		EvaluationModelCode:      po.EvaluationModelCode,
		EvaluationModelVersion:   po.EvaluationModelVersion,
		EvaluationModelTitle:     po.EvaluationModelTitle,
		PrimaryScoreKind:         po.PrimaryScoreKind,
		PrimaryScoreValue:        po.PrimaryScoreValue,
		PrimaryScoreLabel:        po.PrimaryScoreLabel,
		PrimaryScoreMax:          po.PrimaryScoreMax,
		LevelCode:                po.LevelCode,
		LevelLabel:               po.LevelLabel,
		Severity:                 po.Severity,
		OriginType:               po.OriginType,
		OriginID:                 po.OriginID,
		Status:                   po.Status,
		TotalScore:               po.TotalScore,
		RiskLevel:                po.RiskLevel,
		SubmittedAt:              po.SubmittedAt,
		EvaluatedAt:              po.EvaluatedAt,
		FailedAt:                 po.FailedAt,
		FailureReason:            po.FailureReason,
	}
}

type scoreReadModel struct {
	mysql.BaseRepository[*AssessmentScorePO]
}

func NewScoreProjectionReadModel(db *gorm.DB, opts ...mysql.BaseRepositoryOptions) evaluationreadmodel.ScoreProjectionReader {
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
	row.RiskLevel = first.RiskLevel
	row.TotalScore = first.RawScore
	row.FactorScores = make([]evaluationreadmodel.ScoreFactorRow, 0, len(pos))
	for _, po := range pos {
		factor := evaluationreadmodel.ScoreFactorRow{
			FactorCode:   po.FactorCode,
			FactorName:   po.FactorName,
			RawScore:     po.RawScore,
			RiskLevel:    po.RiskLevel,
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
