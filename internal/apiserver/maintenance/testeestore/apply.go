package testeestore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/store"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/actor/testee"
	historyrepo "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/actor/testeestore"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ApplyCommand struct {
	MigrationID    string
	Fingerprint    string
	OrgID, ActorID int64
	MaxRows        int
	WritesPaused   bool
}

func authorizeApply(ctx context.Context, c ApplyCommand) error {
	snapshot, ok := authz.FromContext(ctx)
	if !ok || snapshot == nil || !snapshot.IsQSAdmin() || c.OrgID <= 0 || c.ActorID <= 0 {
		return fmt.Errorf("trusted company administrator required")
	}
	if !c.WritesPaused {
		return fmt.Errorf("ownership and relationship writes must be paused")
	}
	if len(c.MigrationID) == 0 || len(c.MigrationID) > 64 || !validHash(c.Fingerprint) || c.MaxRows < 1 || c.MaxRows > 1000000 {
		return fmt.Errorf("invalid migration parameters")
	}
	for _, r := range c.MigrationID {
		if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_", r) {
			return fmt.Errorf("invalid migration identifier")
		}
	}
	return nil
}

// lockFacts serializes maintenance against live ownership, clinician, and relation changes.
// Maintenance is bounded and follows the same store -> clinician -> testee -> relation order.
func lockFacts(tx *gorm.DB, maxRows int) error {
	for _, table := range []string{"actor_stores", "clinician", "testee", "clinician_relation"} {
		ids := []uint64{}
		if err := tx.Table(table).Select("id").Order("id").Limit(maxRows + 1).Clauses(clause.Locking{Strength: "UPDATE"}).Find(&ids).Error; err != nil {
			return err
		}
		if len(ids) > maxRows {
			return fmt.Errorf("%s lock limit exceeded", table)
		}
	}
	return nil
}
func loadManifest(tx *gorm.DB, id string, maxRows int) (*Manifest, []MigrationItem, error) {
	var manifest Manifest
	err := tx.Where("migration_id=?", id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&manifest).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, err
	}
	items := []MigrationItem{}
	if queryErr := tx.Where("migration_id=?", id).Order("testee_id").Limit(maxRows + 1).Find(&items).Error; queryErr != nil {
		return nil, nil, queryErr
	}
	if len(items) > maxRows {
		return nil, nil, fmt.Errorf("migration item limit exceeded")
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if len(items) > 0 {
			return nil, nil, fmt.Errorf("orphan migration items")
		}
		return nil, items, nil
	}
	return &manifest, items, nil
}

// Apply commits ownership, audit and manifest together. Existing completed migrations are read-only.
func Apply(ctx context.Context, db *gorm.DB, c ApplyCommand) (*Manifest, error) {
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
		existing, items, err := loadManifest(tx, c.MigrationID, c.MaxRows)
		if err != nil {
			return err
		}
		if existing != nil {
			state, err := Classify(c.MigrationID, existing, items, report.Fingerprint)
			if err != nil {
				return err
			}
			if state.State != "applied_unchanged" || c.Fingerprint != existing.BeforeHash {
				return fmt.Errorf("migration cannot be reapplied: %s", state.State)
			}
			if err = validateReversions(tx, *existing, items); err != nil {
				return err
			}
			if err = validateHistory(tx, *existing, items); err != nil {
				return err
			}
			if err = validateAppliedOwnership(tx, c.MigrationID); err != nil {
				return err
			}
			result = existing
			return nil
		}
		if !report.Executable || report.Fingerprint != c.Fingerprint {
			return fmt.Errorf("preflight changed or has unresolved facts")
		}
		now := time.Now().UTC()
		items = make([]MigrationItem, 0, len(report.Candidates))
		histories := make([]historyrepo.HistoryPO, 0, report.Counts["candidate"])
		targets := map[uint64]*store.Store{}
		for _, candidate := range report.Candidates {
			if candidate.OrgID != c.OrgID {
				return fmt.Errorf("preflight contains another company; explicit company migration required")
			}
			item := MigrationItem{MigrationID: c.MigrationID, TesteeID: candidate.TesteeID, OrgID: candidate.OrgID, BeforeStoreID: copyID(candidate.CurrentStoreID), BeforeVersion: candidate.ExpectedVersion, AppliedVersion: candidate.ExpectedVersion, Disposition: candidate.State}
			if candidate.State == "candidate" {
				if candidate.TargetStoreID == nil {
					return fmt.Errorf("candidate target missing")
				}
				target := targets[*candidate.TargetStoreID]
				if target == nil {
					var state store.State
					if err = tx.Table("actor_stores").Where("id=? AND org_id=?", *candidate.TargetStoreID, c.OrgID).Take(&state).Error; err != nil {
						return err
					}
					target = store.Restore(state)
					targets[target.ID()] = target
				}
				subject := testee.NewTestee(c.OrgID, "", testee.Gender(0), nil)
				subject.RestoreStore(candidate.CurrentStoreID, candidate.ExpectedVersion)
				changed, err := subject.AssignInitialStore(target, candidate.ExpectedVersion)
				if err != nil {
					return err
				}
				if !changed || subject.StoreVersion() == 0 {
					return fmt.Errorf("invalid candidate transition")
				}
				item.TargetStoreID = subject.StoreID()
				item.AppliedVersion = subject.StoreVersion()
				historyID := meta.New().Uint64()
				item.HistoryID = &historyID
				item.Reason = "unique_management_store"
				histories = append(histories, historyrepo.HistoryPO{ID: historyID, OrgID: c.OrgID, TesteeID: candidate.TesteeID, ToStoreID: *item.TargetStoreID, Kind: "migration_initial", ActorID: c.ActorID, CreatedAt: now, Reason: "历史有效管理关系唯一门店", RequestID: MigrationRequestID(c.MigrationID), Version: item.AppliedVersion})
			} else if candidate.State == "deferred" {
				item.Reason = "no_management_relation"
			} else if candidate.State != "already_assigned" {
				return fmt.Errorf("unsupported candidate state")
			}
			items = append(items, item)
		}
		if err = writeOwnershipBatches(tx, items, c.ActorID, now); err != nil {
			return err
		}
		if len(histories) > 0 {
			if err = tx.CreateInBatches(&histories, 500).Error; err != nil {
				return err
			}
		}
		if len(items) > 0 {
			if err = tx.CreateInBatches(&items, 500).Error; err != nil {
				return err
			}
		}
		after, err := preflightInTx(tx, c.MaxRows)
		if err != nil {
			return err
		}
		digest, err := ItemsHash(items)
		if err != nil {
			return err
		}
		result = &Manifest{MigrationID: c.MigrationID, State: "applied", BeforeHash: report.Fingerprint, AfterHash: after.Fingerprint, ItemsHash: digest, ItemCount: uint64(len(items)), ActorID: c.ActorID, CreatedAt: now, CompletedAt: &now}
		if err = tx.Create(result).Error; err != nil {
			return err
		}
		if err = validateHistory(tx, *result, items); err != nil {
			return err
		}
		return validateAppliedOwnership(tx, c.MigrationID)
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	return result, nil
}

type ownershipBatchKey struct {
	org     int64
	store   uint64
	version uint32
}

func writeOwnershipBatches(tx *gorm.DB, items []MigrationItem, actor int64, now time.Time) error {
	groups := map[ownershipBatchKey][]uint64{}
	for _, item := range items {
		if item.Disposition == "candidate" {
			key := ownershipBatchKey{item.OrgID, *item.TargetStoreID, item.BeforeVersion}
			groups[key] = append(groups[key], item.TesteeID)
		}
	}
	keys := make([]ownershipBatchKey, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a.org != b.org {
			return a.org < b.org
		}
		if a.store != b.store {
			return a.store < b.store
		}
		return a.version < b.version
	})
	for _, key := range keys {
		ids := groups[key]
		for start := 0; start < len(ids); start += 500 {
			end := start + 500
			if end > len(ids) {
				end = len(ids)
			}
			result := tx.Table("testee").Where("org_id=? AND id IN ? AND store_id IS NULL AND store_version=? AND deleted_at IS NULL", key.org, ids[start:end], key.version).Updates(map[string]any{"store_id": key.store, "store_version": key.version + 1, "updated_at": now, "updated_by": actor})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != int64(end-start) {
				return fmt.Errorf("concurrent ownership change during batch")
			}
		}
	}
	return nil
}
