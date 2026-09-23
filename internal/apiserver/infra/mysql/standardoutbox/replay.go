//go:build reliable_messaging_m4

// Package standardoutbox owns QS-specific governance for the SDK MySQL table.
// It does not change the shared SDK or construct database schema at runtime.
package standardoutbox

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"

	request "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
)

var (
	ErrReplayInputConflict = request.ErrReplayInputConflict
	ErrReplayLedgerCorrupt = request.ErrReplayLedgerCorrupt
)

// ReplayLedger is scoped to the host business database and one named Outbox
// profile. The request header, every result, and every authorization share a
// single MySQL transaction. A lost HTTP response can read the same results.
type ReplayLedger struct {
	db        *sql.DB
	storeName string
}

func NewReplayLedger(db *sql.DB, storeName string) (*ReplayLedger, error) {
	if db == nil || storeName == "" {
		return nil, errors.New("database and store name required")
	}
	return &ReplayLedger{db: db, storeName: storeName}, nil
}

func (l *ReplayLedger) Authorize(ctx context.Context, input request.ReplayRequest) ([]request.ReplayResult, error) {
	fingerprint, err := input.Fingerprint()
	if err != nil {
		return nil, err
	}
	if input.Store != l.storeName {
		return nil, errors.New("replay request targets another Outbox profile")
	}
	tx, err := l.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	insert, err := tx.ExecContext(ctx, `INSERT IGNORE INTO qs_rm_replay_requests
 (org_id,request_id,store_name,reason,input_hash) VALUES (?,?,?,?,?)`,
		input.OrgID, input.RequestID, input.Store, input.Reason, fingerprint[:])
	if err != nil {
		return nil, err
	}
	inserted, err := insert.RowsAffected()
	if err != nil {
		return nil, err
	}
	var storedHash []byte
	if err := tx.QueryRowContext(ctx, `SELECT input_hash FROM qs_rm_replay_requests
 WHERE org_id=? AND request_id=? FOR UPDATE`, input.OrgID, input.RequestID).Scan(&storedHash); err != nil {
		return nil, err
	}
	if !bytes.Equal(storedHash, fingerprint[:]) {
		return nil, ErrReplayInputConflict
	}
	if inserted == 0 {
		results, err := loadReplayResults(ctx, tx, input)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return results, nil
	}
	if inserted != 1 {
		return nil, fmt.Errorf("unexpected replay header insert count %d", inserted)
	}
	results := make([]request.ReplayResult, 0, len(input.Targets))
	for index, target := range input.Targets {
		result, err := authorizeOne(ctx, tx, input, target)
		if err != nil {
			return nil, err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO qs_rm_replay_items
 (org_id,request_id,ordinal,event_id,expected_failure_count,authorized,reason)
 VALUES (?,?,?,?,?,?,?)`, input.OrgID, input.RequestID, index, target.EventID,
			target.ExpectedFailureCount, result.Authorized, result.Reason)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return results, nil
}

// Resolve only reads a committed request and its ordered results. A missing
// record remains unknown to the caller; it must not be treated as permission
// to create a second request ID or replay the message blindly.
func (l *ReplayLedger) Resolve(ctx context.Context, input request.ReplayRequest) ([]request.ReplayResult, bool, error) {
	fingerprint, err := input.Fingerprint()
	if err != nil {
		return nil, false, err
	}
	if input.Store != l.storeName {
		return nil, false, errors.New("replay request targets another Outbox profile")
	}
	tx, err := l.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback()
	var storedHash []byte
	err = tx.QueryRowContext(ctx, `SELECT input_hash FROM qs_rm_replay_requests
 WHERE org_id=? AND request_id=?`, input.OrgID, input.RequestID).Scan(&storedHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !bytes.Equal(storedHash, fingerprint[:]) {
		return nil, true, ErrReplayInputConflict
	}
	results, err := loadReplayResults(ctx, tx, input)
	if err != nil {
		return nil, true, err
	}
	if err := tx.Commit(); err != nil {
		return nil, true, err
	}
	return results, true, nil
}

func loadReplayResults(ctx context.Context, tx *sql.Tx, input request.ReplayRequest) ([]request.ReplayResult, error) {
	rows, err := tx.QueryContext(ctx, `SELECT ordinal,event_id,expected_failure_count,authorized,reason
 FROM qs_rm_replay_items WHERE org_id=? AND request_id=? ORDER BY ordinal`, input.OrgID, input.RequestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	results := make([]request.ReplayResult, 0, len(input.Targets))
	for rows.Next() {
		var ordinal int
		var expected uint64
		var result request.ReplayResult
		if err := rows.Scan(&ordinal, &result.EventID, &expected, &result.Authorized, &result.Reason); err != nil {
			return nil, err
		}
		if ordinal != len(results) || ordinal >= len(input.Targets) ||
			result.EventID != input.Targets[ordinal].EventID || expected != input.Targets[ordinal].ExpectedFailureCount {
			return nil, ErrReplayLedgerCorrupt
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(results) != len(input.Targets) {
		return nil, ErrReplayLedgerCorrupt
	}
	return results, nil
}

func authorizeOne(ctx context.Context, tx *sql.Tx, input request.ReplayRequest, target request.ReplayTarget) (request.ReplayResult, error) {
	result := request.ReplayResult{EventID: target.EventID}
	rows, err := tx.QueryContext(ctx, `SELECT id,scope,state,last_error_code,failure_count,version
 FROM rm_outbox WHERE message_id=? LIMIT 2 FOR UPDATE`, target.EventID)
	if err != nil {
		return result, err
	}
	type record struct {
		id, failures, version uint64
		scope, state, code    string
	}
	var found []record
	for rows.Next() {
		var row record
		if err := rows.Scan(&row.id, &row.scope, &row.state, &row.code, &row.failures, &row.version); err != nil {
			rows.Close()
			return result, err
		}
		found = append(found, row)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	if len(found) == 0 {
		result.Reason = "not_found"
		return result, nil
	}
	if len(found) > 1 {
		result.Reason = "ambiguous_identity"
		return result, nil
	}
	row := found[0]
	switch {
	case row.scope != fmt.Sprintf("org:%d", input.OrgID):
		result.Reason = "organization_mismatch"
	case row.state != "quarantined" || row.code != "publish_unknown":
		result.Reason = "not_manual_required"
	case row.failures != target.ExpectedFailureCount:
		result.Reason = "attempt_conflict"
	default:
		update, err := tx.ExecContext(ctx, `UPDATE rm_outbox
 SET state='retry_wait',next_attempt_at=UTC_TIMESTAMP(6),claim_token=NULL,lease_until=NULL,
 manual_replay_request_id=?,manual_replay_version=?,version=version+1,updated_at=UTC_TIMESTAMP(6)
 WHERE id=? AND scope=? AND state='quarantined' AND last_error_code='publish_unknown'
 AND failure_count=? AND version=?`, input.RequestID, row.version+1, row.id, row.scope, row.failures, row.version)
		if err != nil {
			return result, err
		}
		count, err := update.RowsAffected()
		if err != nil {
			return result, err
		}
		if count != 1 {
			return result, errors.New("conditional replay update lost locked row")
		}
		result.Authorized = true
	}
	return result, nil
}
