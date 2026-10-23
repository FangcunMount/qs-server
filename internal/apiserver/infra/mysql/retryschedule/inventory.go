package retryschedule

import (
	"context"
	"database/sql"
	"time"

	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/retryschedule"
	hostmysql "github.com/FangcunMount/qs-server/internal/pkg/database/mysql"
	"github.com/FangcunMount/reliable-messaging/transaction"
)

var _ port.HandoffInspector = (*Store)(nil)

// InspectLegacy uses a fresh server-enforced read-only snapshot. Its 30s total
// maintenance budget is separate from the SDK's 2s Checker budget; it starts no
// workers and holds one borrowed host admission slot until the DB boundary ends.
func (s *Store) InspectLegacy(ctx context.Context, selected []model.Frozen) (model.LegacyInventory, error) {
	if _, active := hostmysql.TxFromContext(ctx); active {
		return model.LegacyInventory{}, transaction.ErrTransactionRequired
	}
	if err := model.ValidateHandoffSelection(s.binding, selected); err != nil {
		return model.LegacyInventory{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ctx, release, err := s.admitted(ctx)
	if err != nil {
		return model.LegacyInventory{}, err
	}
	defer release()
	tx, err := s.primary.BeginTx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return model.LegacyInventory{}, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.validateIn(ctx, tx, false); err != nil {
		return model.LegacyInventory{}, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT state,COUNT(*),COALESCE(SUM(state='published' AND (transport_confirmed_at IS NULL OR transport_confirmed_at<'1970-01-01')),0)
 FROM rm_outbox GROUP BY state LIMIT 7`)
	if err != nil {
		return model.LegacyInventory{}, err
	}
	counts := make(map[string]uint64, 6)
	var unconfirmed uint64
	for rows.Next() {
		var state string
		var count, missing uint64
		if err := rows.Scan(&state, &count, &missing); err != nil {
			_ = rows.Close()
			return model.LegacyInventory{}, err
		}
		counts[state] = count
		unconfirmed += missing
	}
	readErr := rows.Err()
	closeErr := rows.Close()
	if readErr != nil {
		return model.LegacyInventory{}, readErr
	}
	if closeErr != nil {
		return model.LegacyInventory{}, closeErr
	}
	for _, f := range selected {
		if err := s.inspectHeld(ctx, tx, f); err != nil {
			return model.LegacyInventory{}, err
		}
	}
	result, err := model.Inventory(s.binding, counts, uint64(len(selected)), unconfirmed)
	if err != nil {
		return model.LegacyInventory{}, err
	}
	if err := tx.Commit(); err != nil {
		return model.LegacyInventory{}, err
	}
	return result, nil
}

func (s *Store) inspectHeld(ctx context.Context, tx *sql.Tx, f model.Frozen) error {
	in := f.Message().Input()
	var row model.LegacySource
	var duplicates uint64
	var token, replay []byte
	var lease, confirmed sql.NullTime
	var replayVersion sql.NullInt64
	var due string
	err := tx.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM rm_outbox WHERE producer=? AND message_id=? AND destination=?),producer,message_id,destination,event_type,schema_version,scope,content_type,occurred_at,payload,fingerprint,state,last_error_code,DATE_FORMAT(next_attempt_at,'%Y-%m-%d %H:%i:%s.%f'),version,attempt_count,failure_count,claim_token,lease_until,transport_confirmed_at,manual_replay_request_id,manual_replay_version
 FROM rm_outbox WHERE producer=? AND message_id=? AND destination=? AND OCTET_LENGTH(payload)<=262144 LIMIT 1`,
		[]byte(in.Producer), []byte(in.ID), []byte(in.Destination), []byte(in.Producer), []byte(in.ID), []byte(in.Destination)).Scan(
		&duplicates, &row.Input.Producer, &row.Input.ID, &row.Input.Destination, &row.Input.EventType, &row.Input.SchemaVersion, &row.Input.Scope, &row.Input.ContentType, &row.Input.OccurredAt, &row.Input.Payload, &row.Fingerprint, &row.State, &row.Code, &due, &row.Version, &row.Attempts, &row.Failures, &token, &lease, &confirmed, &replay, &replayVersion)
	if err != nil {
		return err
	}
	if duplicates != 1 || row.Version == 0 {
		return model.ErrLegacyOwnership
	}
	row.NextAttemptAt, err = time.ParseInLocation("2006-01-02 15:04:05.000000", due, time.UTC)
	if err != nil {
		return err
	}
	row.HasClaim, row.HasLease, row.HasConfirmation = token != nil, lease.Valid, confirmed.Valid
	row.HasManualReplay = replay != nil || replayVersion.Valid
	already, err := model.MatchLegacy(f, row, time.Microsecond)
	if err != nil {
		return err
	}
	if !already {
		return model.ErrLegacyOwnership
	}
	key := f.Key()
	record, err := s.readRow(tx.QueryRowContext(ctx, "SELECT "+columns+" FROM qs_retry_schedule WHERE schedule_key=? AND OCTET_LENGTH(frozen_plan)<=1048576 AND (dispatch_evidence IS NULL OR OCTET_LENGTH(dispatch_evidence)<=1048576)", key[:]))
	if err != nil {
		return err
	}
	if record.Plan.Digest() != f.Digest() {
		return model.ErrConflict
	}
	return nil
}
