package testeestore

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

type Manifest struct {
	MigrationID  string
	State        string
	BeforeHash   string
	RollbackHash string
	AfterHash    string
	ItemsHash    string
	ItemCount    uint64
	ActorID      int64
	CreatedAt    time.Time
	CompletedAt  *time.Time
	RolledBackAt *time.Time
}

func (Manifest) TableName() string { return "testee_store_migration_manifests" }

type MigrationItem struct {
	MigrationID    string
	TesteeID       uint64
	OrgID          int64
	BeforeStoreID  *uint64
	TargetStoreID  *uint64
	BeforeVersion  uint32
	AppliedVersion uint32
	HistoryID      *uint64
	Disposition    string
	Reason         string
}

func (MigrationItem) TableName() string { return "testee_store_migration_items" }

type Status struct {
	State              string `json:"state"`
	NextAction         string `json:"next_action"`
	MigrationID        string `json:"migration_id"`
	Fingerprint        string `json:"fingerprint"`
	AfterHash          string `json:"after_hash"`
	RollbackHash       string `json:"rollback_hash"`
	CurrentFingerprint string `json:"current_fingerprint"`
}

// ItemsHash streams canonical items into SHA256; no large serialized manifest buffer.
func ItemsHash(items []MigrationItem) (string, error) {
	sorted := append([]MigrationItem(nil), items...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].TesteeID < sorted[j].TesteeID })
	hash := sha256.New()
	encoder := json.NewEncoder(hash)
	var previous uint64
	for _, item := range sorted {
		if item.TesteeID == 0 || item.TesteeID == previous {
			return "", fmt.Errorf("invalid duplicate migration item")
		}
		previous = item.TesteeID
		if err := encoder.Encode(item); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
func validHash(v string) bool {
	bytes, err := hex.DecodeString(v)
	return err == nil && len(bytes) == sha256.Size
}

// Classify validates the historical manifest before interpreting current drift.
// Neither completed history nor drift authorizes reapplying the migration.
func Classify(id string, manifest *Manifest, items []MigrationItem, current string) (Status, error) {
	result := Status{MigrationID: id, CurrentFingerprint: current, State: "invalid", NextAction: "stop"}
	if id == "" || !validHash(current) {
		return result, fmt.Errorf("migration ID and current fingerprint required")
	}
	if manifest == nil {
		if len(items) != 0 {
			return result, fmt.Errorf("orphan migration items")
		}
		result.State = "pending"
		result.NextAction = "preflight"
		return result, nil
	}
	result.Fingerprint, result.AfterHash = manifest.BeforeHash, manifest.AfterHash
	result.RollbackHash = manifest.RollbackHash
	if manifest.MigrationID != id || manifest.ActorID <= 0 || manifest.CreatedAt.IsZero() || !validHash(manifest.BeforeHash) || manifest.ItemCount != uint64(len(items)) {
		return result, fmt.Errorf("invalid migration manifest")
	}
	for _, item := range items {
		if item.MigrationID != id || item.OrgID <= 0 || item.BeforeVersion == 0 {
			return result, fmt.Errorf("invalid migration item identity")
		}
		switch item.Disposition {
		case "candidate":
			if item.BeforeStoreID != nil || item.TargetStoreID == nil || *item.TargetStoreID == 0 || item.AppliedVersion == 0 || item.AppliedVersion != item.BeforeVersion+1 || item.HistoryID == nil || *item.HistoryID == 0 {
				return result, fmt.Errorf("invalid applied candidate")
			}
		case "deferred", "already_assigned":
			if item.TargetStoreID != nil || item.HistoryID != nil || item.AppliedVersion != item.BeforeVersion {
				return result, fmt.Errorf("non-migrated item was changed")
			}
		default:
			return result, fmt.Errorf("unsupported migration disposition")
		}
	}
	digest, err := ItemsHash(items)
	if err != nil || digest != manifest.ItemsHash {
		return result, fmt.Errorf("migration items checksum mismatch")
	}
	switch manifest.State {
	case "applied":
		if manifest.CompletedAt == nil || manifest.RolledBackAt != nil || manifest.RollbackHash != "" || !validHash(manifest.AfterHash) {
			return result, fmt.Errorf("incomplete applied manifest")
		}
		if current == manifest.AfterHash {
			result.State = "applied_unchanged"
			result.NextAction = "verify"
		} else {
			result.State = "applied_drifted"
			result.NextAction = "review_drift"
		}
	case "rolled_back":
		if manifest.CompletedAt == nil || manifest.RolledBackAt == nil || !validHash(manifest.RollbackHash) || !validHash(manifest.AfterHash) {
			return result, fmt.Errorf("incomplete rollback manifest")
		}
		result.State = "rolled_back"
		result.NextAction = "review_new_migration"
		if current != manifest.RollbackHash {
			result.NextAction = "review_drift"
		}
	default:
		return result, fmt.Errorf("unsupported manifest state")
	}
	return result, nil
}
