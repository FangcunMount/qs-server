package evaluation

import (
	"context"

	actorreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/actorreadmodel"
	"gorm.io/gorm"
)

const assessmentSummarySQL = `
SELECT ranked.testee_id,
       ranked.total_evaluated,
       ranked.evaluated_at AS last_evaluated_at,
       COALESCE(ranked.risk_level, '') AS risk_level
FROM (
    SELECT assessment.testee_id,
           assessment.evaluated_at,
           assessment.risk_level,
           COUNT(*) OVER (PARTITION BY assessment.testee_id) AS total_evaluated,
           ROW_NUMBER() OVER (
               PARTITION BY assessment.testee_id
               ORDER BY assessment.evaluated_at DESC, assessment.id DESC
           ) AS row_num
    FROM assessment FORCE INDEX (idx_assessment_org_testee_evaluated_summary)
    WHERE assessment.org_id = ?
      AND assessment.testee_id IN ?
      AND assessment.status = 'evaluated'
      AND assessment.deleted_at IS NULL
) ranked
WHERE ranked.row_num = 1`

type assessmentSummaryReader struct{ db *gorm.DB }

func NewAssessmentSummaryReader(db *gorm.DB) actorreadmodel.AssessmentSummaryReader {
	return &assessmentSummaryReader{db: db}
}

func (r *assessmentSummaryReader) ReadAssessmentSummaries(ctx context.Context, orgID int64, testeeIDs []uint64) (map[uint64]actorreadmodel.AssessmentSummary, error) {
	result := make(map[uint64]actorreadmodel.AssessmentSummary, len(testeeIDs))
	if len(testeeIDs) == 0 {
		return result, nil
	}
	var rows []actorreadmodel.AssessmentSummary
	if err := r.db.WithContext(ctx).Raw(assessmentSummarySQL, orgID, uniqueSummaryIDs(testeeIDs)).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.TesteeID] = row
	}
	return result, nil
}

func uniqueSummaryIDs(ids []uint64) []uint64 {
	seen := make(map[uint64]struct{}, len(ids))
	result := make([]uint64, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}
