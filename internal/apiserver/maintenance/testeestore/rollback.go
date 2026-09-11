package testeestore

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Reversion records compensation to the original unassigned state. Original
// migration items and ownership history remain immutable; versions never decrease.
type Reversion struct {
	MigrationID       string
	TesteeID          uint64
	OrgID             int64
	FromStoreID       uint64
	OriginalHistoryID uint64
	RestoredVersion   uint32
	ActorID           int64
	CreatedAt         time.Time
}

func (Reversion) TableName() string { return "testee_store_migration_reversions" }

func validateReversions(tx *gorm.DB, manifest Manifest, items []MigrationItem) error {
	var count int64
	if err := tx.Model(&Reversion{}).Where("migration_id=?", manifest.MigrationID).Count(&count).Error; err != nil {
		return err
	}
	if manifest.State != "rolled_back" {
		if count != 0 {
			return fmt.Errorf("unexpected migration compensation history")
		}
		return nil
	}
	expected := make(map[uint64]MigrationItem)
	for _, item := range items {
		if item.Disposition == "candidate" {
			expected[item.TesteeID] = item
		}
	}
	if int64(len(expected)) != count {
		return fmt.Errorf("compensation history count mismatch")
	}
	var cursor uint64
	for {
		rows := []Reversion{}
		if err := tx.Where("migration_id=? AND testee_id>?", manifest.MigrationID, cursor).Order("testee_id").Limit(500).Find(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			item, ok := expected[row.TesteeID]
			if !ok || item.TargetStoreID == nil || item.HistoryID == nil || row.OrgID != item.OrgID || row.FromStoreID != *item.TargetStoreID || row.OriginalHistoryID != *item.HistoryID || row.RestoredVersion == 0 || row.RestoredVersion != item.AppliedVersion+1 || row.ActorID <= 0 || row.CreatedAt.IsZero() {
				return fmt.Errorf("compensation history mismatch")
			}
			cursor = row.TesteeID
		}
		if len(rows) < 500 {
			break
		}
	}
	return nil
}

// Rollback compensates only this migration's initial assignments. Drift is never
// overwritten, and a repeated compensation is read-only when facts still agree.
func Rollback(ctx context.Context, db *gorm.DB, c ApplyCommand) (*Manifest, error) {
	if err := authorizeApply(ctx, c); err != nil {
		return nil, err
	}
	var result *Manifest
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockFacts(tx, c.MaxRows); err != nil {
			return err
		}
		var operators int64
		if err := tx.Table("operators").Where("org_id=? AND user_id=? AND is_active=TRUE AND deleted_at IS NULL", c.OrgID, c.ActorID).Count(&operators).Error; err != nil {
			return err
		}
		if operators != 1 {
			return fmt.Errorf("active company operator required")
		}
		report, err := preflightInTx(tx, c.MaxRows)
		if err != nil {
			return err
		}
		manifest, items, err := loadManifest(tx, c.MigrationID, c.MaxRows)
		if err != nil {
			return err
		}
		if manifest == nil {
			return fmt.Errorf("migration has not been applied")
		}
		state, err := Classify(c.MigrationID, manifest, items, report.Fingerprint)
		if err != nil {
			return err
		}
		if c.Fingerprint != manifest.BeforeHash {
			return fmt.Errorf("original migration fingerprint mismatch")
		}
		for _, item := range items {
			if item.OrgID != c.OrgID {
				return fmt.Errorf("migration contains another company")
			}
		}
		if err = validateHistory(tx, *manifest, items); err != nil {
			return err
		}
		if err = validateReversions(tx, *manifest, items); err != nil {
			return err
		}
		if state.State == "rolled_back" {
			if report.Fingerprint != manifest.RollbackHash {
				return fmt.Errorf("facts changed after rollback")
			}
			if err = validateRestoredOwnership(tx, c.MigrationID); err != nil {
				return err
			}
			result = manifest
			return nil
		}
		if state.State != "applied_unchanged" {
			return fmt.Errorf("rollback rejected: %s", state.State)
		}
		if err = validateAppliedOwnership(tx, c.MigrationID); err != nil {
			return err
		}
		now := time.Now().UTC()
		// Joining the immutable manifest keeps the write set restricted to original
		// candidates; deferred and pre-existing ownership are never modified.
		var candidates int64
		for _, item := range items {
			if item.Disposition == "candidate" {
				if item.AppliedVersion == ^uint32(0) {
					return fmt.Errorf("ownership version exhausted")
				}
				candidates++
			}
		}
		updated := tx.Exec(`UPDATE testee t JOIN testee_store_migration_items i ON i.testee_id=t.id AND i.org_id=t.org_id
   SET t.store_id=NULL,t.store_version=i.applied_version+1,t.updated_at=?,t.updated_by=?
   WHERE i.migration_id=? AND i.disposition='candidate' AND t.deleted_at IS NULL
   AND t.store_id=i.target_store_id AND t.store_version=i.applied_version`, now, c.ActorID, c.MigrationID)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != candidates {
			return fmt.Errorf("ownership changed during compensation")
		}
		inserted := tx.Exec(`INSERT INTO testee_store_migration_reversions
   (migration_id,testee_id,org_id,from_store_id,original_history_id,restored_version,actor_id,created_at)
   SELECT migration_id,testee_id,org_id,target_store_id,history_id,applied_version+1,?,?
   FROM testee_store_migration_items WHERE migration_id=? AND disposition='candidate'`, c.ActorID, now, c.MigrationID)
		if inserted.Error != nil {
			return inserted.Error
		}
		if inserted.RowsAffected != candidates {
			return fmt.Errorf("incomplete compensation audit")
		}
		after, err := preflightInTx(tx, c.MaxRows)
		if err != nil {
			return err
		}
		manifest.State, manifest.RollbackHash, manifest.RolledBackAt = "rolled_back", after.Fingerprint, &now
		update := tx.Model(&Manifest{}).Where("migration_id=? AND state='applied'", c.MigrationID).Updates(map[string]any{"state": manifest.State, "rollback_hash": manifest.RollbackHash, "rolled_back_at": now})
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			return fmt.Errorf("manifest changed during compensation")
		}
		if err = validateReversions(tx, *manifest, items); err != nil {
			return err
		}
		if err = validateRestoredOwnership(tx, c.MigrationID); err != nil {
			return err
		}
		result = manifest
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func validateRestoredOwnership(tx *gorm.DB, id string) error {
	var mismatches int64
	err := tx.Raw(`SELECT COUNT(*) FROM testee_store_migration_reversions r
 LEFT JOIN testee t ON t.id=r.testee_id AND t.org_id=r.org_id AND t.deleted_at IS NULL
 WHERE r.migration_id=? AND (t.id IS NULL OR t.store_id IS NOT NULL OR t.store_version<>r.restored_version)`, id).Scan(&mismatches).Error
	if err != nil {
		return err
	}
	if mismatches != 0 {
		return fmt.Errorf("rollback has inconsistent current ownership")
	}
	return nil
}
