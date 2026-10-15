package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

// This one-time reader has eleven literal business statements and four live
// identity/ledger reads. It never accepts SQL, object names or a target head.
type inventoryMeasurements struct {
	MySQLVersion       inventoryServerVersion       `json:"mysql_version"`
	MigrationVersion   uint64                       `json:"migration_version"`
	MigrationDirty     bool                         `json:"migration_dirty"`
	NineInnoDBVerified bool                         `json:"nine_innodb_objects_verified"`
	ConsistentSnapshot bool                         `json:"consistent_snapshot"`
	Counts             map[string]map[string]uint64 `json:"counts"`
}

type inventoryServerVersion struct {
	Major uint64 `json:"major"`
	Minor uint64 `json:"minor"`
	Patch uint64 `json:"patch"`
}

// Only the three numeric MySQL version components leave RAM. A suffix is
// never emitted, and incompatible or unrecognised forms are not capabilities.
func inventoryVersion(raw string) (inventoryServerVersion, bool) {
	var result inventoryServerVersion
	if strings.Contains(strings.ToLower(raw), "mariadb") {
		return result, false
	}
	prefix, suffix, hasSuffix := strings.Cut(raw, "-")
	if hasSuffix && suffix == "" {
		return result, false
	}
	parts := strings.Split(prefix, ".")
	if len(parts) != 3 {
		return result, false
	}
	values := []*uint64{&result.Major, &result.Minor, &result.Patch}
	for i, part := range parts {
		n, err := strconv.ParseUint(part, 10, 64)
		if err != nil || n > 999 || strconv.FormatUint(n, 10) != part {
			return inventoryServerVersion{}, false
		}
		*values[i] = n
	}
	return result, result.Major >= 8
}

type inventoryReceipt struct {
	FormatVersion       int                    `json:"format_version"`
	SourceSHA           string                 `json:"source_sha,omitempty"`
	RunID               string                 `json:"run_id,omitempty"`
	ExpectedTargetHash  string                 `json:"expected_target_hash,omitempty"`
	SourceTargetHash    string                 `json:"source_target_hash,omitempty"`
	InventoryOnly       bool                   `json:"retirement_inventory_only"`
	FreezeVerified      bool                   `json:"freeze_verified"`
	Complete            bool                   `json:"complete"`
	ErrorCategory       string                 `json:"error_category"`
	FailureStage        string                 `json:"failure_stage"`
	QueryCount          uint64                 `json:"query_count"`
	ControlStatements   uint64                 `json:"control_statement_count"`
	QueryMySQLNumber    *uint16                `json:"query_mysql_error_number,omitempty"`
	TransactionReleased *bool                  `json:"transaction_released"`
	ConnectionReleased  *bool                  `json:"connection_released"`
	DatabaseReleased    *bool                  `json:"database_released"`
	PoolClosedVerified  *bool                  `json:"pool_closed_verified"`
	Measurements        *inventoryMeasurements `json:"measurements,omitempty"`
	deadline            time.Time
}

func baseInventoryReceipt(b binding) inventoryReceipt {
	old := baseReceipt(b)
	return inventoryReceipt{FormatVersion: 1, SourceSHA: old.SourceSHA, RunID: old.RunID,
		ExpectedTargetHash: old.ExpectedTargetHash, InventoryOnly: true,
		ErrorCategory: "input_invalid", FailureStage: "input"}
}

func invalidateInventory(r inventoryReceipt, category, stage string) inventoryReceipt {
	r.Complete, r.Measurements = false, nil
	r.ErrorCategory, r.FailureStage = category, stage
	return r
}

func invalidateInventoryQuery(r inventoryReceipt, err error, stage string) inventoryReceipt {
	var failure *readinessQueryFailure
	if errors.As(err, &failure) && failure != nil {
		r.QueryMySQLNumber = failure.number
	}
	return invalidateInventory(r, err.Error(), stage)
}

var inventoryTables = []string{"ai_bridge_request_assessments", "ai_bridge_requests", "clinician_operator_binding_archive", "operator_retirement_manifests", "operators", "testee", "testee_store_migration_items", "testee_store_migration_manifests", "testee_store_migration_reversions"}

func inventoryObjectShape(rows [][]sql.NullString) bool {
	if len(rows) != len(inventoryTables) {
		return false
	}
	for i, table := range inventoryTables {
		for j, want := range []string{table, "BASE TABLE", "InnoDB"} {
			if !rows[i][j].Valid || rows[i][j].String != want {
				return false
			}
		}
	}
	return true
}

func inventoryScalarCounts(rows [][]sql.NullString, fields []string) (map[string]uint64, bool) {
	if len(rows) != 1 || len(rows[0]) != len(fields) {
		return nil, false
	}
	result := map[string]uint64{}
	for i, field := range fields {
		cell := rows[0][i]
		value, err := strconv.ParseUint(cell.String, 10, 64)
		if !cell.Valid || err != nil || strconv.FormatUint(value, 10) != cell.String {
			return nil, false
		}
		result[field] = value
	}
	return result, true
}

func inventoryGroupedCounts(rows [][]sql.NullString, labels []string, requireAll bool) (map[string]uint64, bool) {
	result, seen := map[string]uint64{}, map[string]bool{}
	for _, label := range labels {
		result[label] = 0
	}
	for _, row := range rows {
		if len(row) != 2 || !row[0].Valid || seen[row[0].String] {
			return nil, false
		}
		if _, known := result[row[0].String]; !known {
			return nil, false
		}
		values, ok := inventoryScalarCounts([][]sql.NullString{{row[1]}}, []string{"count"})
		if !ok {
			return nil, false
		}
		seen[row[0].String], result[row[0].String] = true, values["count"]
	}
	return result, !requireAll || len(seen) == len(labels)
}

func inventorySumEquals(values map[string]uint64, expected uint64, fields ...string) bool {
	var sum uint64
	for _, field := range fields {
		value, known := values[field]
		if !known || math.MaxUint64-sum < value {
			return false
		}
		sum += value
	}
	return sum == expected
}

// Nonzero differences and unknown groups are findings, never eligibility.
func inventoryConserves(c map[string]map[string]uint64) bool {
	tags, roles := c["testee_tags_partition"], c["operators_role_duplication"]
	if !inventorySumEquals(tags, tags["total"], "sql_null", "non_null") || !inventorySumEquals(tags, tags["non_null"], "json_null", "empty_array", "non_empty_array", "other") ||
		!inventorySumEquals(roles, roles["total"], "same_canonical_json_bytes", "different_canonical_json_bytes") {
		return false
	}
	for _, field := range []string{"roles_sql_null", "effective_sql_null", "both_arrays"} {
		if roles[field] > roles["total"] {
			return false
		}
	}
	tables := c["five_table_counts"]
	for _, item := range []struct{ family, table string }{{"testee_manifest_states", "testee_store_migration_manifests"}, {"testee_item_dispositions", "testee_store_migration_items"}, {"operator_manifest_stages", "operator_retirement_manifests"}} {
		var fields []string
		for label := range c[item.family] {
			fields = append(fields, label)
		}
		if !inventorySumEquals(c[item.family], tables[item.table], fields...) {
			return false
		}
	}
	gaps := c["historical_reference_gaps"]
	if gaps["items_without_manifest"] > tables["testee_store_migration_items"] || gaps["reversions_without_item"] > tables["testee_store_migration_reversions"] || gaps["archives_without_manifest"] > tables["clinician_operator_binding_archive"] {
		return false
	}
	ai, expected, actual := c["ai_backfill_scope"], c["ai_expected_relation_gaps"], c["ai_actual_relation_gaps"]
	if !inventorySumEquals(ai, ai["total"], "nonempty_assessment_arrays", "invalid_assessment_array_shape") {
		return false
	}
	for field, count := range ai {
		if field != "total" && count > ai["total"] {
			return false
		}
	}
	if ai["indexed_identity_mismatch"] > ai["total"]-ai["invalid_actor_field_shape"] || expected["invalid_expected_id_entries"] > expected["payload_relation_entries"] || expected["valid_expected_entries_without_relation"] > expected["payload_relation_entries"]-expected["invalid_expected_id_entries"] {
		return false
	}
	return actual["orphan_relations"] <= actual["actual_relation_rows"] && actual["zero_assessment_relations"] <= actual["actual_relation_rows"] && actual["relation_not_in_request_payload"] <= actual["actual_relation_rows"]-actual["orphan_relations"]
}

func probeInventory(ctx context.Context, db *sql.DB, cfg connection, b binding) (r inventoryReceipt) {
	r = baseInventoryReceipt(b)
	if !validBinding(b) {
		return r
	}
	ctx, cancel := context.WithTimeout(ctx, totalLimit)
	defer cancel()
	r.deadline, _ = ctx.Deadline()
	conn, err := db.Conn(ctx)
	if err != nil {
		return invalidateInventory(r, "connection_failed", "connect")
	}
	defer func() {
		ok := conn.Close() == nil
		r.ConnectionReleased = &ok
		if !ok {
			r = invalidateInventory(r, "connection_release_failed", "connection_release")
		}
	}()
	read := func(query, stage string, width int) ([][]sql.NullString, bool) {
		r.QueryCount++
		rows, e := readinessRows(ctx, conn, query, width)
		if e != nil {
			r = invalidateInventoryQuery(r, e, stage)
			return nil, false
		}
		return rows, true
	}
	readIdentity := func(stage string) (identity, bool) {
		rows, ok := read(inventoryIdentitySQL, stage, 5)
		if !ok {
			return identity{}, false
		}
		if len(rows) != 1 {
			r = invalidateInventory(r, "identity_read_failed", stage)
			return identity{}, false
		}
		for _, cell := range rows[0] {
			if !cell.Valid {
				r = invalidateInventory(r, "identity_read_failed", stage)
				return identity{}, false
			}
		}
		id, e := strconv.ParseUint(rows[0][0].String, 10, 64)
		v := identity{ID: id, UUID: rows[0][1].String, Database: rows[0][2].String, Version: rows[0][3].String, Roles: rows[0][4].String}
		if e != nil || id == 0 || !uuidRE.MatchString(v.UUID) || v.Database == "" || v.Version == "" || len(v.Version) > 128 || v.Roles == "" {
			r = invalidateInventory(r, "identity_read_failed", stage)
			return identity{}, false
		}
		return v, true
	}
	before, ok := readIdentity("identity_before")
	if !ok {
		return r
	}
	if before.Database != cfg.Database {
		return invalidateInventory(r, "target_identity_mismatch", "identity_before")
	}
	actualHash := hashText(cfg.Host, strconv.Itoa(cfg.Port), cfg.Database, before.UUID)
	if actualHash != b.Expected {
		return invalidateInventory(r, "target_hash_mismatch", "identity_before")
	}
	r.SourceTargetHash = actualHash
	version, recognized := inventoryVersion(before.Version)
	if !recognized {
		return invalidateInventory(r, "server_version_unrecognized", "identity_before")
	}
	ledger, ok := read(readinessMigrationSQL, "migration_before", 2)
	if !ok {
		return r
	}
	if !readinessClean100(ledger) {
		return invalidateInventory(r, "migration_pair_rejected", "migration_before")
	}
	control := func(query string) error {
		r.ControlStatements++
		deadline, done := context.WithTimeout(ctx, queryLimit)
		defer done()
		_, e := conn.ExecContext(deadline, query)
		if e != nil {
			return readinessQueryError(e)
		}
		return nil
	}
	if err = control(readinessIsolationSQL); err != nil {
		return invalidateInventoryQuery(r, err, "snapshot_begin")
	}
	active, notReleased := true, false
	r.TransactionReleased = &notReleased
	rollback := func() bool {
		if !active {
			return r.TransactionReleased != nil && *r.TransactionReleased
		}
		active = false // Never retry an unknown rollback outcome.
		e := control(readinessRollbackSQL)
		released := e == nil
		r.TransactionReleased = &released
		if e != nil {
			r = invalidateInventoryQuery(r, e, "snapshot_release")
			r = invalidateInventory(r, "transaction_release_failed", "snapshot_release")
		}
		return released
	}
	defer func() { rollback() }()
	if err = control(readinessBeginSQL); err != nil {
		return invalidateInventoryQuery(r, err, "snapshot_begin")
	}
	objects, ok := read(inventoryObjectsPresentSQL, "objects_present", 3)
	if !ok {
		return r
	}
	if !inventoryObjectShape(objects) {
		return invalidateInventory(r, "object_shape_rejected", "objects_present")
	}
	m := &inventoryMeasurements{MySQLVersion: version, MigrationVersion: 100, NineInnoDBVerified: true, ConsistentSnapshot: true, Counts: map[string]map[string]uint64{}}
	for _, step := range []struct {
		stage, query string
		fields       []string
		grouped      bool
		requireAll   bool
	}{
		{"testee_tags_partition", inventoryTesteeTagsPartitionSQL, []string{"total", "sql_null", "non_null", "json_null", "empty_array", "non_empty_array", "other"}, false, false},
		{"operators_role_duplication", inventoryOperatorsRoleDuplicationSQL, []string{"total", "roles_sql_null", "effective_sql_null", "both_arrays", "same_canonical_json_bytes", "different_canonical_json_bytes"}, false, false},
		{"five_table_counts", inventoryFiveTableCountsSQL, []string{"testee_store_migration_manifests", "testee_store_migration_items", "testee_store_migration_reversions", "operator_retirement_manifests", "clinician_operator_binding_archive"}, true, true},
		{"testee_manifest_states", inventoryTesteeManifestStatesSQL, []string{"applied", "rolled_back", "unknown"}, true, false},
		{"testee_item_dispositions", inventoryTesteeItemDispositionsSQL, []string{"candidate", "deferred", "already_assigned", "unknown"}, true, false},
		{"operator_manifest_stages", inventoryOperatorManifestStagesSQL, []string{"pending", "executing", "applied_unchanged", "applied_drifted", "invalid", "unknown"}, true, false},
		{"historical_reference_gaps", inventoryHistoricalReferenceGapsSQL, []string{"items_without_manifest", "reversions_without_item", "archives_without_manifest"}, false, false},
		{"ai_backfill_scope", inventoryAIBackfillScopeSQL, []string{"total", "actual_backfill_pending", "subject_index_null", "testee_index_null", "nonempty_assessment_arrays", "invalid_assessment_array_shape", "invalid_actor_field_shape", "indexed_identity_mismatch"}, false, false},
		{"ai_expected_relation_gaps", inventoryAIExpectedRelationGapsSQL, []string{"payload_relation_entries", "invalid_expected_id_entries", "valid_expected_entries_without_relation"}, false, false},
		{"ai_actual_relation_gaps", inventoryAIActualRelationGapsSQL, []string{"actual_relation_rows", "orphan_relations", "zero_assessment_relations", "relation_not_in_request_payload"}, false, false},
	} {
		width := len(step.fields)
		if step.grouped {
			width = 2
		}
		rows, success := read(step.query, step.stage, width)
		if !success {
			return r
		}
		var counts map[string]uint64
		if step.grouped {
			counts, success = inventoryGroupedCounts(rows, step.fields, step.requireAll)
		} else {
			counts, success = inventoryScalarCounts(rows, step.fields)
		}
		if !success {
			return invalidateInventory(r, "metadata_invalid", step.stage)
		}
		m.Counts[step.stage] = counts
	}
	if !inventoryConserves(m.Counts) {
		return invalidateInventory(r, "counts_inconsistent", "conservation")
	}
	if !rollback() {
		return r
	}
	// These reads are outside the RR transaction: a live migration cannot be
	// hidden by the earlier consistent read view.
	ledger, ok = read(readinessMigrationSQL, "migration_after", 2)
	if !ok {
		return r
	}
	if !readinessClean100(ledger) {
		return invalidateInventory(r, "migration_pair_rejected", "migration_after")
	}
	after, ok := readIdentity("identity_after")
	if !ok {
		return r
	}
	if before != after {
		return invalidateInventory(r, "session_identity_changed", "identity_after")
	}
	r.ErrorCategory, r.FailureStage, r.Measurements = "none", "none", m
	return r
}

func releaseInventoryDatabase(db *sql.DB, r inventoryReceipt) inventoryReceipt {
	ok := db.Close() == nil
	r.DatabaseReleased = &ok
	stats := db.Stats()
	closed := stats.OpenConnections == 0 && stats.InUse == 0
	r.PoolClosedVerified = &closed
	if !ok || !closed {
		return invalidateInventory(r, "database_release_failed", "database_release")
	}
	if r.ErrorCategory == "none" && (r.deadline.IsZero() || !time.Now().Before(r.deadline)) {
		return invalidateInventory(r, "query_deadline_exceeded", "database_release")
	}
	if r.ErrorCategory == "none" && r.ConnectionReleased != nil && *r.ConnectionReleased && r.TransactionReleased != nil && *r.TransactionReleased && r.Measurements != nil && r.QueryCount == 15 && r.ControlStatements == 3 {
		r.Complete = true
	}
	return r
}

func runInventory(get func(string) string, out io.Writer) int {
	_ = mysql.SetLogger(&mysql.NopLogger{})
	b := binding{SHA: get("PROFILE_SOURCE_SHA"), RunID: get("PROFILE_RUN_ID"), Expected: get("PROFILE_EXPECTED_TARGET_HASH")}
	r := baseInventoryReceipt(b)
	if validBinding(b) {
		cfg, ok := envConnection(get)
		if !ok || cfg.Database != "qs" {
			r = invalidateInventory(r, "connection_input_invalid", "input")
		} else if connector, err := mysql.NewConnector(mysqlConfig(cfg)); err != nil {
			r = invalidateInventory(r, "connection_config_failed", "connect")
		} else {
			db := sql.OpenDB(connector)
			db.SetMaxOpenConns(1)
			db.SetMaxIdleConns(1)
			r = releaseInventoryDatabase(db, probeInventory(context.Background(), db, cfg, b))
		}
	}
	if json.NewEncoder(out).Encode(r) != nil || !r.Complete {
		return 1
	}
	return 0
}
