package standardoutbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	mysqlstore "github.com/FangcunMount/reliable-messaging/storage/mysql"
)

var (
	ErrGapRecoveryInputConflict = errors.New("gap recovery request ID has different input")
	ErrGapRecoveryLedgerCorrupt = errors.New("gap recovery request has incomplete result")
)

// GapRecoveryRequest targets one original, already confirmed evaluation event.
// ActorID and Reason are host governance facts, not SDK transport policy.
type GapRecoveryRequest struct {
	OrgID           int64
	RequestID       string
	ActorID         uint64
	AssessmentID    uint64
	EventID         string
	ExpectedVersion uint64
	Reason          string
	SubmittedBefore time.Time
}

type GapRecoveryResult struct {
	Authorized          bool
	Code                string
	OutboxVersionBefore uint64
	OutboxVersionAfter  uint64
}

type GapRecoverySummary struct {
	Authorized   int64
	Denied       int64
	WaitingRelay int64
}

type GapRecoveryLedger struct{ db *sql.DB }

func NewGapRecoveryLedger(db *sql.DB) (*GapRecoveryLedger, error) {
	if db == nil {
		return nil, errors.New("gap recovery requires host database")
	}
	return &GapRecoveryLedger{db: db}, nil
}

// ReadSummary is tenant-scoped and deliberately separate from the standard
// Outbox governance reader: older standard profiles do not have migration 90.
// The durable decision totals include completed requests; WaitingRelay only
// counts the still-current authorized Outbox version awaiting SDK Relay.
func (l *GapRecoveryLedger) ReadSummary(ctx context.Context, orgID int64) (GapRecoverySummary, error) {
	var result GapRecoverySummary
	if l == nil || l.db == nil || orgID <= 0 {
		return result, errors.New("gap recovery summary requires host database and organization")
	}
	var incomplete int64
	err := l.db.QueryRowContext(ctx, `SELECT
	COALESCE(SUM(result_code<>'' AND authorized=1),0),
	COALESCE(SUM(result_code<>'' AND authorized=0),0),
	COALESCE(SUM(result_code=''),0),
	(SELECT COUNT(*) FROM qs_rm_gap_recovery_request AS pending
	 WHERE pending.org_id=? AND pending.authorized=1 AND EXISTS (
	   SELECT 1 FROM rm_outbox AS o WHERE o.message_id=pending.event_id
	   AND o.scope=CONCAT('org:',pending.org_id)
	   AND o.producer='qs-server' AND o.event_type='evaluation.requested'
	   AND o.manual_replay_request_id=pending.request_id
	   AND o.manual_replay_version=pending.outbox_version_after
	   AND o.version=pending.outbox_version_after AND o.state='retry_wait'))
	FROM qs_rm_gap_recovery_request WHERE org_id=?`, orgID, orgID).
		Scan(&result.Authorized, &result.Denied, &incomplete, &result.WaitingRelay)
	if err != nil {
		return GapRecoverySummary{}, err
	}
	if incomplete != 0 {
		return GapRecoverySummary{}, ErrGapRecoveryLedgerCorrupt
	}
	return result, nil
}

func (r GapRecoveryRequest) fingerprint() ([32]byte, string, error) {
	if r.OrgID <= 0 || r.AssessmentID == 0 || r.AssessmentID > math.MaxInt64 ||
		r.ActorID == 0 || r.ExpectedVersion == 0 || r.RequestID == "" || len(r.RequestID) > 64 ||
		r.EventID == "" || len(r.EventID) > 128 || strings.TrimSpace(r.Reason) == "" || len(r.Reason) > 1024 ||
		r.SubmittedBefore.IsZero() || r.SubmittedBefore.After(time.Now().Add(-10*time.Minute)) {
		return [32]byte{}, "", errors.New("invalid bounded gap recovery request")
	}
	cutoff := r.SubmittedBefore.In(time.FixedZone("UTC+8", 8*3600)).Format(time.RFC3339Nano)
	encoded, err := json.Marshal(struct {
		OrgID           int64  `json:"org_id"`
		RequestID       string `json:"request_id"`
		ActorID         uint64 `json:"actor_id"`
		AssessmentID    uint64 `json:"assessment_id"`
		EventID         string `json:"event_id"`
		ExpectedVersion uint64 `json:"expected_version"`
		Reason          string `json:"reason"`
		SubmittedBefore string `json:"submitted_before"`
	}{r.OrgID, r.RequestID, r.ActorID, r.AssessmentID, r.EventID, r.ExpectedVersion, r.Reason, cutoff})
	if err != nil {
		return [32]byte{}, "", err
	}
	return sha256.Sum256(encoded), cutoff, nil
}

// Authorize atomically records the decision and, only for a never-claimed
// original event, schedules the existing Outbox row for SDK Relay. A repeated
// request ID returns the same committed result. It never publishes directly.
func (l *GapRecoveryLedger) Authorize(ctx context.Context, input GapRecoveryRequest) (GapRecoveryResult, error) {
	if l == nil || l.db == nil {
		return GapRecoveryResult{}, errors.New("gap recovery ledger is not configured")
	}
	hash, cutoff, err := input.fingerprint()
	if err != nil {
		return GapRecoveryResult{}, err
	}
	tx, err := l.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return GapRecoveryResult{}, err
	}
	defer func() { _ = tx.Rollback() }()
	insert, err := tx.ExecContext(ctx, `INSERT IGNORE INTO qs_rm_gap_recovery_request
	(org_id,request_id,actor_id,assessment_id,event_id,expected_version,reason,submitted_before,input_hash)
	VALUES (?,?,?,?,?,?,?,?,?)`, input.OrgID, input.RequestID, input.ActorID, input.AssessmentID, input.EventID,
		input.ExpectedVersion, input.Reason, cutoff, hash[:])
	if err != nil {
		return GapRecoveryResult{}, err
	}
	inserted, err := insert.RowsAffected()
	if err != nil {
		return GapRecoveryResult{}, err
	}
	var storedHash []byte
	var code string
	var authorized bool
	var before, after sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT input_hash,result_code,authorized,outbox_version_before,outbox_version_after
	FROM qs_rm_gap_recovery_request WHERE org_id=? AND request_id=? FOR UPDATE`, input.OrgID, input.RequestID).
		Scan(&storedHash, &code, &authorized, &before, &after)
	if err != nil {
		return GapRecoveryResult{}, err
	}
	if !bytes.Equal(storedHash, hash[:]) {
		return GapRecoveryResult{}, ErrGapRecoveryInputConflict
	}
	if inserted == 0 {
		if code == "" || (authorized && (!before.Valid || !after.Valid)) {
			return GapRecoveryResult{}, ErrGapRecoveryLedgerCorrupt
		}
		result := GapRecoveryResult{Authorized: authorized, Code: code}
		if before.Valid {
			result.OutboxVersionBefore = uint64(before.Int64)
		}
		if after.Valid {
			result.OutboxVersionAfter = uint64(after.Int64)
		}
		if err := tx.Commit(); err != nil {
			return GapRecoveryResult{}, err
		}
		return result, nil
	}
	if inserted != 1 {
		return GapRecoveryResult{}, fmt.Errorf("unexpected gap recovery insert count %d", inserted)
	}
	result, err := authorizeGapOne(ctx, tx, input)
	if err != nil {
		return GapRecoveryResult{}, err
	}
	update, err := tx.ExecContext(ctx, `UPDATE qs_rm_gap_recovery_request
	SET result_code=?,authorized=?,outbox_version_before=?,outbox_version_after=?,updated_at=UTC_TIMESTAMP(6)
	WHERE org_id=? AND request_id=? AND result_code=''`, result.Code, result.Authorized,
		nullVersion(result.OutboxVersionBefore), nullVersion(result.OutboxVersionAfter), input.OrgID, input.RequestID)
	if err != nil {
		return GapRecoveryResult{}, err
	}
	n, err := update.RowsAffected()
	if err != nil {
		return GapRecoveryResult{}, err
	}
	if n != 1 {
		return GapRecoveryResult{}, ErrGapRecoveryLedgerCorrupt
	}
	if err := tx.Commit(); err != nil {
		return GapRecoveryResult{}, err
	}
	return result, nil
}

func nullVersion(version uint64) any {
	if version == 0 {
		return nil
	}
	return version
}

func authorizeGapOne(ctx context.Context, tx *sql.Tx, input GapRecoveryRequest) (GapRecoveryResult, error) {
	denied := func(code string) (GapRecoveryResult, error) { return GapRecoveryResult{Code: code}, nil }
	var assessmentID uint64
	err := tx.QueryRowContext(ctx, `SELECT id FROM assessment WHERE id=? AND org_id=? FOR UPDATE`, input.AssessmentID, input.OrgID).Scan(&assessmentID)
	if errors.Is(err, sql.ErrNoRows) {
		return denied("assessment_missing")
	}
	if err != nil {
		return GapRecoveryResult{}, err
	}
	refs, err := tx.QueryContext(ctx, `SELECT event_id FROM qs_rm_evaluation_request_ref
	FORCE INDEX (ix_qs_rm_evaluation_request_assessment)
	WHERE assessment_id=? FOR UPDATE`, input.AssessmentID)
	if err != nil {
		return GapRecoveryResult{}, err
	}
	var eventIDs []string
	for refs.Next() {
		var id string
		if err := refs.Scan(&id); err != nil {
			_ = refs.Close()
			return GapRecoveryResult{}, err
		}
		if len(eventIDs) < 2 {
			eventIDs = append(eventIDs, id)
		}
	}
	if err := refs.Err(); err != nil {
		_ = refs.Close()
		return GapRecoveryResult{}, err
	}
	if err := refs.Close(); err != nil {
		return GapRecoveryResult{}, err
	}
	if len(eventIDs) != 1 || eventIDs[0] != input.EventID {
		return denied("reference_missing_or_ambiguous")
	}
	runs, err := tx.QueryContext(ctx, `SELECT id FROM runtime_checkpoint FORCE INDEX (idx_runtime_checkpoint_assessment_id)
	WHERE assessment_id=? FOR UPDATE`, input.AssessmentID)
	if err != nil {
		return GapRecoveryResult{}, err
	}
	hasRun := runs.Next()
	if err := runs.Err(); err != nil {
		_ = runs.Close()
		return GapRecoveryResult{}, err
	}
	if err := runs.Close(); err != nil {
		return GapRecoveryResult{}, err
	}
	if hasRun {
		return denied("ever_claimed")
	}
	outboxRows, err := tx.QueryContext(ctx, `SELECT id,version,fingerprint FROM rm_outbox FORCE INDEX (ix_rm_outbox_message_id)
	WHERE message_id=? FOR UPDATE`, input.EventID)
	if err != nil {
		return GapRecoveryResult{}, err
	}
	var found []struct {
		id, version uint64
		fingerprint []byte
	}
	for outboxRows.Next() {
		var row struct {
			id, version uint64
			fingerprint []byte
		}
		if err := outboxRows.Scan(&row.id, &row.version, &row.fingerprint); err != nil {
			_ = outboxRows.Close()
			return GapRecoveryResult{}, err
		}
		if len(found) < 2 {
			found = append(found, row)
		}
	}
	if err := outboxRows.Err(); err != nil {
		_ = outboxRows.Close()
		return GapRecoveryResult{}, err
	}
	if err := outboxRows.Close(); err != nil {
		return GapRecoveryResult{}, err
	}
	if len(found) != 1 {
		return denied("outbox_missing_or_ambiguous")
	}
	if found[0].version != input.ExpectedVersion {
		return denied("version_conflict")
	}
	inspection, err := inspectOriginalRequest(ctx, tx, input.OrgID, input.AssessmentID, input.SubmittedBefore)
	if err != nil {
		return GapRecoveryResult{}, err
	}
	if inspection.State != "candidate_never_claimed" || inspection.EventID != input.EventID {
		if inspection.State == "delivery_pending" {
			return denied("delivery_pending")
		}
		if inspection.State == "within_grace" {
			return denied("within_grace")
		}
		if inspection.Reason != "" {
			return denied(inspection.Reason)
		}
		return denied("identity_mismatch")
	}
	if len(found[0].fingerprint) != 32 {
		return denied("fingerprint_missing")
	}
	var fingerprint [32]byte
	copy(fingerprint[:], found[0].fingerprint)
	appender, err := mysqlstore.Bind(tx)
	if err != nil {
		return GapRecoveryResult{}, err
	}
	after, err := appender.RequeueConfirmed(ctx, mysqlstore.ConfirmedRequeue{
		RecordID: strconv.FormatUint(found[0].id, 10), ExpectedVersion: found[0].version,
		ExpectedFingerprint: fingerprint, RequestID: input.RequestID,
	})
	if err != nil {
		return GapRecoveryResult{}, err
	}
	return GapRecoveryResult{Authorized: true, Code: "authorized", OutboxVersionBefore: found[0].version, OutboxVersionAfter: after}, nil
}

// Resolve reads a committed decision after a lost or unknown client response.
// Missing is not permission to create a different request ID.
func (l *GapRecoveryLedger) Resolve(ctx context.Context, input GapRecoveryRequest) (GapRecoveryResult, bool, error) {
	if l == nil || l.db == nil {
		return GapRecoveryResult{}, false, errors.New("gap recovery ledger is not configured")
	}
	hash, _, err := input.fingerprint()
	if err != nil {
		return GapRecoveryResult{}, false, err
	}
	var storedHash []byte
	var code string
	var authorized bool
	var before, after sql.NullInt64
	err = l.db.QueryRowContext(ctx, `SELECT input_hash,result_code,authorized,outbox_version_before,outbox_version_after
	FROM qs_rm_gap_recovery_request WHERE org_id=? AND request_id=?`, input.OrgID, input.RequestID).
		Scan(&storedHash, &code, &authorized, &before, &after)
	if errors.Is(err, sql.ErrNoRows) {
		return GapRecoveryResult{}, false, nil
	}
	if err != nil {
		return GapRecoveryResult{}, false, err
	}
	if !bytes.Equal(storedHash, hash[:]) {
		return GapRecoveryResult{}, true, ErrGapRecoveryInputConflict
	}
	if code == "" || (authorized && (!before.Valid || !after.Valid)) {
		return GapRecoveryResult{}, true, ErrGapRecoveryLedgerCorrupt
	}
	result := GapRecoveryResult{Authorized: authorized, Code: code}
	if before.Valid {
		result.OutboxVersionBefore = uint64(before.Int64)
	}
	if after.Valid {
		result.OutboxVersionAfter = uint64(after.Int64)
	}
	return result, true, nil
}
