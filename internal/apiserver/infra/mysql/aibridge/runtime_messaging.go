package aibridge

import (
	"context"
	"database/sql"
	"errors"
)

// ErrMessagingLedgerIntegrity prevents an incomplete or conflicting ledger from
// being reported as zero pending commands. Its message carries no identities.
var ErrMessagingLedgerIntegrity = errors.New("AI messaging ledger integrity check failed")

// Request statistics are business commands, not ACKs or evaluation-run messages.
// A HELD command still has unsettled responsibility. Attempts are the persisted
// delivery budget, including a budget inherited by an already verified handoff.
const runtimeCommandRows = ` FROM ai_messaging_operations o
 JOIN ai_messaging_outbox b ON b.producer='qs-server' AND b.destination='qs-ai' AND b.message_id=o.command_id
 WHERE o.retired=FALSE AND o.organization_id=r.organization_id AND o.aggregate_key=r.request_id
 AND o.kind IN (1,2,3) AND b.kind=o.kind AND b.topic='qs.ai.commands.v1'
 AND b.requires_receipt=TRUE AND b.organization_id=o.organization_id AND b.aggregate_key=o.aggregate_key`

const runtimePendingSelect = `(SELECT COUNT(*)` + runtimeCommandRows + ` AND o.decision NOT IN ('accepted','rejected'))`
const runtimeAttemptsSelect = `(SELECT COALESCE(SUM(b.attempts),0)` + runtimeCommandRows + `)`

// Check both directions: an inner join alone hides orphaned operations and
// messages. Request ownership also scopes both scans: two matching forged
// ledger organizations must not hide a command from its actual request owner.
// The checks distinguish broker settlement from business receipt.
func requireRuntimeMessagingIntegrity(ctx context.Context, db *sql.DB, org int64, requestID string) error {
	operationScope, messageScope := "", ""
	args := []any{org, org}
	if requestID != "" {
		operationScope = " AND o.aggregate_key=?"
		args = append(args, requestID)
	}
	args = append(args, org, org)
	if requestID != "" {
		messageScope = " AND b.aggregate_key=?"
		args = append(args, requestID)
	}
	query := `SELECT EXISTS(
 SELECT 1 FROM ai_messaging_operations o
 LEFT JOIN ai_messaging_outbox b ON b.producer='qs-server' AND b.destination='qs-ai' AND b.message_id=o.command_id
 LEFT JOIN ai_bridge_requests r ON r.request_id=o.aggregate_key
 WHERE o.retired=FALSE AND (o.organization_id=? OR r.organization_id=?) AND o.kind IN (1,2,3)` + operationScope + ` AND (
 b.message_id IS NULL OR r.request_id IS NULL OR r.organization_id IS NULL OR r.organization_id<>o.organization_id
 OR o.body_sha256 IS NULL OR o.aggregate_sequence IS NULL OR o.created_at IS NULL
 OR b.body_sha256<>o.body_sha256 OR b.aggregate_sequence<>o.aggregate_sequence
 OR b.organization_id<>o.organization_id OR b.aggregate_key<>o.aggregate_key OR b.kind<>o.kind
 OR b.topic<>'qs.ai.commands.v1' OR b.requires_receipt<>TRUE OR b.ordered<>TRUE
 OR (o.kind=1 AND o.resource_id<>o.aggregate_key)
 OR (o.kind IN (2,3) AND (r.session_id IS NULL OR o.resource_id<>r.session_id))
 OR b.stage NOT IN ('staged','awaiting_receipt','held','confirmed')
 OR o.decision NOT IN ('','held','accepted','rejected')
 OR (o.decision IN ('accepted','rejected') AND (b.stage<>'confirmed' OR o.receipt_id IS NULL OR o.receipt IS NULL OR OCTET_LENGTH(o.receipt)=0 OR o.decided_at IS NULL OR b.confirmed_at IS NULL))
 OR (o.decision NOT IN ('accepted','rejected') AND b.stage='confirmed')
 OR (o.decision='held' AND b.stage<>'held')
 )
 UNION ALL
 SELECT 1 FROM ai_messaging_outbox b
 LEFT JOIN ai_messaging_operations o ON o.command_id=b.message_id
 LEFT JOIN ai_bridge_requests r ON r.request_id=b.aggregate_key
 WHERE b.producer='qs-server' AND b.destination='qs-ai' AND (b.organization_id=? OR r.organization_id=?) AND b.kind IN (1,2,3)` + messageScope + ` AND (
 o.command_id IS NULL OR o.retired<>FALSE OR o.organization_id<>b.organization_id
 OR o.aggregate_key<>b.aggregate_key OR o.kind<>b.kind)
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
