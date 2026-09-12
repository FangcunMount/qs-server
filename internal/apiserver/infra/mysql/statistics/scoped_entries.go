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

func (s *ReadStore) ScopedEntries(ctx context.Context, orgID int64, stores authz.StoreRange, entryID, clinicianID *uint64, active *bool, from, to time.Time, page, size int) ([]app.EntryItem, int64, error) {
	if orgID <= 0 || page < 1 || size < 1 || size > 100 {
		return nil, 0, fmt.Errorf("invalid scoped entry query")
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer release()
	var items []app.EntryItem
	var total int64
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		reader := &ReadStore{db: tx}
		base := applyCurrentStoreRange(tx.Table("assessment_entry en").Joins("JOIN clinician c ON c.id=en.clinician_id AND c.org_id=en.org_id").Where("en.org_id=? AND en.deleted_at IS NULL AND c.deleted_at IS NULL", orgID), "c.store_id", stores)
		if entryID != nil {
			base = base.Where("en.id=?", *entryID)
		}
		if clinicianID != nil {
			base = base.Where("en.clinician_id=?", *clinicianID)
		}
		if active != nil {
			base = base.Where("en.is_active=?", *active)
		}
		if err := base.Session(&gorm.Session{}).Count(&total).Error; err != nil {
			return err
		}
		access := reader.scopedFacts(ctx, orgID, stores, "statistics_access_fact").Where("f.stat_date>=? AND f.stat_date<?", from, to).Select("f.entry_id,SUM(fact_type='entry_opened') entry_opened_count,SUM(fact_type='intake_confirmed') intake_confirmed_count").Group("f.entry_id")
		assessments := reader.scopedFacts(ctx, orgID, stores, "statistics_assessment_fact").Where("f.stat_date>=? AND f.stat_date<?", from, to).Select("f.entry_id,SUM(fact_type='assessment_created') assessment_created_count,SUM(fact_type='outcome_committed') outcome_committed_count,SUM(fact_type='report_generated') report_generated_count").Group("f.entry_id")
		return base.Joins("LEFT JOIN (?) a ON a.entry_id=en.id", access).Joins("LEFT JOIN (?) e ON e.entry_id=en.id", assessments).Select(`en.id,en.clinician_id,c.name clinician_name,en.token,en.target_type,en.target_code,COALESCE(en.target_version,'') target_version,en.is_active,en.expires_at,en.created_at,
 COALESCE(a.entry_opened_count,0) entry_opened_count,COALESCE(a.intake_confirmed_count,0) intake_confirmed_count,
 COALESCE(e.assessment_created_count,0) assessment_created_count,COALESCE(e.outcome_committed_count,0) outcome_committed_count,COALESCE(e.report_generated_count,0) report_generated_count`).Order("en.id").Offset((page - 1) * size).Limit(size).Scan(&items).Error
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}
