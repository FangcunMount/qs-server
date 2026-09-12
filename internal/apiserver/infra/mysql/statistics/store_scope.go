package statistics

import (
	"context"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"gorm.io/gorm"
)

// scopedFacts filters before aggregation. Clinical history follows the Testee's
// current ownership; the historical clinician is never an alternative grant.
func (s *ReadStore) scopedFacts(ctx context.Context, orgID int64, stores authz.StoreRange, table string) *gorm.DB {
	switch table {
	case "statistics_access_fact", "statistics_assessment_fact", "statistics_plan_fact":
	default:
		return s.db.WithContext(ctx).Table("statistics_assessment_fact AS f").Where("1=0")
	}
	query := s.db.WithContext(ctx).Table(table+" AS f").Where("f.org_id=?", orgID)
	if orgID <= 0 {
		return query.Where("1=0")
	}
	owners := s.db.WithContext(ctx).Table("testee").Select("id").Where("org_id=? AND deleted_at IS NULL", orgID)
	owners = applyCurrentStoreRange(owners, "store_id", stores)
	if table != "statistics_access_fact" {
		return query.Where("f.testee_id IN (?)", owners)
	}
	// Opening an entry precedes identifying a Testee. Only this fact type may
	// use entry ownership; intake and all clinical facts must match a Testee.
	entries := s.db.WithContext(ctx).Table("assessment_entry AS en").Select("en.id").Joins("JOIN clinician c ON c.id=en.clinician_id AND c.org_id=en.org_id").Where("en.org_id=? AND en.deleted_at IS NULL AND c.deleted_at IS NULL", orgID)
	entries = applyCurrentStoreRange(entries, "c.store_id", stores)
	return query.Where("f.testee_id IN (?) OR (f.fact_type=? AND (f.testee_id IS NULL OR f.testee_id=0) AND f.entry_id IN (?))", owners, "entry_opened", entries)
}

func applyCurrentStoreRange(query *gorm.DB, column string, stores authz.StoreRange) *gorm.DB {
	// column is an internal constant supplied by this file, never request input.
	if stores.AllStores {
		return query.Where(column + " IS NOT NULL AND " + column + ">0")
	}
	return query.Where(column+" IN ?", stores.StoreIDs)
}
