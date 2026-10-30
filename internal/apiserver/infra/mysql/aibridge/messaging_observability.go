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
	// Current command admission receipts are owned by operations and the exact
	// committed transaction archive. Retired publication queues are not backlog.
	var committed, awaiting, held, oldest, invalid float64
	err = tx.QueryRowContext(ctx, `SELECT /*+ MAX_EXECUTION_TIME(1000) */
 COUNT(*), COALESCE(SUM(o.decision NOT IN ('accepted','rejected')),0),
 COALESCE(SUM(o.decision='held'),0),
 COALESCE(GREATEST(MAX(CASE WHEN o.decision NOT IN ('accepted','rejected') THEN
 TIMESTAMPDIFF(MICROSECOND,o.created_at,UTC_TIMESTAMP(6)) END),0)/1000000,0),
 COALESCE(SUM(a.message_id IS NULL OR e.execution_key IS NULL OR w.branch_key IS NULL OR e.state<>'committed' OR w.state<>'committed'
 OR w.reference<>a.reference),0)
 FROM ai_messaging_operations o
 LEFT JOIN ai_messaging_archive a ON a.producer='qs-server' AND a.destination='qs-ai'
 AND a.message_id=CAST(o.command_id AS BINARY)
 LEFT JOIN rm_tx_execution e ON e.execution_key=a.execution_key AND e.owner='qs.mysql'
 LEFT JOIN rm_tx_witness w ON w.execution_key=a.execution_key AND w.branch_key=a.branch_key
 WHERE o.retired=FALSE AND o.kind IN (1,2,3,4,5)`).Scan(&committed, &awaiting, &held, &oldest, &invalid)
	if err != nil {
		return nil, err
	}
	if invalid != 0 {
		return nil, ErrMessagingLedgerIntegrity
	}
	values["committed_commands"] = committed
	values["awaiting_command_receipts"] = awaiting
	values["held_command_receipts"] = held
	values["oldest_awaiting_command_receipt_seconds"] = oldest
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
