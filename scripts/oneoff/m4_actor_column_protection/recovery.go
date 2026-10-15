package main

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"reflect"
	"strings"
	"time"
)

// These immutable original resources supply only the three fixed column
// statements. No SQL supplied by a capture or approval is ever executed.
//
//go:embed original_actor_schema.sql
var originalActorSchema string

//go:embed original_role_projection.sql
var originalRoleProjection string

const originalActorHash = "3129f7f4e40c4ea58463601cb30fd76b04bf4693b899ed678089de57dcfc9dd4"
const originalRoleHash = "2d1c10ff9a1122d9a919e0e1fc7d97dedb0d9caaa85a6736d65affa57ccbb1d6"

type columnState uint8

const (
	columnAbsent columnState = iota
	columnOriginal
	columnTemporaryNullable
)

type recoveryState struct {
	Material material
	Columns  [2]columnState
}

func fixedRestorationSQL() ([3]string, error) {
	var out [3]string
	if fingerprint([]byte(originalActorSchema)) != originalActorHash || fingerprint([]byte(originalRoleProjection)) != originalRoleHash {
		return out, errors.New("compiled_original_column_resources_changed")
	}
	lines := []struct{ source, line, statement string }{
		{originalActorSchema, "`tags` json DEFAULT NULL COMMENT '标签（JSON数组）',", "ALTER TABLE `testee` ADD COLUMN `tags` json DEFAULT NULL COMMENT '标签（JSON数组）' AFTER `birthday`"},
		{originalRoleProjection, "ADD COLUMN `effective_roles` JSON NULL AFTER `roles`,", "ALTER TABLE `operators` ADD COLUMN `effective_roles` JSON NULL AFTER `roles`"},
		{originalRoleProjection, "MODIFY COLUMN `effective_roles` JSON NOT NULL;", "ALTER TABLE `operators` MODIFY COLUMN `effective_roles` JSON NOT NULL"},
	}
	for i, fixed := range lines {
		count := 0
		for _, line := range strings.Split(fixed.source, "\n") {
			if strings.TrimSpace(line) == fixed.line {
				count++
			}
		}
		if count != 1 {
			return [3]string{}, errors.New("compiled_original_column_statement_rejected")
		}
		out[i] = fixed.statement
	}
	return out, nil
}

func restoredColumnShape(text string, i int) (columnState, string, error) {
	if i < 0 || i > 1 {
		return 0, "", errors.New("recovery_column_scope_rejected")
	}
	present := false
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "  `"+scope[i].target+"` ") {
			present = true
		}
	}
	if !present {
		return columnAbsent, text, nil
	}
	normalized := text
	state := columnOriginal
	if i == 1 && strings.Contains(text, "  `effective_roles` json DEFAULT NULL,\n") {
		normalized = strings.Replace(text, "  `effective_roles` json DEFAULT NULL,\n", "  `effective_roles` json NOT NULL,\n", 1)
		state = columnTemporaryNullable
	}
	retained, e := retainedDDL(normalized, i, true)
	if e != nil {
		return 0, "", e
	}
	return state, retained, nil
}

func sameValue(a, b *originalValue) bool {
	return a != nil && b != nil && a.Kind == b.Kind && bytes.Equal(a.JSON, b.JSON)
}

// allowContinuation is fixed BEFORE this invocation can create a boundary or
// consume an operation. A newly made boundary never admits a foreign partial
// nullable column or partial row population on its own.
func validateRecoveryState(original material, current recoveryState, allowContinuation bool) error {
	m := current.Material
	if m.Format != original.Format || m.Kind != original.Kind || m.Binding != original.Binding || m.ToolSource != original.ToolSource || m.CSource != cSource || m.CResources != cResources || m.CResourceFiles != cResourceFiles || m.SessionZone != original.SessionZone || m.SessionCharset != original.SessionCharset || m.SessionSQLMode != original.SessionSQLMode || m.LedgerDDL != original.LedgerDDL || len(m.Ledger) != 1 || m.Ledger[0].Version != 102 || !bytes.Equal(m.MongoHead, original.MongoHead) {
		return errors.New("restore_identity_or_original_ledger_rejected")
	}
	tags, roles := current.Columns[0], current.Columns[1]
	if tags > columnOriginal || roles > columnTemporaryNullable || (roles == columnAbsent && tags != columnAbsent) || (roles == columnTemporaryNullable && (tags != columnAbsent || !allowContinuation)) || (!allowContinuation && !m.Ledger[0].Dirty && (tags != columnAbsent || roles != columnAbsent)) {
		return errors.New("unproved_restore_prefix_or_partial_state")
	}
	for i, old := range original.Tables {
		now := m.Tables[i]
		if now.Table != old.Table || !now.EOF || now.Pages != old.Pages || len(now.Rows) != len(old.Rows) || !reflect.DeepEqual(now.Columns, old.Columns) || now.RetainedDDL != old.RetainedDDL {
			return errors.New("restore_retained_structure_or_complete_range_changed")
		}
		if i >= 2 {
			if now.DDL != old.DDL {
				return errors.New("modern_archive_original_ddl_changed")
			}
		} else {
			want := old.RetainedDDL
			switch current.Columns[i] {
			case columnOriginal:
				want = old.DDL
			case columnTemporaryNullable:
				want = strings.Replace(old.DDL, "  `effective_roles` json NOT NULL,\n", "  `effective_roles` json DEFAULT NULL,\n", 1)
			}
			if now.DDL != want {
				return errors.New("restore_exact_column_definition_changed")
			}
		}
		for n, before := range old.Rows {
			after := now.Rows[n]
			if before.Key != after.Key || before.Retained != after.Retained {
				return errors.New("restore_retained_fact_or_key_changed")
			}
			if i >= 2 || current.Columns[i] == columnAbsent {
				if after.Value != nil {
					return errors.New("restore_unexpected_target_value")
				}
				continue
			}
			if sameValue(before.Value, after.Value) {
				continue
			}
			partial := allowContinuation && (i == 0 || current.Columns[i] == columnTemporaryNullable)
			if !partial || !checkValue(after.Value) || after.Value.Kind != "sql_null" {
				return errors.New("restore_original_value_changed")
			}
		}
	}
	return nil
}

func (d database) recoverySnapshot(ctx context.Context, tool string) (out recoveryState, resultErr error) {
	if e := requireTransactionalTables(ctx, d.conn); e != nil {
		return out, e
	}
	before, e := d.observe(ctx, d.conn)
	if e != nil || before.Ledger.Version != 102 {
		return out, errors.New("restore_requires_original10240_state")
	}
	tx, e := d.conn.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if e != nil {
		return out, errors.New("restore_readonly_snapshot_failed")
	}
	done := false
	defer func() {
		if !done {
			recordRollback(&resultErr, tx)
		}
	}()
	m := material{Format: 1, Kind: "m4_c_actor_column_protection", ToolSource: tool, CSource: cSource, CResources: cResources, CResourceFiles: cResourceFiles, Binding: before.Binding, SessionZone: "+00:00", SessionCharset: "utf8mb4", SessionSQLMode: sessionMode, MongoHead: before.Mongo}
	if e := requireLedgerShape(ctx, tx); e != nil {
		return out, e
	}
	m.Ledger, e = readLedger(ctx, tx)
	if e != nil || m.Ledger[0] != before.Ledger {
		return out, errors.New("restore_snapshot_ledger_changed")
	}
	m.LedgerDDL, e = ddl(ctx, tx, "schema_migrations")
	if e != nil {
		return out, e
	}
	used := 0
	for i, item := range scope {
		text, e := ddl(ctx, tx, item.table)
		if e != nil {
			return out, e
		}
		retained, state := text, columnAbsent
		if i < 2 {
			state, retained, e = restoredColumnShape(text, i)
			if e != nil {
				return out, e
			}
			out.Columns[i] = state
		}
		cols, e := recoveryColumns(ctx, tx, i, state != columnAbsent, state == columnTemporaryNullable)
		if e != nil {
			return out, e
		}
		t, e := scanTable(ctx, tx, i, cols, state != columnAbsent, &used)
		if e != nil {
			return out, e
		}
		t.DDL, t.RetainedDDL = text, retained
		m.Tables[i] = t
	}
	if tx.Commit() != nil {
		return out, errors.New("restore_readonly_snapshot_commit_unproven")
	}
	done = true
	after, e := d.observe(ctx, d.conn)
	if e != nil || !reflect.DeepEqual(before, after) {
		return out, errors.New("restore_post_window_binding_changed")
	}
	out.Material = m
	return out, nil
}

const valueBatchRows = 100
const valueBatchBytes = 1 << 20

func valueBatchSQL(i, count int, cols []column) (string, error) {
	if i < 0 || i > 1 || count < 1 || count > valueBatchRows {
		return "", errors.New("fixed_value_batch_scope_rejected")
	}
	var cases, conditions []string
	for n := 0; n < count; n++ {
		cases = append(cases, "WHEN `id`=CAST(? AS DECIMAL(20,0)) THEN CAST(? AS JSON)")
		conditions = append(conditions, "(`id`=CAST(? AS DECIMAL(20,0)) AND `"+scope[i].target+"` IS NULL AND "+retainedHashSQL(cols)+"=?)")
	}
	return "UPDATE `" + scope[i].table + "` SET `" + scope[i].target + "`=CASE " + strings.Join(cases, " ") + " ELSE `" + scope[i].target + "` END,`updated_at`=`updated_at` WHERE " + strings.Join(conditions, " OR "), nil
}

func (d *database) fillOriginalValues(ctx context.Context, m material, state recoveryState, i int, a approval, hash, tool string) error {
	var pending []rowProof
	for n, row := range m.Tables[i].Rows {
		if !sameValue(row.Value, state.Material.Tables[i].Rows[n].Value) {
			if row.Value.Kind != "json" || state.Material.Tables[i].Rows[n].Value == nil || state.Material.Tables[i].Rows[n].Value.Kind != "sql_null" {
				return errors.New("original_value_cas_input_rejected")
			}
			pending = append(pending, row)
		}
	}
	for len(pending) > 0 {
		n, used := 0, 0
		for n < len(pending) && n < valueBatchRows {
			size := len(pending[n].Value.JSON)
			if n > 0 && used+size > valueBatchBytes {
				break
			}
			used += size
			n++
		}
		if e := d.commitValueBatch(ctx, m, pending[:n], i, a, hash, tool); e != nil {
			return e
		}
		pending = pending[n:]
	}
	return nil
}

func (d *database) commitValueBatch(ctx context.Context, m material, rows []rowProof, i int, a approval, hash, tool string) (resultErr error) {
	query, e := valueBatchSQL(i, len(rows), m.Tables[i].Columns)
	if e != nil {
		return e
	}
	var args []any
	for _, row := range rows {
		args = append(args, row.Key, row.Value.JSON)
	}
	for _, row := range rows {
		args = append(args, row.Key, row.Retained)
	}
	if e := validateMutableBoundary(m, a, hash, tool); e != nil {
		return e
	}
	tx, e := d.conn.BeginTx(ctx, nil)
	if e != nil {
		return errors.New("original_value_transaction_begin_failed")
	}
	committed := false
	defer func() {
		if !committed {
			recordRollback(&resultErr, tx)
		}
	}()
	if e := validateMutableBoundary(m, a, hash, tool); e != nil {
		return e
	}
	q, cancel := context.WithTimeout(ctx, queryLimit)
	r, e := tx.ExecContext(q, query, args...)
	cancel()
	if e != nil {
		return errors.New("original_value_batch_failed_or_unknown")
	}
	affected, e := r.RowsAffected()
	if e != nil || affected != int64(len(rows)) {
		return errors.New("original_value_cas_count_rejected")
	}
	if e := validateMutableBoundary(m, a, hash, tool); e != nil {
		return e
	}
	if tx.Commit() != nil {
		return errors.New("original_value_commit_outcome_unknown")
	}
	committed = true
	return nil
}

type transactionReleaseFailure struct{ operation error }

func (e *transactionReleaseFailure) Error() string { return e.operation.Error() }
func (e *transactionReleaseFailure) Unwrap() error { return e.operation }
func recordRollback(result *error, tx interface{ Rollback() error }) {
	if e := tx.Rollback(); e != nil && !errors.Is(e, sql.ErrTxDone) {
		if *result == nil {
			*result = errors.New("transaction_release_failed")
		}
		*result = &transactionReleaseFailure{operation: *result}
	}
}

func rereadApprovedInputs(m material, a approval, path, hash, approvalPath, approvalHash, tool string) error {
	raw, e := readPrivate(path, hash)
	var again material
	if e != nil || strictDecode(raw, &again) != nil || validateMaterial(again, tool, m.Binding) != nil || again.CapturePath != path || m.CapturePath != path || !reflect.DeepEqual(m, again) {
		return errors.New("frozen_material_changed_before_claim")
	}
	raw, e = readPrivateLimit(approvalPath, approvalHash, 32768)
	var approved approval
	if e != nil || len(raw) > 32768 || strictDecode(raw, &approved) != nil || approved != a {
		return errors.New("frozen_approval_changed_before_claim")
	}
	return validateApproval(approved, again, hash, tool, time.Now())
}

func (d *database) restore(ctx context.Context, m material, a approval, materialPath, materialHash, approvalPath, approvalHash, tool string, consumed *bool) error {
	prior, e := readRecoveryBoundary(m, a, materialHash)
	if e != nil {
		return e
	}
	current, e := d.recoverySnapshot(ctx, tool)
	if e != nil {
		return e
	}
	if e := validateRecoveryState(m, current, prior); e != nil {
		return e
	}
	statements, e := fixedRestorationSQL()
	if e != nil {
		return e
	}
	if e := rereadApprovedInputs(m, a, materialPath, materialHash, approvalPath, approvalHash, tool); e != nil {
		return e
	}
	if e := consumeApproval(m, a, approvalHash); e != nil {
		return e
	}
	*consumed = true
	for _, i := range []int{1, 0} {
		if current.Columns[i] == columnAbsent {
			if e := validateMutableBoundary(m, a, materialHash, tool); e != nil {
				return e
			}
			if _, e := d.conn.ExecContext(ctx, statements[i]); e != nil {
				return errors.New("column_add_failed_or_unknown")
			}
			current, e = d.recoverySnapshot(ctx, tool)
			if e != nil {
				return e
			}
			if e := validateRecoveryState(m, current, true); e != nil {
				return e
			}
		}
		if e := d.fillOriginalValues(ctx, m, current, i, a, materialHash, tool); e != nil {
			return e
		}
		if i == 1 && current.Columns[i] == columnTemporaryNullable {
			if e := validateMutableBoundary(m, a, materialHash, tool); e != nil {
				return e
			}
			if _, e := d.conn.ExecContext(ctx, statements[2]); e != nil {
				return errors.New("original_not_null_restore_failed_or_unknown")
			}
		}
		current, e = d.recoverySnapshot(ctx, tool)
		if e != nil {
			return e
		}
		if e := validateRecoveryState(m, current, true); e != nil {
			return e
		}
	}
	if e := requireFullyRestored(m, current); e != nil {
		return e
	}
	if e := d.replayLedgerLast(ctx, m, current.Material.Ledger[0], a, materialHash, tool); e != nil {
		return e
	}
	final, e := d.snapshot(ctx, tool, 101, true)
	if e != nil || !reflect.DeepEqual(final.Tables, m.Tables) || !reflect.DeepEqual(final.Ledger, m.Ledger) || final.LedgerDDL != m.LedgerDDL || !bytes.Equal(final.MongoHead, m.MongoHead) {
		return errors.New("final_original_values_ledger_or_binding_unproven")
	}
	return nil
}

func requireFullyRestored(original material, state recoveryState) error {
	if state.Columns != ([2]columnState{columnOriginal, columnOriginal}) || !reflect.DeepEqual(state.Material.Tables, original.Tables) {
		return errors.New("full_original_values_or_archives_not_restored")
	}
	return nil
}

func (d *database) replayLedgerLast(ctx context.Context, m material, expected ledgerRow, a approval, hash, tool string) (resultErr error) {
	observed, e := d.observe(ctx, d.conn)
	if e != nil || observed.Ledger != expected || !bytes.Equal(observed.Mongo, m.MongoHead) {
		return errors.New("binding_or_ledger_changed_before_last_transaction")
	}
	if e := validateMutableBoundary(m, a, hash, tool); e != nil {
		return e
	}
	tx, e := d.conn.BeginTx(ctx, nil)
	if e != nil {
		return errors.New("ledger_transaction_begin_failed")
	}
	committed := false
	defer func() {
		if !committed {
			recordRollback(&resultErr, tx)
		}
	}()
	rows, e := readLedger(ctx, tx)
	if e != nil || rows[0] != expected {
		return errors.New("original102_ledger_changed_before_replay")
	}
	if e := validateMutableBoundary(m, a, hash, tool); e != nil {
		return e
	}
	deleted, e := tx.ExecContext(ctx, "DELETE FROM schema_migrations")
	if e != nil {
		return errors.New("ledger_transaction_delete_failed")
	}
	if n, e := deleted.RowsAffected(); e != nil || n != 1 {
		return errors.New("ledger_transaction_delete_count_rejected")
	}
	for _, row := range m.Ledger {
		if e := validateMutableBoundary(m, a, hash, tool); e != nil {
			return e
		}
		if _, e := tx.ExecContext(ctx, "INSERT INTO schema_migrations(version,dirty) VALUES(?,?)", row.Version, row.Dirty); e != nil {
			return errors.New("original_ledger_insert_failed")
		}
	}
	final, e := d.observe(ctx, tx)
	if e != nil || final.Ledger != m.Ledger[0] || !bytes.Equal(final.Mongo, m.MongoHead) {
		return errors.New("binding_or_original_ledger_changed_before_commit")
	}
	if e := validateMutableBoundary(m, a, hash, tool); e != nil {
		return e
	}
	if tx.Commit() != nil {
		return errors.New("ledger_commit_outcome_unknown")
	}
	committed = true
	return nil
}

func restoreActionContext(ctx context.Context, a approval) (context.Context, context.CancelFunc) {
	deadline, _ := time.Parse(time.RFC3339Nano, a.Deadline)
	expiry, _ := time.Parse(time.RFC3339Nano, a.Expires)
	if expiry.Before(deadline) {
		deadline = expiry
	}
	return context.WithDeadline(ctx, deadline)
}
