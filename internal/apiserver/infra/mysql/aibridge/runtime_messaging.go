package aibridge

import (
	"context"
	"database/sql"
	"errors"
)

var ErrMessagingLedgerIntegrity = errors.New("AI messaging ledger integrity check failed")

// Runtime counts business commands awaiting admission receipts. The immutable
// archive and exact committed branch identify the current protocol responsibility.
const runtimeArchiveJoin = ` JOIN ai_messaging_archive a ON a.producer='qs-server' AND a.destination='qs-ai' AND a.message_id=CAST(o.command_id AS BINARY)
 JOIN rm_tx_execution e ON e.execution_key=a.execution_key AND e.owner='qs.mysql' AND e.state='committed'
 JOIN rm_tx_witness w ON w.execution_key=a.execution_key AND w.branch_key=a.branch_key AND w.reference=a.reference AND w.state='committed'`
const runtimeCommandRows = ` FROM ai_messaging_operations o` + runtimeArchiveJoin + `
 WHERE o.retired=FALSE AND o.organization_id=r.organization_id AND o.aggregate_key=r.request_id AND o.kind IN (1,2,3)`
const runtimePendingSelect = `(SELECT COUNT(*)` + runtimeCommandRows + ` AND o.decision NOT IN ('accepted','rejected'))`

// The SDK does not expose the retired publication-attempt counter as a business
// or provider invocation budget. The retained JSON value is explicitly unavailable.
const runtimeAttemptsSelect = `0`

type RuntimeMessagingReadConnection interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func requireRuntimeMessagingIntegrity(ctx context.Context, db RuntimeMessagingReadConnection, org int64, requestID string) error {
	operationScope, archiveScope := "", ""
	args := []any{org, org}
	if requestID != "" {
		operationScope = " AND o.aggregate_key=?"
		args = append(args, requestID)
	}
	args = append(args, org, org)
	if requestID != "" {
		archiveScope = " AND o.aggregate_key=?"
		args = append(args, requestID)
	}
	query := `SELECT EXISTS(
 SELECT 1 FROM ai_messaging_operations o
 LEFT JOIN ai_bridge_requests r ON r.request_id=o.aggregate_key
 LEFT JOIN ai_messaging_archive a ON a.producer='qs-server' AND a.destination='qs-ai' AND a.message_id=CAST(o.command_id AS BINARY)
 LEFT JOIN rm_tx_execution e ON e.execution_key=a.execution_key
 LEFT JOIN rm_tx_witness w ON w.execution_key=a.execution_key AND w.branch_key=a.branch_key
 LEFT JOIN rm_tx_request q ON q.request_key=e.request_key
 WHERE o.retired=FALSE AND (o.organization_id=? OR r.organization_id=?) AND o.kind IN(1,2,3)` + operationScope + ` AND (
 a.message_id IS NULL OR e.execution_key IS NULL OR w.branch_key IS NULL OR q.request_key IS NULL
 OR r.request_id IS NULL OR r.organization_id IS NULL OR r.organization_id<>o.organization_id
 OR a.organization_id<>o.organization_id OR e.owner<>'qs.mysql' OR e.state<>'committed' OR w.state<>'committed' OR w.reference<>a.reference
 OR q.owner<>'qs.mysql' OR q.scope<>CAST(CONCAT('org:',o.organization_id) AS BINARY)
 OR q.idempotency_key<>CONCAT(_binary'ai-command/',a.message_id)
 OR o.body_sha256 IS NULL OR o.aggregate_sequence IS NULL OR o.aggregate_sequence=0 OR o.created_at IS NULL
 OR (o.kind=1 AND o.resource_id<>o.aggregate_key)
 OR (o.kind IN(2,3) AND (r.session_id IS NULL OR o.resource_id<>r.session_id))
 OR o.decision NOT IN('','held','accepted','rejected')
 OR (o.decision<>'' AND (o.receipt_id IS NULL OR o.receipt IS NULL OR OCTET_LENGTH(o.receipt)=0 OR o.decided_at IS NULL))
 OR (o.decision='' AND (o.receipt_id IS NOT NULL OR o.receipt IS NOT NULL OR o.decided_at IS NOT NULL OR o.code<>''))
 )
 UNION ALL
 SELECT 1 FROM ai_messaging_archive a
 JOIN rm_tx_execution e ON e.execution_key=a.execution_key
 JOIN rm_tx_request q ON q.request_key=e.request_key
 LEFT JOIN ai_messaging_operations o ON CAST(o.command_id AS BINARY)=a.message_id
 LEFT JOIN ai_bridge_requests r ON r.request_id=o.aggregate_key
 WHERE a.producer='qs-server' AND a.destination='qs-ai' AND (a.organization_id=? OR r.organization_id=?)` + archiveScope + `
 AND q.idempotency_key=CONCAT(_binary'ai-command/',a.message_id)
 AND (o.command_id IS NULL OR o.retired<>FALSE OR o.organization_id<>a.organization_id
 OR e.owner<>'qs.mysql' OR e.state<>'committed' OR q.owner<>'qs.mysql'
 OR q.scope<>CAST(CONCAT('org:',a.organization_id) AS BINARY))
 )`
	var invalid bool
	if err := db.QueryRowContext(ctx, query, args...).Scan(&invalid); err != nil {
		return err
	}
	if invalid {
		return ErrMessagingLedgerIntegrity
	}
	return nil
}
