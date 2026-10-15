package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/go-sql-driver/mysql"
)

// This temporary D-only control table belongs to the bound database, never to
// a capture directory. It is retired separately after all recovery obligations
// and old tool approvals have ended. Capture does not initialize it.
const runtimeBoundaryTable = "m4_d_runtime_boundary"
const maxRuntimeBoundaryBytes = 8192
const runtimeBoundaryDDL = "CREATE TABLE `m4_d_runtime_boundary` (\n" +
	"  `singleton` tinyint unsigned NOT NULL,\n" +
	"  `generation` bigint unsigned NOT NULL,\n" +
	"  `state_sha256` binary(32) NOT NULL,\n" +
	"  `state_json` varbinary(8192) NOT NULL,\n" +
	"  PRIMARY KEY (`singleton`),\n" +
	"  CONSTRAINT `m4_d_runtime_boundary_singleton` CHECK ((`singleton` = 1))\n" +
	") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin"
const runtimeBoundaryShow = "SHOW CREATE TABLE `m4_d_runtime_boundary`"
const runtimeBoundarySelect = "SELECT singleton,generation,state_sha256,state_json FROM `m4_d_runtime_boundary` LIMIT 2"
const runtimeBoundaryCreate = runtimeBoundaryDDL + " AS SELECT ? AS singleton,? AS generation,? AS state_sha256,? AS state_json"
const runtimeBoundaryUpdate = "UPDATE `m4_d_runtime_boundary` SET generation=?,state_sha256=?,state_json=? WHERE singleton=1 AND generation=? AND state_sha256=? AND state_json=?"

type runtimeBoundaryState struct {
	Generation  uint64 `json:"generation"`
	Fingerprint string `json:"state_sha256"`
}

func (s *runtimeBoundaryState) UnmarshalJSON(raw []byte) error {
	var members map[string]json.RawMessage
	var value struct {
		Generation  *uint64 `json:"generation"`
		Fingerprint *string `json:"state_sha256"`
	}
	if strictDecode(raw, &value) != nil || json.Unmarshal(raw, &members) != nil || len(members) != 2 || members["generation"] == nil || members["state_sha256"] == nil || value.Generation == nil || value.Fingerprint == nil {
		return errors.New("runtime_database_boundary_state_rejected")
	}
	*s = runtimeBoundaryState{Generation: *value.Generation, Fingerprint: *value.Fingerprint}
	return nil
}

type runtimeBoundaryRecord struct {
	Format         int    `json:"format_version"`
	Kind           string `json:"kind"`
	SQLIdentity    string `json:"mysql_identity_sha256"`
	SQLName        string `json:"mysql_database"`
	Generation     uint64 `json:"generation"`
	Previous       string `json:"previous_state_sha256"`
	ToolSource     string `json:"compiled_tool_source"`
	DSource        string `json:"d_execution_source"`
	DResources     string `json:"d_execution_resources_sha256"`
	DResourceFiles int    `json:"d_execution_resource_files"`
	MaterialHash   string `json:"original_material_sha256"`
	IntentHash     string `json:"runtime_intent_sha256"`
	Mode           string `json:"mode"`
	SQLVersion     uint64 `json:"sql_version_before_runtime"`
	Created        string `json:"committed_intent_created_at"`
}

type runtimeBoundaryObservation struct {
	State  runtimeBoundaryState
	Record *runtimeBoundaryRecord
	Raw    []byte
}

func initialRuntimeBoundary(b binding) runtimeBoundaryState {
	raw, _ := json.Marshal([]string{"m4_d_runtime_boundary_initial_v1", b.SQLIdentity, b.SQLName})
	return runtimeBoundaryState{Fingerprint: fingerprint(raw)}
}

func validRuntimeBoundaryState(s runtimeBoundaryState, b binding) bool {
	if !validBinding(b) || !nonzeroHash(s.Fingerprint) {
		return false
	}
	initial := initialRuntimeBoundary(b)
	if s.Generation == 0 {
		return s == initial
	}
	return s.Fingerprint != initial.Fingerprint
}

func validateRuntimeBoundaryRecord(v runtimeBoundaryRecord, raw []byte, b binding) error {
	if len(raw) == 0 || len(raw) > maxRuntimeBoundaryBytes || !validBinding(b) || v.Format != 1 || v.Kind != "m4_d_runtime_boundary" || v.SQLIdentity != b.SQLIdentity || v.SQLName != b.SQLName || v.Generation == 0 || !nonzeroHash(v.Previous) || !sourceRE.MatchString(v.ToolSource) || v.DSource != dSource || v.DResources != dResources || v.DResourceFiles != dResourceFiles || !nonzeroHash(v.MaterialHash) || !nonzeroHash(v.IntentHash) {
		return errors.New("runtime_database_boundary_row_rejected")
	}
	if v.Generation == 1 && v.Previous != initialRuntimeBoundary(b).Fingerprint {
		return errors.New("runtime_database_boundary_row_rejected")
	}
	if (v.Mode == "target103" && v.SQLVersion != 103) || ((v.Mode == "cancel102" || v.Mode == "restored102") && v.SQLVersion != 102) || (v.Mode != "target103" && v.Mode != "cancel102" && v.Mode != "restored102") {
		return errors.New("runtime_database_boundary_row_rejected")
	}
	if _, e := time.Parse(time.RFC3339Nano, v.Created); e != nil {
		return errors.New("runtime_database_boundary_row_rejected")
	}
	canonical, e := json.Marshal(v)
	if e != nil || !bytes.Equal(canonical, raw) {
		return errors.New("runtime_database_boundary_row_rejected")
	}
	return nil
}

// SHOW does not evaluate an unknown view. Only the server's exact missing-table
// response plus an empty catalog may mean initial. Permission or release errors
// never initialize or recreate this object.
func readRuntimeBoundary(ctx context.Context, r sqlReader, b binding) (out runtimeBoundaryObservation, resultErr error) {
	if !validBinding(b) || ctx.Err() != nil {
		return out, errors.New("runtime_database_boundary_unknown")
	}
	q, cancel := context.WithTimeout(ctx, queryLimit)
	rows, e := r.QueryContext(q, runtimeBoundaryShow)
	if e != nil {
		cancel()
		var server *mysql.MySQLError
		if !errors.As(e, &server) || server.Number != 1146 {
			return out, errors.New("runtime_database_boundary_unknown")
		}
		q, cancel = context.WithTimeout(ctx, queryLimit)
		defer cancel()
		var count uint64
		if r.QueryRowContext(q, "SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='m4_d_runtime_boundary'").Scan(&count) != nil || count != 0 || ctx.Err() != nil {
			return out, errors.New("runtime_database_boundary_absence_unproven")
		}
		out.State = initialRuntimeBoundary(b)
		return out, nil
	}
	var name, ddl string
	count := 0
	for rows.Next() {
		if rows.Scan(&name, &ddl) != nil || name != runtimeBoundaryTable || ddl != runtimeBoundaryDDL {
			_ = rows.Close()
			cancel()
			return out, errors.New("runtime_database_boundary_schema_rejected")
		}
		count++
		if count > 1 {
			break
		}
	}
	re, ce := rows.Err(), rows.Close()
	cancel()
	if re != nil || ce != nil || count != 1 {
		return out, errors.New("runtime_database_boundary_schema_or_release_rejected")
	}
	q, cancel = context.WithTimeout(ctx, queryLimit)
	defer cancel()
	rows, e = r.QueryContext(q, runtimeBoundarySelect)
	if e != nil {
		return out, errors.New("runtime_database_boundary_unknown")
	}
	count = 0
	for rows.Next() {
		var singleton uint64
		var generation uint64
		var hash, raw []byte
		var record runtimeBoundaryRecord
		if rows.Scan(&singleton, &generation, &hash, &raw) != nil || singleton != 1 || generation == 0 || len(hash) != 32 || len(raw) == 0 || len(raw) > maxRuntimeBoundaryBytes || strictDecode(raw, &record) != nil || record.Generation != generation || validateRuntimeBoundaryRecord(record, raw, b) != nil || hex.EncodeToString(hash) != fingerprint(raw) {
			_ = rows.Close()
			return out, errors.New("runtime_database_boundary_row_rejected")
		}
		out = runtimeBoundaryObservation{State: runtimeBoundaryState{Generation: generation, Fingerprint: fingerprint(raw)}, Record: &record, Raw: raw}
		count++
		if count > 1 {
			break
		}
	}
	re, ce = rows.Err(), rows.Close()
	if re != nil || ce != nil || count != 1 || ctx.Err() != nil {
		return runtimeBoundaryObservation{}, errors.New("runtime_database_boundary_row_or_release_rejected")
	}
	return out, nil
}

func requireRuntimeBoundary(ctx context.Context, r sqlReader, b binding, expected runtimeBoundaryState) error {
	if !validRuntimeBoundaryState(expected, b) {
		return errors.New("runtime_database_boundary_capture_rejected")
	}
	current, e := readRuntimeBoundary(ctx, r, b)
	if e != nil {
		return e
	}
	if current.State != expected {
		return errors.New("runtime_database_boundary_changed")
	}
	return nil
}

func nextRuntimeBoundary(m material, v runtimeIntent, a arguments, tool string, now time.Time) (runtimeBoundaryRecord, []byte, error) {
	if !validRuntimeBoundaryState(m.RuntimeBoundary, m.Binding) || m.RuntimeBoundary.Generation == math.MaxUint64 || !sourceRE.MatchString(tool) || !nonzeroHash(a.materialHash) || !nonzeroHash(a.runtimeIntentHash) {
		return runtimeBoundaryRecord{}, nil, errors.New("runtime_database_boundary_generation_rejected")
	}
	version := uint64(102)
	if v.Mode == "target103" {
		version = 103
	}
	record := runtimeBoundaryRecord{Format: 1, Kind: "m4_d_runtime_boundary", SQLIdentity: m.Binding.SQLIdentity, SQLName: m.Binding.SQLName, Generation: m.RuntimeBoundary.Generation + 1, Previous: m.RuntimeBoundary.Fingerprint, ToolSource: tool, DSource: dSource, DResources: dResources, DResourceFiles: dResourceFiles, MaterialHash: a.materialHash, IntentHash: a.runtimeIntentHash, Mode: v.Mode, SQLVersion: version, Created: now.UTC().Format(time.RFC3339Nano)}
	raw, e := json.Marshal(record)
	if e != nil || validateRuntimeBoundaryRecord(record, raw, m.Binding) != nil {
		return runtimeBoundaryRecord{}, nil, errors.New("runtime_database_boundary_generation_rejected")
	}
	return record, raw, nil
}

// Initial creation is one atomic MySQL8.0.36 InnoDB CTAS, including the only row.
// Later advances use a transactional compare-and-set on generation, hash and
// original bytes. Neither path resets, drops, repairs or recreates an old state.
func (d database) advanceRuntimeBoundary(ctx context.Context, m material, v runtimeIntent, a arguments, tool string, r *receipt) error {
	if d.binding != m.Binding || m.Format != 3 {
		return errors.New("runtime_database_boundary_capture_rejected")
	}
	if e := d.gate.require(ctx); e != nil {
		return e
	}
	current, e := readRuntimeBoundary(ctx, d.conn, m.Binding)
	if e != nil {
		return e
	}
	if current.State != m.RuntimeBoundary || !validRuntimeBoundaryState(m.RuntimeBoundary, m.Binding) {
		return errors.New("runtime_database_boundary_changed")
	}
	record, raw, e := nextRuntimeBoundary(m, v, a, tool, time.Now())
	if e != nil || ctx.Err() != nil {
		return errors.New("runtime_database_boundary_generation_rejected")
	}
	hash, _ := hex.DecodeString(fingerprint(raw))
	if current.State.Generation == 0 {
		q, cancel := context.WithTimeout(ctx, queryLimit)
		r.DatabaseBoundaryAdvanceUnknown = true
		result, e := d.conn.ExecContext(q, runtimeBoundaryCreate, 1, record.Generation, hash, raw)
		cancel()
		if e != nil {
			return errors.New("runtime_database_boundary_create_outcome_unknown")
		}
		if count, e := result.RowsAffected(); e != nil || count != 1 {
			return errors.New("runtime_database_boundary_create_outcome_unknown")
		}
	} else if e := d.updateRuntimeBoundary(ctx, current, record, raw, hash, m.Binding, r); e != nil {
		return e
	}
	got, e := readRuntimeBoundary(ctx, d.conn, m.Binding)
	if e != nil || got.Record == nil || *got.Record != record || !bytes.Equal(got.Raw, raw) || ctx.Err() != nil {
		return errors.New("runtime_database_boundary_advance_readback_unproven")
	}
	r.DatabaseBoundary, r.DatabaseBoundaryRecord = &got.State, got.Record
	r.DatabaseBoundaryAdvanced, r.DatabaseBoundaryAdvanceUnknown = true, false
	return nil
}

func (d database) updateRuntimeBoundary(ctx context.Context, current runtimeBoundaryObservation, next runtimeBoundaryRecord, raw, hash []byte, b binding, r *receipt) (resultErr error) {
	tx, e := d.conn.BeginTx(ctx, nil)
	if e != nil {
		return errors.New("runtime_database_boundary_begin_failed")
	}
	done := false
	defer func() {
		if !done {
			recordRollback(&resultErr, tx)
		}
	}()
	if e := requireRuntimeBoundary(ctx, tx, b, current.State); e != nil {
		return e
	}
	oldHash, _ := hex.DecodeString(current.State.Fingerprint)
	q, cancel := context.WithTimeout(ctx, queryLimit)
	r.DatabaseBoundaryAdvanceUnknown = true
	result, e := tx.ExecContext(q, runtimeBoundaryUpdate, next.Generation, hash, raw, current.State.Generation, oldHash, current.Raw)
	cancel()
	if e != nil {
		return errors.New("runtime_database_boundary_update_failed_or_unknown")
	}
	if count, e := result.RowsAffected(); e != nil || count != 1 {
		return errors.New("runtime_database_boundary_compare_and_set_rejected")
	}
	if e := requireRuntimeBoundary(ctx, tx, b, runtimeBoundaryState{Generation: next.Generation, Fingerprint: fingerprint(raw)}); e != nil {
		return e
	}
	if ctx.Err() != nil {
		return errors.New("runtime_database_boundary_deadline_exhausted")
	}
	if tx.Commit() != nil {
		return errors.New("runtime_database_boundary_commit_outcome_unknown")
	}
	done = true
	return nil
}

// This task deliberately does not auto-complete a previously committed intent
// after a local marker/output failure. Fresh read-only inspection exposes the
// committed material+intent; a separate completion approval remains required.
func (d database) requireCapturedRuntimeBoundary(ctx context.Context, m material) error {
	if d.binding != m.Binding || m.Format != 3 {
		return errors.New("runtime_database_boundary_capture_rejected")
	}
	if e := d.gate.require(ctx); e != nil {
		return e
	}
	return requireRuntimeBoundary(ctx, d.conn, m.Binding, m.RuntimeBoundary)
}
