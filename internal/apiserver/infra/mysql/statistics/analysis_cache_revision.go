package statistics

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	authz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	"gorm.io/gorm"
	"hash"
)

// AnalysisCacheRevision hashes only the current inputs used by a topic. It does
// not scan historical statistics facts. Streaming keeps memory bounded, and
// exact membership detects even count-preserving transfers and manual repairs.
// The published run identifies historical facts separately in the cache key.
func (s *ReadStore) AnalysisCacheRevision(ctx context.Context, org int64, scope authz.StoreRange, kind string) (string, error) {
	if org <= 0 {
		return "", fmt.Errorf("invalid statistics company")
	}
	if kind != "overview" && kind != "clinicians" && kind != "entries" {
		return "", fmt.Errorf("invalid statistics topic")
	}
	ctx, release, err := s.acquire(ctx)
	if err != nil {
		return "", err
	}
	defer release()
	h := sha256.New()
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		owners := func() *gorm.DB {
			return applyCurrentStoreRange(tx.Table("testee").Where("org_id=? AND deleted_at IS NULL", org), "store_id", scope)
		}
		doctors := func() *gorm.DB {
			return applyCurrentStoreRange(tx.Table("clinician c").Where("c.org_id=? AND c.deleted_at IS NULL", org), "c.store_id", scope)
		}
		entries := func() *gorm.DB {
			return applyCurrentStoreRange(tx.Table("assessment_entry en").Joins("JOIN clinician c ON c.id=en.clinician_id AND c.org_id=en.org_id").Where("en.org_id=? AND en.deleted_at IS NULL AND c.deleted_at IS NULL", org), "c.store_id", scope)
		}
		queries := []*gorm.DB{owners().Select("id,store_id").Order("id")}
		if kind == "overview" {
			queries = append(queries, doctors().Select("c.id,c.store_id,c.is_active").Order("c.id"), entries().Select("en.id,en.clinician_id,en.is_active").Order("en.id"), tx.Table("plan_enrollment").Where("org_id=? AND status='active' AND deleted_at IS NULL", org).Where("testee_id IN (?)", owners().Select("id")).Select("testee_id,COUNT(*)").Group("testee_id").Order("testee_id"))
		} else {
			queries = append(queries, doctors().Select("c.id,c.store_id,c.name,c.department,c.title,c.clinician_type,c.is_active").Order("c.id"), entries().Select("en.id,en.clinician_id,en.is_active,en.target_type,en.target_code,en.target_version,en.expires_at,en.created_at").Order("en.id"))
			if kind == "clinicians" {
				queries = append(queries, tx.Table("clinician_relation").Where("org_id=? AND is_active=1 AND deleted_at IS NULL", org).Where("clinician_id IN (?)", doctors().Select("c.id")).Where("testee_id IN (?)", owners().Select("id")).Select("clinician_id,testee_id,relation_type").Order("clinician_id,testee_id,relation_type"))
			}
		}
		for i, query := range queries {
			_, _ = fmt.Fprintf(h, "query:%d;", i)
			if err := hashAnalysisRows(h, query); err != nil {
				return err
			}
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashAnalysisRows(h hash.Hash, query *gorm.DB) error {
	rows, err := query.Rows()
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	columns, err := rows.Columns()
	if err != nil {
		return err
	}
	values := make([]sql.NullString, len(columns))
	args := make([]any, len(values))
	for i := range values {
		args[i] = &values[i]
	}
	var length [8]byte
	for rows.Next() {
		if err = rows.Scan(args...); err != nil {
			return err
		}
		for _, value := range values {
			if !value.Valid {
				_, _ = h.Write([]byte{0})
				continue
			}
			_, _ = h.Write([]byte{1})
			binary.BigEndian.PutUint64(length[:], uint64(len(value.String)))
			_, _ = h.Write(length[:])
			_, _ = h.Write([]byte(value.String))
		}
	}
	return rows.Err()
}
