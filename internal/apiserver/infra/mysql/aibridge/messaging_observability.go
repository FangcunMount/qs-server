package aibridge

import (
	"context"
	"database/sql"
	"errors"
)

// MessagingSnapshot borrows the host pool for one consistent, read-only snapshot.
// The caller bounds its context. No payload/identity, scheduling, commit or close.
func MessagingSnapshot(ctx context.Context, db *sql.DB) (map[string]float64, error) {
	if db == nil {
		return nil, errors.New("MQ observation storage unavailable")
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	values := map[string]float64{}
	keys := []string{"staged_messages", "due_messages", "awaiting_receipt_messages", "held_messages", "oldest_staged_seconds", "oldest_awaiting_receipt_seconds"}
	var staged, due, awaiting, held, stagedAge, awaitingAge float64
	err = tx.QueryRowContext(ctx, `SELECT /*+ MAX_EXECUTION_TIME(1000) */
 COALESCE(SUM(stage='staged'),0),
 COALESCE(SUM(stage IN('staged','awaiting_receipt') AND available_at<=UTC_TIMESTAMP(6)),0),
 COALESCE(SUM(stage='awaiting_receipt'),0),COALESCE(SUM(stage='held'),0),
 COALESCE(GREATEST(MAX(CASE WHEN stage='staged' THEN TIMESTAMPDIFF(MICROSECOND,created_at,UTC_TIMESTAMP(6)) END),0)/1000000,0),
 COALESCE(GREATEST(MAX(CASE WHEN stage='awaiting_receipt' THEN TIMESTAMPDIFF(MICROSECOND,created_at,UTC_TIMESTAMP(6)) END),0)/1000000,0)
 FROM ai_messaging_outbox`).Scan(&staged, &due, &awaiting, &held, &stagedAge, &awaitingAge)
	if err != nil {
		return nil, err
	}
	for i, v := range []float64{staged, due, awaiting, held, stagedAge, awaitingAge} {
		values[keys[i]] = v
	}
	var inboxHeld float64
	if err = tx.QueryRowContext(ctx, "SELECT /*+ MAX_EXECUTION_TIME(1000) */ COUNT(*) FROM ai_messaging_inbox WHERE outcome='held'").Scan(&inboxHeld); err != nil {
		return nil, err
	}
	values["held_inbox_events"] = inboxHeld
	for _, code := range []string{"authentication_failed", "identity_conflict", "technical_budget_exhausted", "failed_delivery"} {
		var n float64
		if err = tx.QueryRowContext(ctx, "SELECT /*+ MAX_EXECUTION_TIME(1000) */ COUNT(*) FROM ai_messaging_quarantine WHERE code=?", code).Scan(&n); err != nil {
			return nil, err
		}
		values["quarantine_"+code+"_records"] = n
	}
	technical, err := collectTechnicalObservations(ctx, tx)
	if err != nil {
		return nil, err
	}
	for kind, value := range technical {
		values[kind] = value
	}
	return values, nil
}
