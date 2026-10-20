package retryschedule

import (
	"context"
	"database/sql"
	"time"

	model "github.com/FangcunMount/qs-server/internal/apiserver/eventing/retryschedule"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/retryschedule"
	"github.com/FangcunMount/reliable-messaging/transaction"
)

var _ port.HandoffStore = (*Store)(nil)

func (s *Store) TransferLegacy(ctx context.Context, f model.Frozen, authority port.AuthorityValidator) (bool, error) {
	if !f.Valid() || f.Authority().Owner != s.binding.Owner || authority == nil {
		return false, transaction.ErrBinding
	}
	tx, err := s.root(ctx)
	if err != nil {
		return false, err
	}
	// The host owns the root slot; SaveFrozen acquires its own adapter slot.
	// Do not recursively acquire a single-slot limiter around it.
	ctx, cancel := context.WithTimeout(ctx, readBudget)
	defer cancel()
	if err = s.validateIn(ctx, tx, true); err != nil {
		return false, err
	}
	in := f.Message().Input()
	var row model.LegacySource
	var id uint64
	var token, replay []byte
	var lease, confirmed sql.NullTime
	var replayVersion sql.NullInt64
	var nowText, dueText string
	err = tx.QueryRowContext(ctx, `SELECT id,producer,message_id,destination,event_type,schema_version,scope,content_type,occurred_at,payload,fingerprint,state,last_error_code,DATE_FORMAT(next_attempt_at,'%Y-%m-%d %H:%i:%s.%f'),version,attempt_count,failure_count,claim_token,lease_until,transport_confirmed_at,manual_replay_request_id,manual_replay_version,DATE_FORMAT(UTC_TIMESTAMP(6),'%Y-%m-%d %H:%i:%s.%f')
 FROM rm_outbox WHERE producer=? AND message_id=? AND destination=? FOR UPDATE`, []byte(in.Producer), []byte(in.ID), []byte(in.Destination)).Scan(&id, &row.Input.Producer, &row.Input.ID, &row.Input.Destination, &row.Input.EventType, &row.Input.SchemaVersion, &row.Input.Scope, &row.Input.ContentType, &row.Input.OccurredAt, &row.Input.Payload, &row.Fingerprint, &row.State, &row.Code, &dueText, &row.Version, &row.Attempts, &row.Failures, &token, &lease, &confirmed, &replay, &replayVersion, &nowText)
	if err != nil {
		return false, err
	}
	// The old SDK stores UTC clock digits using strings, independent of the
	// business driver's loc. Parse those digits explicitly instead of relabeling
	// a host-local time.Time returned by database/sql.
	row.NextAttemptAt, err = time.ParseInLocation("2006-01-02 15:04:05.000000", dueText, time.UTC)
	if err != nil {
		return false, err
	}
	now, err := time.ParseInLocation("2006-01-02 15:04:05.000000", nowText, time.UTC)
	if err != nil {
		return false, err
	}
	row.HasClaim = token != nil
	row.HasLease = lease.Valid
	row.HasConfirmation = confirmed.Valid
	row.HasManualReplay = replay != nil || replayVersion.Valid
	already, err := model.MatchLegacy(f, row, time.Microsecond)
	if err != nil {
		return false, err
	}
	if already {
		key := f.Key()
		record, err := s.readRow(tx.QueryRowContext(ctx, "SELECT "+columns+" FROM qs_retry_schedule WHERE schedule_key=? FOR UPDATE", key[:]))
		if err != nil {
			return false, err
		}
		if record.Plan.Digest() != f.Digest() {
			return false, model.ErrConflict
		}
		return true, nil
	}
	if !row.NextAttemptAt.After(now) {
		return false, model.ErrLegacyOwnership
	}
	winner, err := s.SaveFrozen(ctx, f)
	if err != nil {
		return false, err
	}
	if winner.Digest() != f.Digest() {
		return false, model.ErrConflict
	}
	if err = authority.ValidateDispatchAuthority(ctx, f); err != nil {
		return false, err
	}
	changed, err := tx.ExecContext(ctx, `UPDATE rm_outbox SET state='held',last_error_code=?,version=version+1,updated_at=UTC_TIMESTAMP(6)
 WHERE id=? AND version=? AND state='pending' AND attempt_count=0 AND failure_count=0
 AND claim_token IS NULL AND lease_until IS NULL AND transport_confirmed_at IS NULL
 AND manual_replay_request_id IS NULL AND manual_replay_version IS NULL AND next_attempt_at>UTC_TIMESTAMP(6)`, model.HandoffMarker(f), id, row.Version)
	if err != nil {
		return false, err
	}
	count, err := changed.RowsAffected()
	if err != nil {
		return false, err
	}
	if count != 1 {
		return false, model.ErrLegacyOwnership
	}
	return false, nil
}
