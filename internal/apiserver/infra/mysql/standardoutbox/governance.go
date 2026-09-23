//go:build reliable_messaging_m4

package standardoutbox

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
)

func (r *StatusReader) ReadOutboxGovernance(ctx context.Context, orgID int64) (app.OutboxGovernanceSummary, error) {
	var summary app.OutboxGovernanceSummary
	if r == nil || r.db == nil || orgID <= 0 {
		return summary, fmt.Errorf("standard MySQL governance requires a store and organization")
	}
	err := r.db.QueryRowContext(ctx, `SELECT
 COALESCE(SUM(state='retry_wait' AND (manual_replay_request_id IS NULL OR manual_replay_version IS NULL OR manual_replay_version<>version)),0),
 COALESCE(SUM(state='quarantined' AND last_error_code='publish_unknown'),0),
 COALESCE(SUM(state='retry_wait' AND manual_replay_request_id IS NOT NULL AND manual_replay_version=version),0),
 COALESCE(SUM(state='quarantined' AND last_error_code<>'publish_unknown'),0),
 COALESCE(SUM(state='quarantined' AND last_error_code='publish_unknown'
   AND event_type IN ('evaluation.retry.requested','interpretation.retry.requested')),0)
 FROM rm_outbox WHERE scope=?`, fmt.Sprintf("org:%d", orgID)).Scan(
		&summary.Automatic, &summary.ManualRequired, &summary.Authorized,
		&summary.Terminal, &summary.BlockedRetryEvents)
	return summary, err
}

func (r *StatusReader) ListOutboxCandidates(ctx context.Context, orgID int64, limit int) ([]app.RetryCandidate, error) {
	if r == nil || r.db == nil || orgID <= 0 || limit < 1 || limit > 10101 {
		return nil, fmt.Errorf("invalid standard MySQL candidate query")
	}
	rows, err := r.db.QueryContext(ctx, `SELECT message_id,failure_count,state,
 CASE WHEN state='retry_wait' THEN DATE_FORMAT(next_attempt_at,'%Y-%m-%d %H:%i:%s.%f') ELSE NULL END,
 last_error_code,DATE_FORMAT(updated_at,'%Y-%m-%d %H:%i:%s.%f')
 FROM rm_outbox WHERE scope=? AND (
 (state='retry_wait' AND (manual_replay_request_id IS NULL OR manual_replay_version IS NULL OR manual_replay_version<>version)) OR
 (state='quarantined' AND last_error_code='publish_unknown'))
 ORDER BY updated_at DESC,id DESC LIMIT ?`, fmt.Sprintf("org:%d", orgID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]app.RetryCandidate, 0)
	for rows.Next() {
		var eventID, state, lastError, updatedText string
		var failures uint64
		var nextText sql.NullString
		if err := rows.Scan(&eventID, &failures, &state, &nextText, &lastError, &updatedText); err != nil {
			return nil, err
		}
		if failures > uint64(int(^uint(0)>>1)) {
			return nil, fmt.Errorf("standard MySQL failure count exceeds candidate range")
		}
		updated, err := time.ParseInLocation(standardUTCLayout, updatedText, time.UTC)
		if err != nil {
			return nil, fmt.Errorf("decode standard MySQL update time: %w", err)
		}
		item := app.RetryCandidate{
			Kind: "outbox", Store: "assessment-mysql-outbox", ResourceID: eventID,
			Attempt: int(failures), LastErrorKind: lastError, UpdatedAt: updated,
		}
		switch state {
		case "retry_wait":
			item.Disposition = "automatic"
			if !nextText.Valid {
				return nil, fmt.Errorf("standard MySQL automatic candidate has no due time")
			}
			next, err := time.ParseInLocation(standardUTCLayout, nextText.String, time.UTC)
			if err != nil {
				return nil, fmt.Errorf("decode standard MySQL due time: %w", err)
			}
			item.NextAttemptAt = &next
		case "quarantined":
			item.Disposition = "manual_required"
		default:
			return nil, fmt.Errorf("invalid standard MySQL candidate state %q", state)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

var _ app.OutboxGovernanceReader = (*StatusReader)(nil)
