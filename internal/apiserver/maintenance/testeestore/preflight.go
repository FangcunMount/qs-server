package testeestore

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

type Report struct {
	FormatVersion int            `json:"format_version"`
	GeneratedAt   time.Time      `json:"generated_at"`
	Fingerprint   string         `json:"fingerprint"`
	SchemaReady   bool           `json:"schema_ready"`
	Executable    bool           `json:"executable"`
	Counts        map[string]int `json:"counts"`
	Candidates    []Candidate    `json:"candidates"`
}

// Preflight reads one bounded, consistent snapshot. It never migrates data or schema.
// The row ceiling is explicit: an incomplete snapshot must not appear executable.
func Preflight(ctx context.Context, db *gorm.DB, maxRows int) (*Report, error) {
	var report *Report
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		report, err = preflightInTx(tx, maxRows)
		return err
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return report, err
}

func preflightInTx(tx *gorm.DB, maxRows int) (*Report, error) {
	if maxRows < 1 || maxRows > 1000000 {
		return nil, fmt.Errorf("max rows must be between 1 and 1000000")
	}
	var facts Facts
	ready := false
	err := func(tx *gorm.DB) error {
		columns := struct{ Count int }{}
		if err := tx.Raw("SELECT COUNT(*) AS count FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='testee' AND column_name IN ('store_id','store_version')").Scan(&columns).Error; err != nil {
			return err
		}
		if columns.Count != 0 && columns.Count != 2 {
			return fmt.Errorf("partial testee ownership schema")
		}
		ready = columns.Count == 2
		ownership := "NULL AS store_id, 1 AS version"
		if ready {
			ownership = "store_id, store_version AS version"
		}
		if err := tx.Table("testee").Select("id,org_id," + ownership).Where("deleted_at IS NULL").Order("id").Limit(maxRows + 1).Scan(&facts.Testees).Error; err != nil {
			return err
		}
		if len(facts.Testees) > maxRows {
			return fmt.Errorf("testee fact limit exceeded")
		}
		if err := tx.Table("clinician").Select("id,org_id,store_id,is_active AS active,(deleted_at IS NOT NULL) AS deleted").Order("id").Limit(maxRows + 1).Scan(&facts.Clinicians).Error; err != nil {
			return err
		}
		if len(facts.Clinicians) > maxRows {
			return fmt.Errorf("clinician fact limit exceeded")
		}
		if err := tx.Table("actor_stores").Select("id,org_id,is_active AS active").Order("id").Limit(maxRows + 1).Scan(&facts.Stores).Error; err != nil {
			return err
		}
		if len(facts.Stores) > maxRows {
			return fmt.Errorf("store fact limit exceeded")
		}
		if err := tx.Table("clinician_relation").Select("id,org_id,testee_id,clinician_id,relation_type AS type,is_active AS active,(deleted_at IS NOT NULL) AS deleted,unbound_at").Where("is_active=TRUE AND deleted_at IS NULL").Order("id").Limit(maxRows + 1).Scan(&facts.Relations).Error; err != nil {
			return err
		}
		if len(facts.Relations) > maxRows {
			return fmt.Errorf("relation fact limit exceeded")
		}
		return nil
	}(tx)
	if err != nil {
		return nil, err
	}
	candidates, err := Plan(facts)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(facts)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(encoded)
	report := &Report{FormatVersion: 1, GeneratedAt: time.Now().UTC(), Fingerprint: hex.EncodeToString(sum[:]), SchemaReady: ready, Executable: ready, Counts: map[string]int{}, Candidates: candidates}
	for _, candidate := range candidates {
		report.Counts[candidate.State]++
		if candidate.State == "unresolved" {
			report.Executable = false
		}
	}
	return report, nil
}
