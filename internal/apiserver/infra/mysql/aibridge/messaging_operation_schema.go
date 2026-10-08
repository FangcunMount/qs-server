package aibridge

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// RequireMessagingOperationSchema is read-only and borrows the host transaction.
// Presence of a column alone does not prove enforced retirement shape or usable
// bounded request/organization statistics indexes.
func RequireMessagingOperationSchema(ctx context.Context, tx *sql.Tx) error {
	if tx == nil {
		return errors.New("AI MQ required operation schema unavailable")
	}
	rows, err := tx.QueryContext(ctx, "SELECT retired,retirement_evidence,retired_at,body_sha256,aggregate_sequence,created_at FROM ai_messaging_operations LIMIT 0")
	if err != nil {
		return errors.New("AI MQ required operation schema unavailable")
	}
	if err = rows.Close(); err != nil {
		return errors.New("AI MQ operation schema read unavailable")
	}
	var enforced int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.TABLE_CONSTRAINTS WHERE CONSTRAINT_SCHEMA=DATABASE() AND TABLE_NAME='ai_messaging_operations' AND CONSTRAINT_NAME='chk_ai_messaging_operations_retirement' AND CONSTRAINT_TYPE='CHECK' AND ENFORCED='YES'`).Scan(&enforced)
	if err != nil || enforced != 1 {
		return errors.New("AI MQ required retirement constraint unavailable")
	}
	expected := map[string][]string{
		"idx_ai_messaging_operations_request_stats": {"organization_id", "retired", "aggregate_key", "kind", "decision", "command_id"},
		"idx_ai_messaging_operations_pending_stats": {"organization_id", "retired", "kind", "decision", "aggregate_key", "command_id"},
	}
	indexes, err := tx.QueryContext(ctx, `SELECT INDEX_NAME,SEQ_IN_INDEX,COLUMN_NAME,SUB_PART,NON_UNIQUE,INDEX_TYPE,IS_VISIBLE FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='ai_messaging_operations' AND INDEX_NAME IN ('idx_ai_messaging_operations_request_stats','idx_ai_messaging_operations_pending_stats') ORDER BY INDEX_NAME,SEQ_IN_INDEX`)
	if err != nil {
		return errors.New("AI MQ required operation statistics indexes unavailable")
	}
	defer func() { _ = indexes.Close() }()
	seen := map[string]int{}
	for indexes.Next() {
		var name, column, indexType, visible string
		var ordinal, nonUnique int
		var prefix sql.NullInt64
		if err = indexes.Scan(&name, &ordinal, &column, &prefix, &nonUnique, &indexType, &visible); err != nil {
			return errors.New("AI MQ operation statistics index read unavailable")
		}
		columns, ok := expected[name]
		if !ok || ordinal != seen[name]+1 || ordinal > len(columns) || columns[ordinal-1] != column || prefix.Valid || nonUnique != 1 || !strings.EqualFold(indexType, "BTREE") || visible != "YES" {
			return errors.New("AI MQ required operation statistics index mismatch")
		}
		seen[name]++
	}
	if indexes.Err() != nil {
		return errors.New("AI MQ operation statistics index read unavailable")
	}
	for name, columns := range expected {
		if seen[name] != len(columns) {
			return errors.New("AI MQ required operation statistics indexes unavailable")
		}
	}
	return nil
}
