package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"time"
)

type recoveryState struct {
	Material material
	Missing  []bool
}

func (d database) recoveryPresence(ctx context.Context, r sqlReader) ([]bool, error) {
	q, cancel := context.WithTimeout(ctx, queryLimit)
	defer cancel()
	rows, e := r.QueryContext(q, fixedTablesQuery)
	if e != nil {
		return nil, errors.New("restore_table_presence_unavailable")
	}
	seen := map[string]bool{}
	for rows.Next() {
		var name, kind, engine string
		if rows.Scan(&name, &kind, &engine) != nil || kind != "BASE TABLE" || engine != "InnoDB" || seen[name] {
			_ = rows.Close()
			return nil, errors.New("restore_fixed_innodb_tables_required")
		}
		seen[name] = true
	}
	re, ce := rows.Err(), rows.Close()
	if re != nil || ce != nil || !seen["schema_migrations"] {
		return nil, errors.New("restore_table_presence_or_release_rejected")
	}
	for _, s := range retainedScope {
		if !seen[s.name] {
			return nil, errors.New("restore_modern_table_missing")
		}
		delete(seen, s.name)
	}
	delete(seen, "schema_migrations")
	missing := make([]bool, len(scope))
	for i, s := range scope {
		missing[i] = !seen[s.name]
		delete(seen, s.name)
	}
	if len(seen) != 0 {
		return nil, errors.New("restore_table_scope_rejected")
	}
	return missing, nil
}

func (d database) recoverySnapshot(ctx context.Context, tool string) (out recoveryState, resultErr error) {
	before, e := d.observe(ctx, d.conn)
	if e != nil || before.Ledger.Version != 103 {
		return out, errors.New("restore_requires_sql103_mongo40")
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
	if e := requireLedgerShape(ctx, tx); e != nil {
		return out, e
	}
	m := material{Format: 3, Kind: "m4_d_maintenance_table_protection", ToolSource: tool, DSource: dSource, DResources: dResources, DResourceFiles: dResourceFiles, Binding: before.Binding, RuntimeBoundary: before.RuntimeBoundary.State, SessionZone: "+00:00", SessionCharset: "utf8mb4", SessionSQLMode: sessionMode, MongoHead: before.Mongo}
	m.Ledger, e = readLedger(ctx, tx)
	if e != nil || m.Ledger[0] != before.Ledger {
		return out, errors.New("restore_snapshot_ledger_changed")
	}
	m.LedgerDDL, e = ddl(ctx, tx, "schema_migrations")
	if e != nil {
		return out, e
	}
	out.Missing, e = d.recoveryPresence(ctx, tx)
	if e != nil {
		return out, e
	}
	totalRows, totalBytes := 0, 0
	for i, s := range scope {
		if out.Missing[i] {
			continue
		}
		cols, e := columns(ctx, tx, i)
		if e != nil {
			return out, e
		}
		t, e := scanTable(ctx, tx, i, cols, &totalRows, &totalBytes)
		if e != nil {
			return out, e
		}
		t.DDL, e = ddl(ctx, tx, s.name)
		if e != nil {
			return out, e
		}
		if e := qualifyOriginalDDL(i, t.DDL); e != nil {
			return out, e
		}
		m.Tables[i] = t
	}
	for i := range retainedScope {
		m.Retained[i], e = scanRetained(ctx, tx, i, &totalRows, &totalBytes)
		if e != nil {
			return out, e
		}
	}
	if tx.Commit() != nil {
		return out, errors.New("restore_readonly_commit_unproven")
	}
	done = true
	after, e := d.observe(ctx, d.conn)
	if e != nil || !reflect.DeepEqual(before, after) {
		return out, errors.New("restore_post_window_binding_changed")
	}
	missing, e := d.recoveryPresence(ctx, d.conn)
	if e != nil || !sameMissing(missing, out.Missing) {
		return out, errors.New("restore_post_window_table_presence_changed")
	}
	out.Material = m
	return out, nil
}

func validateRecoveryState(original material, current recoveryState, prior *recoveryBoundary) error {
	m := current.Material
	if m.Format != original.Format || m.Kind != original.Kind || m.ToolSource != original.ToolSource || m.DSource != dSource || m.DResources != dResources || m.DResourceFiles != dResourceFiles || m.Binding != original.Binding || m.SessionZone != original.SessionZone || m.SessionCharset != original.SessionCharset || m.SessionSQLMode != original.SessionSQLMode || m.LedgerDDL != original.LedgerDDL || len(m.Ledger) != 1 || m.Ledger[0].Version != 103 || !bytes.Equal(m.MongoHead, original.MongoHead) || !sameRetained(m.Retained, original.Retained) {
		return errors.New("restore_retained_identity_or_facts_changed")
	}
	if len(current.Missing) != len(scope) {
		return errors.New("restore_table_state_unknown")
	}
	initial := current.Missing
	if prior == nil {
		if !validInitialRecoveryState(m.Ledger[0], current.Missing) {
			return errors.New("restore_first_drop_prefix_rejected")
		}
	} else {
		initial = prior.InitialMissing
		if !validInitialRecoveryState(prior.InitialLedger, initial) || m.Ledger[0] != prior.InitialLedger {
			return errors.New("restore_initial_ledger_changed")
		}
		if !validInitialRecoveryState(ledgerRow{103, true}, current.Missing) {
			return errors.New("restore_continuation_not_inverse_prefix")
		}
		for i, missing := range current.Missing {
			if missing && !initial[i] {
				return errors.New("restore_missing_set_expanded")
			}
		}
	}
	// Reverse CREATE/INSERT permits only the first currently present originally
	// missing table to be partially populated. Later tables must be complete.
	partialIndex := -1
	if prior != nil {
		for i := range initial {
			if initial[i] && !current.Missing[i] {
				partialIndex = i
				break
			}
		}
	}
	for i, old := range original.Tables {
		now := m.Tables[i]
		if current.Missing[i] {
			if now.Table != "" || len(now.Rows) != 0 {
				return errors.New("restore_absent_table_has_facts")
			}
			continue
		}
		if now.Table != old.Table || now.DDL != old.DDL || !sameColumns(now.Columns, old.Columns) || !sameStrings(now.Keys, old.Keys) || !now.EOF || now.Pages != (len(now.Rows)+pageSize-1)/pageSize+1 {
			return errors.New("restore_original_table_definition_changed")
		}
		if i != partialIndex {
			if !reflect.DeepEqual(now, old) {
				return errors.New("restore_original_suffix_or_completed_table_changed")
			}
			continue
		}
		if !originalRowSubset(scope[i], old.Rows, now.Rows) {
			return errors.New("restore_partial_table_foreign_values")
		}
	}
	return nil
}

func originalRowSubset(s tableSpec, original, current []rowProof) bool {
	n := 0
	for _, row := range current {
		for n < len(original) && keyLess(s, original[n].Key, row.Key) {
			n++
		}
		if n == len(original) || !reflect.DeepEqual(original[n], row) {
			return false
		}
		n++
	}
	return true
}

func restorationStatements(m material) ([5]string, error) {
	var out [5]string
	if !originalResourceBytesValid() {
		return out, errors.New("compiled_original_table_resources_changed")
	}
	for i, s := range scope {
		collation := ""
		for _, c := range originalCollations {
			if m.Tables[i].DDL == originalDDL(i, c) {
				collation = c
			}
		}
		if collation == "" {
			return out, errors.New("original_table_collation_not_qualified")
		}
		source := original078
		if i >= 3 {
			source = original075
		}
		for _, statement := range strings.Split(string(source), ";") {
			statement = strings.TrimSpace(statement)
			if strings.HasPrefix(statement, "CREATE TABLE "+s.name+" (") {
				if out[i] != "" || strings.Count(statement, "DEFAULT CHARSET=utf8mb4") != 1 {
					return [5]string{}, errors.New("compiled_original_table_statement_rejected")
				}
				out[i] = strings.Replace(statement, "DEFAULT CHARSET=utf8mb4", "DEFAULT CHARSET=utf8mb4 COLLATE="+collation, 1)
			}
		}
		if out[i] == "" {
			return [5]string{}, errors.New("compiled_original_table_statement_missing")
		}
	}
	return out, nil
}

const insertBatchRows = 100
const insertBatchBytes = 1 << 20

func insertBatchSQL(i, n int) (string, error) {
	if i < 0 || i >= len(scope) || n < 1 || n > insertBatchRows {
		return "", errors.New("original_insert_batch_scope_rejected")
	}
	var cols []string
	for _, c := range scope[i].columns {
		cols = append(cols, "`"+c.Name+"`")
	}
	row := "(" + strings.TrimSuffix(strings.Repeat("?,", len(cols)), ",") + ")"
	return "INSERT INTO `" + scope[i].name + "` (" + strings.Join(cols, ",") + ") VALUES " + strings.TrimSuffix(strings.Repeat(row+",", n), ","), nil
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
	var frozen approval
	if e != nil || strictDecode(raw, &frozen) != nil || frozen != a {
		return errors.New("frozen_approval_changed_before_claim")
	}
	return validateApproval(a, m, hash, tool, time.Now())
}

func (d database) insertOriginalBatch(ctx context.Context, m material, a approval, hash, tool string, i int, rows []rowProof) (resultErr error) {
	query, e := insertBatchSQL(i, len(rows))
	if e != nil {
		return e
	}
	var args []any
	used := 0
	for _, row := range rows {
		if validateRow(scope[i], row) != nil {
			return errors.New("original_insert_row_rejected")
		}
		for _, v := range row.Values {
			used += len(v.Bytes)
			if v.SQLNull {
				args = append(args, nil)
			} else {
				args = append(args, v.Bytes)
			}
		}
	}
	if used > insertBatchBytes {
		return errors.New("original_insert_batch_bytes_rejected")
	}
	if e := validateMutableBoundary(m, a, hash, tool); e != nil {
		return e
	}
	if e := d.requireCapturedRuntimeBoundary(ctx, m); e != nil {
		return e
	}
	tx, e := d.conn.BeginTx(ctx, nil)
	if e != nil {
		return errors.New("original_insert_begin_failed")
	}
	done := false
	defer func() {
		if !done {
			recordRollback(&resultErr, tx)
		}
	}()
	if e := validateMutableBoundary(m, a, hash, tool); e != nil {
		return e
	}
	if e := d.requireCapturedRuntimeBoundary(ctx, m); e != nil {
		return e
	}
	q, cancel := context.WithTimeout(ctx, queryLimit)
	r, e := tx.ExecContext(q, query, args...)
	cancel()
	if e != nil {
		return errors.New("original_insert_failed_or_unknown")
	}
	n, e := r.RowsAffected()
	if e != nil || n != int64(len(rows)) {
		return errors.New("original_insert_count_rejected")
	}
	// Includes time spent waiting for row/index locks; the original absolute
	// deadline and marker are checked again immediately before COMMIT.
	if e := validateMutableBoundary(m, a, hash, tool); e != nil {
		return e
	}
	if e := d.requireCapturedRuntimeBoundary(ctx, m); e != nil {
		return e
	}
	if tx.Commit() != nil {
		return errors.New("original_insert_commit_outcome_unknown")
	}
	done = true
	return nil
}

func (d database) fillOriginalRows(ctx context.Context, m material, current recoveryState, a approval, hash, tool string, i int) error {
	var pending []rowProof
	n := 0
	now := current.Material.Tables[i].Rows
	for _, row := range m.Tables[i].Rows {
		if n < len(now) && sameStrings(row.Key, now[n].Key) {
			n++
			continue
		}
		pending = append(pending, row)
	}
	for len(pending) > 0 {
		n, used := 0, 0
		for n < len(pending) && n < insertBatchRows {
			size := 0
			for _, v := range pending[n].Values {
				size += len(v.Bytes)
			}
			if size > insertBatchBytes {
				return errors.New("original_insert_batch_bytes_rejected")
			}
			if used+size > insertBatchBytes {
				break
			}
			used += size
			n++
		}
		if e := d.insertOriginalBatch(ctx, m, a, hash, tool, i, pending[:n]); e != nil {
			return e
		}
		pending = pending[n:]
	}
	return nil
}

func (d database) restore(ctx context.Context, m material, a approval, materialPath, materialHash, approvalPath, approvalHash, evidencePath, evidenceHash, tool string, consumed *bool) error {
	if e := d.requireCapturedRuntimeBoundary(ctx, m); e != nil {
		return e
	}
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
	statements, e := restorationStatements(m)
	if e != nil {
		return e
	}
	if e := rereadApprovedInputs(m, a, materialPath, materialHash, approvalPath, approvalHash, tool); e != nil {
		return e
	}
	if e := readCaptureEvidence(m, materialHash, tool, evidencePath, evidenceHash); e != nil {
		return e
	}
	if ctx.Err() != nil {
		return errors.New("total_budget_exceeded")
	}
	if e := d.requireCapturedRuntimeBoundary(ctx, m); e != nil {
		return e
	}
	initial, missing := current.Material.Ledger[0], current.Missing
	if prior != nil {
		initial, missing = prior.InitialLedger, prior.InitialMissing
	}
	if e := consumeApproval(m, a, approvalHash, initial, missing); e != nil {
		return e
	}
	*consumed = true
	boundary, e := readRecoveryBoundary(m, a, materialHash)
	if e != nil || boundary == nil {
		return errors.New("first_recovery_boundary_unproven")
	}
	for i := len(scope) - 1; i >= 0; i-- {
		if !boundary.InitialMissing[i] {
			continue
		}
		if current.Missing[i] {
			if e := validateMutableBoundary(m, a, materialHash, tool); e != nil {
				return e
			}
			if e := d.requireCapturedRuntimeBoundary(ctx, m); e != nil {
				return e
			}
			if _, e := d.conn.ExecContext(ctx, statements[i]); e != nil {
				return errors.New("original_table_create_failed_or_unknown")
			}
			current, e = d.recoverySnapshot(ctx, tool)
			if e != nil {
				return e
			}
			if e := validateRecoveryState(m, current, boundary); e != nil {
				return e
			}
		}
		if e := d.fillOriginalRows(ctx, m, current, a, materialHash, tool, i); e != nil {
			return e
		}
		current, e = d.recoverySnapshot(ctx, tool)
		if e != nil {
			return e
		}
		if e := validateRecoveryState(m, current, boundary); e != nil {
			return e
		}
	}
	if !reflect.DeepEqual(current.Material.Tables, m.Tables) || !sameRetained(current.Material.Retained, m.Retained) {
		return errors.New("full_original_tables_or_modern_facts_not_restored")
	}
	if e := d.replayLedgerLast(ctx, m, current.Material.Ledger[0], a, materialHash, tool); e != nil {
		return e
	}
	final, e := d.snapshot(ctx, tool, 102, true)
	if e != nil || verifyRestored(m, final) != nil {
		return errors.New("final_original_tables_or_ledger_unproven")
	}
	return d.requireCapturedRuntimeBoundary(ctx, m)
}

func (d database) replayLedgerLast(ctx context.Context, m material, expected ledgerRow, a approval, hash, tool string) (resultErr error) {
	if e := d.requireCapturedRuntimeBoundary(ctx, m); e != nil {
		return e
	}
	before, e := d.observe(ctx, d.conn)
	if e != nil || before.Ledger != expected || !bytes.Equal(before.Mongo, m.MongoHead) {
		return errors.New("binding_or_ledger_changed_before_last_transaction")
	}
	if e := validateMutableBoundary(m, a, hash, tool); e != nil {
		return e
	}
	tx, e := d.conn.BeginTx(ctx, nil)
	if e != nil {
		return errors.New("ledger_transaction_begin_failed")
	}
	done := false
	defer func() {
		if !done {
			recordRollback(&resultErr, tx)
		}
	}()
	rows, e := readLedger(ctx, tx)
	if e != nil || rows[0] != expected {
		return errors.New("sql103_ledger_changed_before_replay")
	}
	if e := validateMutableBoundary(m, a, hash, tool); e != nil {
		return e
	}
	if e := d.requireCapturedRuntimeBoundary(ctx, m); e != nil {
		return e
	}
	r, e := tx.ExecContext(ctx, "DELETE FROM schema_migrations")
	if e != nil {
		return errors.New("ledger_transaction_delete_failed")
	}
	if n, e := r.RowsAffected(); e != nil || n != 1 {
		return errors.New("ledger_transaction_delete_count_rejected")
	}
	for _, row := range m.Ledger {
		if e := validateMutableBoundary(m, a, hash, tool); e != nil {
			return e
		}
		if e := d.requireCapturedRuntimeBoundary(ctx, m); e != nil {
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
	if e := d.requireCapturedRuntimeBoundary(ctx, m); e != nil {
		return e
	}
	if tx.Commit() != nil {
		return errors.New("ledger_commit_outcome_unknown")
	}
	done = true
	return nil
}

func verifyRestored(original, current material) error {
	if current.Format != original.Format || current.Kind != original.Kind || current.ToolSource != original.ToolSource || current.DSource != original.DSource || current.DResources != original.DResources || current.DResourceFiles != original.DResourceFiles || current.Binding != original.Binding || current.RuntimeBoundary != original.RuntimeBoundary || current.SessionZone != original.SessionZone || current.SessionCharset != original.SessionCharset || current.SessionSQLMode != original.SessionSQLMode || current.LedgerDDL != original.LedgerDDL || !reflect.DeepEqual(current.Ledger, original.Ledger) || !bytes.Equal(current.MongoHead, original.MongoHead) || !reflect.DeepEqual(current.Tables, original.Tables) || !sameRetained(current.Retained, original.Retained) {
		return errors.New("original_restoration_proof_rejected")
	}
	return nil
}

func restoreActionContext(ctx context.Context, a approval) (context.Context, context.CancelFunc) {
	deadline, _ := time.Parse(time.RFC3339Nano, a.Deadline)
	expires, _ := time.Parse(time.RFC3339Nano, a.Expires)
	if expires.Before(deadline) {
		deadline = expires
	}
	return context.WithDeadline(ctx, deadline)
}
