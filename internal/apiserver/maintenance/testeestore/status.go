package testeestore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// ReadStatus observes current facts and historical evidence in the same read-only snapshot.
func ReadStatus(ctx context.Context, db *gorm.DB, id string, maxRows int) (Status, error) {
	result := Status{MigrationID: id, State: "invalid", NextAction: "stop"}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		report, err := preflightInTx(tx, maxRows)
		if err != nil {
			return err
		}
		var count int64
		if err = tx.Raw("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name IN ('testee_store_migration_manifests','testee_store_migration_items','testee_store_migration_reversions')").Scan(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			result, err = Classify(id, nil, nil, report.Fingerprint)
			return err
		}
		if count != 3 {
			return fmt.Errorf("partial migration manifest schema")
		}
		var manifest Manifest
		err = tx.Where("migration_id=?", id).Take(&manifest).Error
		absent := errors.Is(err, gorm.ErrRecordNotFound)
		if err != nil && !absent {
			return err
		}
		items := []MigrationItem{}
		if err = tx.Where("migration_id=?", id).Order("testee_id").Limit(maxRows + 1).Find(&items).Error; err != nil {
			return err
		}
		if len(items) > maxRows {
			return fmt.Errorf("migration item limit exceeded")
		}
		if absent {
			result, err = Classify(id, nil, items, report.Fingerprint)
		} else {
			result, err = Classify(id, &manifest, items, report.Fingerprint)
			if err == nil {
				if result.State == "applied_unchanged" {
					if ownershipErr := validateAppliedOwnership(tx, id); ownershipErr != nil {
						result.State = "invalid"
						result.NextAction = "stop"
						return ownershipErr
					}
				}
				if result.State == "rolled_back" && result.CurrentFingerprint == manifest.RollbackHash {
					if restoreErr := validateRestoredOwnership(tx, id); restoreErr != nil {
						result.State, result.NextAction = "invalid", "stop"
						return restoreErr
					}
				}
				if reversionErr := validateReversions(tx, manifest, items); reversionErr != nil {
					result.State, result.NextAction = "invalid", "stop"
					return reversionErr
				}
				if historyErr := validateHistory(tx, manifest, items); historyErr != nil {
					result.State = "invalid"
					result.NextAction = "stop"
					return historyErr
				}
			}
		}
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return result, err
}

// Verify is strict: readable history with drift is not successful acceptance.
func Verify(ctx context.Context, db *gorm.DB, id string, maxRows int) (Status, error) {
	state, err := ReadStatus(ctx, db, id, maxRows)
	if err != nil {
		return state, err
	}
	if state.State != "applied_unchanged" {
		return state, fmt.Errorf("ownership migration verification requires applied_unchanged, got %s", state.State)
	}
	return state, nil
}
