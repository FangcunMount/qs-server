package testeestore

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"gorm.io/gorm"
)

// MigrationRequestID fits the 64-character request key without truncating caller IDs.
func MigrationRequestID(id string) string {
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:])
}

type migrationHistory struct {
	ID, TesteeID, ToStoreID uint64
	OrgID, ActorID          int64
	FromStoreID             *uint64
	Kind, RequestID         string
	Version                 uint32
}

// validateHistory reads bounded batches; the original audit must survive later drift or rollback.
func validateHistory(db *gorm.DB, manifest Manifest, items []MigrationItem) error {
	const batchSize = 500
	expected := make(map[uint64]MigrationItem, batchSize)
	ids := make([]uint64, 0, batchSize)
	flush := func() error {
		if len(ids) == 0 {
			return nil
		}
		rows := []migrationHistory{}
		if err := db.Table("testee_store_history").Where("id IN ?", ids).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) != len(ids) {
			return fmt.Errorf("migration ownership history missing")
		}
		for _, row := range rows {
			item, ok := expected[row.ID]
			if !ok || row.TesteeID != item.TesteeID || row.OrgID != item.OrgID || row.ActorID != manifest.ActorID || row.FromStoreID != nil || item.TargetStoreID == nil || row.ToStoreID != *item.TargetStoreID || row.Version != item.AppliedVersion || row.Kind != "migration_initial" || row.RequestID != MigrationRequestID(manifest.MigrationID) {
				return fmt.Errorf("migration ownership history mismatch")
			}
		}
		clear(expected)
		ids = ids[:0]
		return nil
	}
	for _, item := range items {
		if item.Disposition != "candidate" {
			continue
		}
		if item.HistoryID == nil {
			return fmt.Errorf("candidate history ID missing")
		}
		if _, duplicate := expected[*item.HistoryID]; duplicate {
			return fmt.Errorf("duplicate migration history ID")
		}
		ids = append(ids, *item.HistoryID)
		expected[*item.HistoryID] = item
		if len(ids) == batchSize {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}

func validateAppliedOwnership(db *gorm.DB, id string) error {
	var mismatches int64
	err := db.Raw(`SELECT COUNT(*) FROM testee_store_migration_items i
 LEFT JOIN testee t ON t.id=i.testee_id AND t.org_id=i.org_id AND t.deleted_at IS NULL
 WHERE i.migration_id=? AND i.disposition='candidate'
 AND (t.id IS NULL OR NOT(t.store_id <=> i.target_store_id) OR t.store_version<>i.applied_version)`, id).Scan(&mismatches).Error
	if err != nil {
		return err
	}
	if mismatches > 0 {
		return fmt.Errorf("completed migration has inconsistent current ownership")
	}
	return nil
}
