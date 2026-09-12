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
	facts := func() *gorm.DB { return s.scopedFacts(ctx, orgID, stores, "statistics_assessment_fact") }
	for _, q := range []struct {
		kind   string
		target *int64
	}{{"answersheet_submitted", &result.AnswerSheetSubmissionCount}, {"assessment_created", &result.AssessmentCount}, {"report_generated", &result.ReportCount}} {
		if err := facts().Where("f.fact_type=?", q.kind).Count(q.target).Error; err != nil {
			return result, err
		}
	}
	var questionnaires, models int64
	if err := facts().Distinct("f.questionnaire_code").Count(&questionnaires).Error; err != nil {
		return result, err
	}
	modelPairs := facts().Where("f.model_code IS NOT NULL AND f.model_code<>'' AND f.model_kind IS NOT NULL").Select("f.model_kind,f.model_code").Group("f.model_kind,f.model_code")
	if err := s.db.WithContext(ctx).Table("(?) AS models", modelPairs).Count(&models).Error; err != nil {
		return result, err
	}
	result.ContentCount = questionnaires + models
	return result, nil
}
