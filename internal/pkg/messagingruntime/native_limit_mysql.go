package messagingruntime

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	txmysql "github.com/FangcunMount/reliable-messaging/storage/transaction/mysql"
	"github.com/FangcunMount/reliable-messaging/transaction"
)

// SQLNativeLimits borrows the original primary, installs no schema and starts no work.
type SQLNativeLimits struct {
	db        *sql.DB
	authority *txmysql.Store
	binding   transaction.Binding
	revision  string
}

func NewSQLNativeLimits(db *sql.DB, b transaction.Binding, revision string) (*SQLNativeLimits, error) {
	if db == nil || !nativeNoticeBinding(b, revision) || b.Owner == "qs.mongo" {
		return nil, transaction.ErrBinding
	}
	authority, err := txmysql.New(db, b)
	if err != nil {
		return nil, err
	}
	return &SQLNativeLimits{db, authority, b, revision}, nil
}
func (s *SQLNativeLimits) Binding() transaction.Binding {
	if s == nil {
		return transaction.Binding{}
	}
	return s.binding
}

type sqlNativeNotice struct {
	key, owner, store, epoch, revision, index, proof, state []byte
	schema                                                  int
	resolvedAt                                              *time.Time
	receipt                                                 []byte
}

func (s *SQLNativeLimits) validate(row sqlNativeNotice) error {
	if row.schema != 1 {
		return transaction.ErrSchema
	}
	if !bytes.Equal(row.owner, []byte(s.binding.Owner)) || !bytes.Equal(row.store, []byte(s.binding.StoreID)) || !bytes.Equal(row.epoch, []byte(s.binding.WriterEpoch)) || !bytes.Equal(row.revision, []byte(s.revision)) || len(row.key) != 32 || len(row.index) != 32 || len(row.proof) != 32 || !nativeNoticeResolutionValid(string(row.state), row.resolvedAt, row.receipt) {
		return transaction.ErrBinding
	}
	var index, proof [32]byte
	copy(index[:], row.index)
	copy(proof[:], row.proof)
	key := nativeNoticeKey(s.binding, s.revision, index)
	if index == [32]byte{} || proof == [32]byte{} || !bytes.Equal(row.key, key[:]) {
		return transaction.ErrBinding
	}
	return nil
}

const nativeNoticeColumns = "notice_key,owner,store_id,writer_epoch,route_revision,native_index_sha256,evidence_sha256,status,schema_version,resolved_at,resolution_receipt_sha256"

// RecordNativeLimit writes deny-only governance, never bindings/witnesses or
// Broker confirmations. The caller establishes authority/provenance separately.
// A batch is bounded by64 indices and one caller budget. First audit provenance
// stays immutable when logs/order/formatting change in a later export.
// Re-importing an independently resolved row neither reopens nor rewrites it.
func (s *SQLNativeLimits) RecordNativeLimit(ctx context.Context, e NativeLimitEvidence) error {
	if s == nil || s.db == nil || s.authority == nil {
		return transaction.ErrBinding
	}
	if !e.valid() {
		return ErrNativeLimitEvidence
	}
	ctx, cancel := nativeNoticeBudget(ctx)
	defer cancel()
	if err := s.authority.Validate(ctx); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var store, epoch []byte
	var schema int
	if err = tx.QueryRowContext(ctx, `SELECT store_id,writer_epoch,schema_version FROM rm_tx_binding WHERE owner=? FOR SHARE`, []byte(s.binding.Owner)).Scan(&store, &epoch, &schema); err != nil {
		return err
	}
	if schema != 1 {
		return transaction.ErrSchema
	}
	if !bytes.Equal(store, []byte(s.binding.StoreID)) || !bytes.Equal(epoch, []byte(s.binding.WriterEpoch)) {
		return transaction.ErrBinding
	}
	values := make([]string, 0, len(e.indices))
	args := make([]any, 0, 7*len(e.indices))
	keys := make([]any, 0, len(e.indices))
	expected := map[[32]byte]bool{}
	for _, index := range e.indices {
		key := nativeNoticeKey(s.binding, s.revision, index)
		expected[key] = true
		keys = append(keys, key[:])
		values = append(values, "(?,?,?,?,?,?,?,'blocked',1,UTC_TIMESTAMP(6))")
		args = append(args, key[:], []byte(s.binding.Owner), []byte(s.binding.StoreID), []byte(s.binding.WriterEpoch), []byte(s.revision), index[:], e.digest[:])
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO messaging_native_limit_notices (notice_key,owner,store_id,writer_epoch,route_revision,native_index_sha256,evidence_sha256,status,schema_version,recorded_at) VALUES `+strings.Join(values, ",")+` ON DUPLICATE KEY UPDATE notice_key=notice_key`, args...)
	if err != nil {
		return err
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(keys)), ",")
	rows, err := tx.QueryContext(ctx, `SELECT `+nativeNoticeColumns+` FROM messaging_native_limit_notices WHERE notice_key IN (`+placeholders+`) FOR UPDATE`, keys...)
	if err != nil {
		return err
	}
	verify := func() error {
		for rows.Next() {
			var row sqlNativeNotice
			if err := rows.Scan(&row.key, &row.owner, &row.store, &row.epoch, &row.revision, &row.index, &row.proof, &row.state, &row.schema, &row.resolvedAt, &row.receipt); err != nil {
				return err
			}
			if err := s.validate(row); err != nil {
				return err
			}
			var key [32]byte
			copy(key[:], row.key)
			if !expected[key] {
				return transaction.ErrBinding
			}
			delete(expected, key)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(expected) != 0 {
			return transaction.ErrBinding
		}
		return nil
	}
	err = errors.Join(verify(), rows.Close())
	if err != nil {
		return err
	}
	return tx.Commit()
}
func (s *SQLNativeLimits) ReadNativeLimit(ctx context.Context) (bool, error) {
	if s == nil || s.db == nil || s.authority == nil {
		return false, transaction.ErrBinding
	}
	ctx, cancel := nativeNoticeBudget(ctx)
	defer cancel()
	if err := s.authority.Validate(ctx); err != nil {
		return false, err
	}
	var row sqlNativeNotice
	err := s.db.QueryRowContext(ctx, `SELECT `+nativeNoticeColumns+` FROM messaging_native_limit_notices WHERE owner=? AND status<>'resolved' LIMIT 1`, []byte(s.binding.Owner)).Scan(&row.key, &row.owner, &row.store, &row.epoch, &row.revision, &row.index, &row.proof, &row.state, &row.schema, &row.resolvedAt, &row.receipt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err = s.validate(row); err != nil {
		return false, err
	}
	return true, nil
}
