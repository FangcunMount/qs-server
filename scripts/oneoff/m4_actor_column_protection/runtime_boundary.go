package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/go-sql-driver/mysql"
)

// This temporary C-only control table belongs to the bound database, never to
// a capture directory. It is retired separately after all recovery obligations
// and old tool approvals have ended. Capture does not initialize it.
const runtimeBoundaryTable = "m4_c_runtime_boundary"
const maxRuntimeBoundaryBytes = 8192
const runtimeBoundaryDDL = "CREATE TABLE `m4_c_runtime_boundary` (\n" +
	"  `singleton` tinyint unsigned NOT NULL,\n" +
	"  `generation` bigint unsigned NOT NULL,\n" +
	"  `state_sha256` binary(32) NOT NULL,\n" +
	"  `state_json` varbinary(8192) NOT NULL,\n" +
	"  PRIMARY KEY (`singleton`),\n" +
	"  CONSTRAINT `m4_c_runtime_boundary_singleton` CHECK ((`singleton` = 1))\n" +
	") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin"
const runtimeBoundaryShow = "SHOW CREATE TABLE `m4_c_runtime_boundary`"
const runtimeBoundarySelect = "SELECT singleton,generation,state_sha256,state_json FROM `m4_c_runtime_boundary` LIMIT 2"
const runtimeBoundaryCreate = runtimeBoundaryDDL + " AS SELECT ? AS singleton,? AS generation,? AS state_sha256,? AS state_json"
const runtimeBoundaryUpdate = "UPDATE `m4_c_runtime_boundary` SET generation=?,state_sha256=?,state_json=? WHERE singleton=1 AND generation=? AND state_sha256=? AND state_json=?"

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
	CSource        string `json:"c_execution_source"`
	CResources     string `json:"c_execution_resources_sha256"`
	CResourceFiles int    `json:"c_execution_resource_files"`
	MaterialHash   string `json:"original_material_sha256"`
	IntentHash     string `json:"runtime_intent_sha256"`
	Mode           string `json:"mode"`
	SQLVersion     uint64 `json:"sql_version_before_runtime"`
	Created        string `json:"committed_intent_created_at"`
	MarkerRaw      []byte `json:"original_marker_bytes"`
	MarkerHash     string `json:"original_marker_sha256"`
}

type runtimeBoundaryObservation struct {
	State  runtimeBoundaryState
	Record *runtimeBoundaryRecord
	Raw    []byte
}

func initialRuntimeBoundary(b binding) runtimeBoundaryState {
	raw, _ := json.Marshal([]string{"m4_c_runtime_boundary_initial_v1", b.SQLIdentity, b.SQLName})
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
	if len(raw) == 0 || len(raw) > maxRuntimeBoundaryBytes || !validBinding(b) || v.Format != 1 || v.Kind != "m4_c_runtime_boundary" || v.SQLIdentity != b.SQLIdentity || v.SQLName != b.SQLName || v.Generation == 0 || !nonzeroHash(v.Previous) || !sourceRE.MatchString(v.ToolSource) || v.CSource != cSource || v.CResources != cResources || v.CResourceFiles != cResourceFiles || !nonzeroHash(v.MaterialHash) || !nonzeroHash(v.IntentHash) {
		return errors.New("runtime_database_boundary_row_rejected")
	}
	if v.Generation == 1 && v.Previous != initialRuntimeBoundary(b).Fingerprint {
		return errors.New("runtime_database_boundary_row_rejected")
	}
	if (v.Mode == "target102" && v.SQLVersion != 102) || ((v.Mode == "cancel101" || v.Mode == "restored101") && v.SQLVersion != 101) || (v.Mode != "target102" && v.Mode != "cancel101" && v.Mode != "restored101") {
		return errors.New("runtime_database_boundary_row_rejected")
	}
	if _, e := time.Parse(time.RFC3339Nano, v.Created); e != nil || validateFrozenRuntimeMarker(v) != nil {
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
		if r.QueryRowContext(q, "SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='m4_c_runtime_boundary'").Scan(&count) != nil || count != 0 || ctx.Err() != nil {
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

// A marker is encoded before any database advance, from the independently
// frozen intent. It intentionally does not include the containing record hash.
type runtimeMarker struct {
	Format         int    `json:"format_version"`
	Kind           string `json:"kind"`
	ToolSource     string `json:"compiled_tool_source"`
	CSource        string `json:"c_execution_source"`
	CResources     string `json:"c_execution_resources_sha256"`
	CResourceFiles int    `json:"c_execution_resource_files"`
	SQLIdentity    string `json:"mysql_identity_sha256"`
	SQLName        string `json:"mysql_database"`
	MaterialHash   string `json:"material_sha256"`
	IntentHash     string `json:"runtime_intent_sha256"`
	Generation     uint64 `json:"generation"`
	Previous       string `json:"previous_state_sha256"`
	Mode           string `json:"mode"`
	SQLVersion     uint64 `json:"sql_version_before_runtime"`
	Created        string `json:"intent_decision_at"`
}

func markerForRecord(v runtimeBoundaryRecord) runtimeMarker {
	return runtimeMarker{1, "m4_c_first_runtime_boundary", v.ToolSource, v.CSource, v.CResources, v.CResourceFiles, v.SQLIdentity, v.SQLName, v.MaterialHash, v.IntentHash, v.Generation, v.Previous, v.Mode, v.SQLVersion, v.Created}
}

func validateFrozenRuntimeMarker(v runtimeBoundaryRecord) error {
	if len(v.MarkerRaw) == 0 || len(v.MarkerRaw) > 4096 || !nonzeroHash(v.MarkerHash) || fingerprint(v.MarkerRaw) != v.MarkerHash {
		return errors.New("runtime_database_marker_bytes_rejected")
	}
	expected, e := json.Marshal(markerForRecord(v))
	if e != nil || !bytes.Equal(expected, v.MarkerRaw) {
		return errors.New("runtime_database_marker_bytes_rejected")
	}
	return nil
}
