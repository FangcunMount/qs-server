package statistics

import (
	"context"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"gorm.io/gorm"
)

// scopedPopulationMetrics uses the same current-ownership rule as subject reads.
// ContentCount retains the existing meaning: distinct content used by visible facts.
func (s *ReadStore) scopedPopulationMetrics(ctx context.Context, orgID int64, stores authz.StoreRange) (app.OverviewMetrics, error) {
	result := app.OverviewMetrics{}
	if orgID <= 0 {
		return result, nil
	}
	subjects := func() *gorm.DB {
		return applyCurrentStoreRange(s.db.WithContext(ctx).Table("testee").Where("org_id=? AND deleted_at IS NULL", orgID), "store_id", stores)
	}
	clinicians := func() *gorm.DB {
		return applyCurrentStoreRange(s.db.WithContext(ctx).Table("clinician").Where("org_id=? AND deleted_at IS NULL", orgID), "store_id", stores)
	}
	entries := func() *gorm.DB {
		return applyCurrentStoreRange(s.db.WithContext(ctx).Table("assessment_entry en").Joins("JOIN clinician c ON c.id=en.clinician_id AND c.org_id=en.org_id").Where("en.org_id=? AND en.deleted_at IS NULL AND c.deleted_at IS NULL", orgID), "c.store_id", stores)
	}
	for _, q := range []struct {
		query  *gorm.DB
		target *int64
	}{
		{subjects(), &result.TesteeCount},
		{clinicians(), &result.ClinicianCount},
		{clinicians().Where("is_active=1"), &result.ActiveClinicianCount},
		{entries(), &result.EntryCount},
		{entries().Where("en.is_active=1"), &result.ActiveEntryCount},
		{s.db.WithContext(ctx).Table("plan_enrollment").Where("org_id=? AND status='active' AND deleted_at IS NULL", orgID).Where("testee_id IN (?)", subjects().Select("id")), &result.ActiveEnrollmentCount},
	} {
		if err := q.query.Count(q.target).Error; err != nil {
			return result, err
		}
	}
	// Materialize the small grouped result once; totals and distinct content
	// counts retain database collation semantics without rescanning history.
	groups := s.scopedFacts(ctx, orgID, stores, "statistics_assessment_fact").
		Select("f.fact_type, f.questionnaire_code, f.model_kind, f.model_code, COUNT(*) total").
		Group("f.fact_type, f.questionnaire_code, f.model_kind, f.model_code")
	var history app.OverviewMetrics
	if err := s.db.WithContext(ctx).Raw(`WITH content_groups AS (?)
SELECT
 COALESCE(SUM(CASE WHEN fact_type='answersheet_submitted' THEN total ELSE 0 END),0) answer_sheet_submission_count,
 COALESCE(SUM(CASE WHEN fact_type='assessment_created' THEN total ELSE 0 END),0) assessment_count,
 COALESCE(SUM(CASE WHEN fact_type='report_generated' THEN total ELSE 0 END),0) report_count,
 COUNT(DISTINCT questionnaire_code) + (SELECT COUNT(*) FROM (
   SELECT model_kind,model_code FROM content_groups
   WHERE model_code IS NOT NULL AND model_code<>'' AND model_kind IS NOT NULL
   GROUP BY model_kind,model_code
 ) AS models) content_count
FROM content_groups`, groups).Scan(&history).Error; err != nil {
		return result, err
	}
	result.AnswerSheetSubmissionCount = history.AnswerSheetSubmissionCount
	result.AssessmentCount = history.AssessmentCount
	result.ReportCount = history.ReportCount
	result.ContentCount = history.ContentCount
	return result, nil
}
