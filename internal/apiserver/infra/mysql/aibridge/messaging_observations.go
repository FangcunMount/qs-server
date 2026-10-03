package aibridge

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var observationKinds = []string{"duplicate_event", "payload_fetch_unavailable", "payload_fetch_reference_mismatch", "payload_fetch_workload_denied", "payload_serve_reference_mismatch", "payload_serve_workload_denied", "payload_serve_storage_unavailable"}

func validObservationKind(kind string) bool {
	for _, allowed := range observationKinds {
		if allowed == kind {
			return true
		}
	}
	return false
}

// RecordMessagingObservation borrows the original active message transaction.
// Rollback also discards the duplicate observation. No identity/body is retained.
func RecordMessagingObservation(ctx context.Context, tx *sql.Tx, kind string) error {
	if tx == nil || !validObservationKind(kind) {
		return errors.New("fixed MQ observation and original transaction required")
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO ai_messaging_observations(kind,recorded_count,recording_since,last_observed_at) VALUES(?,1,UTC_TIMESTAMP(6),UTC_TIMESTAMP(6)) ON DUPLICATE KEY UPDATE recorded_count=IF(recorded_count<18446744073709551615,recorded_count+1,recorded_count),last_observed_at=UTC_TIMESTAMP(6)`, kind)
	return err
}

// RecordPayloadObservation owns only this explicit bounded technical transaction,
// after a failed read. It never commits the caller's business/read transaction.
func RecordPayloadObservation(ctx context.Context, db *sql.DB, kind string) error {
	if db == nil || kind == "duplicate_event" || !validObservationKind(kind) {
		return errors.New("fixed payload observation storage required")
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = RecordMessagingObservation(ctx, tx, kind); err != nil {
		return err
	}
	return tx.Commit()
}
func collectTechnicalObservations(ctx context.Context, tx *sql.Tx) (result map[string]float64, resultErr error) {
	rows, err := tx.QueryContext(ctx, `SELECT /*+ MAX_EXECUTION_TIME(1000) */ kind,recorded_count,TIMESTAMPDIFF(MICROSECOND,'1970-01-01 00:00:00',recording_since)/1000000 FROM ai_messaging_observations`)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			result, resultErr = nil, errors.Join(resultErr, closeErr)
		}
	}()
	counts := map[string]float64{}
	var latest float64
	for rows.Next() {
		var kind string
		var count uint64
		var since float64
		if err = rows.Scan(&kind, &count, &since); err != nil {
			return nil, err
		}
		if validObservationKind(kind) {
			counts["recorded_"+kind] = float64(count)
			if since > latest {
				latest = since
			}
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(counts) != len(observationKinds) {
		return map[string]float64{"duplicate_observations_available": 0, "payload_error_observations_available": 0}, nil
	}
	counts["duplicate_observations_available"] = 1
	counts["payload_error_observations_available"] = 1
	counts["observations_history_complete"] = 0
	counts["observations_recording_since_epoch_seconds"] = latest
	return counts, nil
}

// RequireMessagingObservations validates the fixed recording coverage at startup.
func RequireMessagingObservations(ctx context.Context, tx *sql.Tx) error {
	values, err := collectTechnicalObservations(ctx, tx)
	if err != nil || values["duplicate_observations_available"] != 1 || values["payload_error_observations_available"] != 1 {
		return errors.New("AI MQ required technical observation schema unavailable")
	}
	return nil
}
