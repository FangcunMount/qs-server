package statistics

import (
	"context"
	"database/sql"
	"fmt"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/statistics"
	"gorm.io/gorm"
	"time"
)

func (s *ReadStore) ScopedClinicians(ctx context.Context, orgID int64, stores authz.StoreRange, clinicianID *uint64, operatorUserID *int64, from, to time.Time, page, size int) ([]app.ClinicianItem, int64, error) {
	if orgID <= 0 || page < 1 || size < 1 || size > 100 {
		return nil, 0, fmt.Errorf("invalid scoped clinician query")
	}
	if operatorUserID != nil {
		return nil, 0, fmt.Errorf("clinician operator binding is retired")
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer release()
	var items []app.ClinicianItem
	var total int64
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		reader := &ReadStore{db: tx}
		base := applyCurrentStoreRange(tx.Table("clinician c").Where("c.org_id=? AND c.deleted_at IS NULL", orgID), "c.store_id", stores)
		if clinicianID != nil {
			base = base.Where("c.id=?", *clinicianID)
		}
		if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			return err
		}
		access := reader.scopedFacts(ctx, orgID, stores, "statistics_access_fact").Where("f.stat_date>=? AND f.stat_date<?", from, to).Select("f.clinician_id,SUM(fact_type='entry_opened') entry_opened_count,SUM(fact_type='intake_confirmed') intake_confirmed_count,SUM(fact_type='care_relationship_established') care_relationship_established_count").Group("f.clinician_id")
		assessments := reader.scopedFacts(ctx, orgID, stores, "statistics_assessment_fact").Where("f.stat_date>=? AND f.stat_date<?", from, to).Select("f.clinician_id,SUM(fact_type='assessment_created') assessment_created_count,SUM(fact_type='outcome_committed') outcome_committed_count,SUM(fact_type='report_generated') report_generated_count").Group("f.clinician_id")
		owners := applyCurrentStoreRange(tx.Table("testee").Select("id").Where("org_id=? AND deleted_at IS NULL", orgID), "store_id", stores)
		relations := tx.Table("clinician_relation").Where("org_id=? AND is_active=1 AND deleted_at IS NULL", orgID).Where("testee_id IN (?)", owners).Select(`clinician_id,COUNT(DISTINCT CASE WHEN relation_type='primary' THEN testee_id END) primary_testee_count,COUNT(DISTINCT CASE WHEN relation_type='attending' THEN testee_id END) attending_testee_count,COUNT(DISTINCT CASE WHEN relation_type='collaborator' THEN testee_id END) collaborator_testee_count,COUNT(DISTINCT testee_id) total_accessible_testees`).Group("clinician_id")
		entries := tx.Table("assessment_entry").Where("org_id=? AND is_active=1 AND deleted_at IS NULL", orgID).Select("clinician_id,COUNT(*) active_entry_count").Group("clinician_id")
		return base.Joins("LEFT JOIN (?) a ON a.clinician_id=c.id", access).Joins("LEFT JOIN (?) e ON e.clinician_id=c.id", assessments).Joins("LEFT JOIN (?) r ON r.clinician_id=c.id", relations).Joins("LEFT JOIN (?) en ON en.clinician_id=c.id", entries).Select(`c.id,c.name,c.department,c.title,c.clinician_type,c.is_active,
 COALESCE(a.entry_opened_count,0) entry_opened_count,COALESCE(a.intake_confirmed_count,0) intake_confirmed_count,COALESCE(a.care_relationship_established_count,0) care_relationship_established_count,
 COALESCE(e.assessment_created_count,0) assessment_created_count,COALESCE(e.outcome_committed_count,0) outcome_committed_count,COALESCE(e.report_generated_count,0) report_generated_count,
 COALESCE(r.primary_testee_count,0) primary_testee_count,COALESCE(r.attending_testee_count,0) attending_testee_count,COALESCE(r.collaborator_testee_count,0) collaborator_testee_count,COALESCE(r.total_accessible_testees,0) total_accessible_testees,COALESCE(en.active_entry_count,0) active_entry_count`).Order("c.id").Offset((page - 1) * size).Limit(size).Scan(&items).Error
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}
