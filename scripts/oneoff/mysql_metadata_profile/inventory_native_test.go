//go:build integration

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

// These exact, disposable reader shapes exercise every fixed statement. They
// are not a production migration bootstrap or permission to write a live head.
func TestRetirementInventoryNativeOwnedMySQL(t *testing.T) {
	if os.Getenv("QS_METADATA_PROFILE_NATIVE_REQUIRED") != "1" {
		t.Fatal("native_mysql_required_not_enabled")
	}
	cfg, ok := envConnection(os.Getenv)
	if !ok || cfg.Host != "127.0.0.1" || cfg.Port == 3306 || cfg.User != "root" || !regexp.MustCompile(`^qs_profile_owned_[a-z0-9]{8,24}$`).MatchString(cfg.Database) {
		t.Fatal("owned_mysql_input_guard_failed")
	}
	connector, err := mysql.NewConnector(mysqlConfig(cfg))
	if err != nil {
		t.Fatal("owned_mysql_connector_failed")
	}
	admin := sql.OpenDB(connector)
	admin.SetMaxOpenConns(1)
	admin.SetMaxIdleConns(1)
	defer func() {
		if admin.Close() != nil {
			t.Error("owned_admin_release_failed")
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	var uuid, version string
	var count int
	if admin.QueryRowContext(ctx, "SELECT @@server_uuid,VERSION()").Scan(&uuid, &version) != nil || uuid != os.Getenv("QS_METADATA_PROFILE_NATIVE_UUID") || !uuidRE.MatchString(uuid) || version != "8.0.36" {
		t.Fatal("owned_mysql_identity_guard_failed")
	}
	if admin.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA=DATABASE()").Scan(&count) != nil || count != 0 {
		t.Fatal("owned_schema_not_empty")
	}
	const password = "native-inventory-private-fixture"
	ddl := map[string]string{
		"testee":                             "CREATE TABLE testee(id BIGINT UNSIGNED PRIMARY KEY,tags JSON NULL) ENGINE=InnoDB",
		"operators":                          "CREATE TABLE operators(id BIGINT UNSIGNED PRIMARY KEY,roles JSON NULL,effective_roles JSON NULL) ENGINE=InnoDB",
		"testee_store_migration_manifests":   "CREATE TABLE testee_store_migration_manifests(migration_id VARCHAR(64) PRIMARY KEY,state VARCHAR(32)) ENGINE=InnoDB",
		"testee_store_migration_items":       "CREATE TABLE testee_store_migration_items(migration_id VARCHAR(64),testee_id BIGINT UNSIGNED,disposition VARCHAR(32),PRIMARY KEY(migration_id,testee_id)) ENGINE=InnoDB",
		"testee_store_migration_reversions":  "CREATE TABLE testee_store_migration_reversions(migration_id VARCHAR(64),testee_id BIGINT UNSIGNED,PRIMARY KEY(migration_id,testee_id)) ENGINE=InnoDB",
		"operator_retirement_manifests":      "CREATE TABLE operator_retirement_manifests(migration_id VARCHAR(64) PRIMARY KEY,stage VARCHAR(32)) ENGINE=InnoDB",
		"clinician_operator_binding_archive": "CREATE TABLE clinician_operator_binding_archive(id BIGINT UNSIGNED PRIMARY KEY,migration_id VARCHAR(64)) ENGINE=InnoDB",
		"ai_bridge_requests":                 "CREATE TABLE ai_bridge_requests(request_id VARCHAR(64) PRIMARY KEY,payload JSON NOT NULL,organization_id BIGINT UNSIGNED NULL,subject_id VARCHAR(128) NULL,testee_id BIGINT UNSIGNED NULL) ENGINE=InnoDB",
		"ai_bridge_request_assessments":      "CREATE TABLE ai_bridge_request_assessments(request_id VARCHAR(64),assessment_id BIGINT UNSIGNED,PRIMARY KEY(request_id,assessment_id)) ENGINE=InnoDB",
	}
	exec := func(t *testing.T, statement string, args ...any) {
		t.Helper()
		if _, e := admin.ExecContext(ctx, statement, args...); e != nil {
			t.Fatal("owned_inventory_fixture_statement_failed")
		}
	}
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 10*time.Second)
		defer done()
		for _, name := range append(append([]string{}, inventoryTables...), "schema_migrations") {
			if _, e := admin.ExecContext(cleanup, "DROP TABLE IF EXISTS `"+name+"`"); e != nil {
				t.Error("owned_inventory_table_release_failed")
			}
		}
		for _, user := range []string{"inventory_reader", "inventory_columns"} {
			if _, e := admin.ExecContext(cleanup, "DROP USER IF EXISTS `"+user+"`@`%`"); e != nil {
				t.Error("owned_inventory_user_release_failed")
			}
		}
	}()
	exec(t, "CREATE TABLE schema_migrations(version BIGINT UNSIGNED PRIMARY KEY,dirty BOOLEAN NOT NULL) ENGINE=InnoDB")
	exec(t, "INSERT INTO schema_migrations VALUES(100,false)")
	for _, name := range inventoryTables {
		exec(t, ddl[name])
	}
	for _, user := range []string{"inventory_reader", "inventory_columns"} {
		exec(t, "CREATE USER `"+user+"`@`%` IDENTIFIED BY '"+password+"'")
	}
	exec(t, "GRANT SELECT ON `"+cfg.Database+"`.* TO `inventory_reader`@`%`")
	exec(t, "GRANT SELECT ON `"+cfg.Database+"`.schema_migrations TO `inventory_columns`@`%`")
	for _, name := range inventoryTables {
		columns := ""
		if name == "testee" {
			columns = " (id)"
		}
		exec(t, "GRANT SELECT"+columns+" ON `"+cfg.Database+"`.`"+name+"` TO `inventory_columns`@`%`")
	}
	exec(t, "INSERT INTO testee VALUES(1,NULL),(2,'null'),(3,'[]'),(4,'[\"native-private-tag\"]'),(5,'{\"native-private-body\":1}')")
	exec(t, "INSERT INTO operators VALUES(1,'[]','[]'),(2,'[\"a\"]','[\"b\"]'),(3,NULL,NULL),(4,'[\"α\",\"β\"]','[ \"α\", \"β\" ]')")
	exec(t, "INSERT INTO testee_store_migration_manifests VALUES('m1','applied'),('m2','rolled_back'),('m3','APPLIED')")
	exec(t, "INSERT INTO testee_store_migration_items VALUES('m1',1,'candidate'),('m1',2,'deferred'),('m2',3,'already_assigned'),('missing',4,'UNRECOGNIZED')")
	exec(t, "INSERT INTO testee_store_migration_reversions VALUES('m1',1),('m1',999)")
	exec(t, "INSERT INTO operator_retirement_manifests VALUES('o1','pending'),('o2','executing'),('o3','UNRECOGNIZED')")
	exec(t, "INSERT INTO clinician_operator_binding_archive VALUES(1,'o1'),(2,'missing')")
	max := "18446744073709551615"
	for _, v := range []struct {
		request                                   string
		org, subject, testee                      any
		indexedOrg, indexedSubject, indexedTestee any
		assessments                               any
	}{
		{"r1", "1", "opaque-subject", "1", "1", "opaque-subject", "1", []any{"1", "1", "2"}},
		{"r2", "2", "MixedCase", "2", "2", "mixedcase", "2", []any{"3", "01", 1, max, "1\n"}},
		{"r3", 1, "opaque", "1", nil, nil, nil, []any{}},
		{"r4", "18446744073709551616", "nul\x00subject", "0", "1", "opaque", "1", nil},
		{"r5", "1", strings.Repeat("界", 43), "1", "1", "opaque", "1", []any{"5"}},
		{"r6", max, strings.Repeat("x", 128), max, max, strings.Repeat("x", 128), max, []any{"6"}},
	} {
		raw, e := json.Marshal(map[string]any{"actor": map[string]any{"org_id": v.org, "subject_id": v.subject}, "testee_id": v.testee, "assessment_ids": v.assessments})
		if e != nil {
			t.Fatal("owned_inventory_payload_failed")
		}
		exec(t, "INSERT INTO ai_bridge_requests VALUES(?,?,?,?,?)", v.request, raw, v.indexedOrg, v.indexedSubject, v.indexedTestee)
	}
	exec(t, "INSERT INTO ai_bridge_request_assessments VALUES('r1',1),('r1',99),('r2',18446744073709551615),('r2',0),('r6',6),('orphan',7)")
	open := func(t *testing.T, user string) *sql.DB {
		t.Helper()
		copy := cfg
		copy.User, copy.Password = user, password
		con, e := mysql.NewConnector(mysqlConfig(copy))
		if e != nil {
			t.Fatal("owned_inventory_reader_connector_failed")
		}
		db := sql.OpenDB(con)
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		return db
	}
	b := binding{SHA: strings.Repeat("a", 40), RunID: "12345-1", Expected: hashText(cfg.Host, strconv.Itoa(cfg.Port), cfg.Database, uuid)}
	read := func(t *testing.T, user string) inventoryReceipt {
		t.Helper()
		db := open(t, user)
		r := releaseInventoryDatabase(db, probeInventory(ctx, db, cfg, b))
		if r.ConnectionReleased == nil || !*r.ConnectionReleased || r.DatabaseReleased == nil || !*r.DatabaseReleased || r.PoolClosedVerified == nil || !*r.PoolClosedVerified {
			t.Fatal("owned_inventory_release_unverified")
		}
		if remaining, observations, e := inventoryServerConnectionReleased(ctx, admin, user); e != nil {
			t.Fatalf("owned_inventory_server_connection_not_released: remaining=%d observations=%d deadline=%t", remaining, observations, errors.Is(e, context.DeadlineExceeded))
		}
		raw, e := json.Marshal(r)
		if e != nil {
			t.Fatal("owned_inventory_receipt_failed")
		}
		for _, private := range []string{uuid, password, user, cfg.Database, "native-private-tag", "native-private-body", "opaque-subject", "MixedCase", max, "UNRECOGNIZED", "CURRENT_ROLE"} {
			if bytes.Contains(raw, []byte(private)) {
				t.Fatal("owned_inventory_private_value_leaked")
			}
		}
		if r.FreezeVerified || !r.InventoryOnly || !r.Complete && r.Measurements != nil {
			t.Fatal("owned_inventory_scope_or_discard_failed")
		}
		return r
	}
	t.Run("closed_nonzero_counts_duplicate_expected_and_complete_relations", func(t *testing.T) {
		r := read(t, "inventory_reader")
		if !r.Complete || r.QueryCount != 15 || r.ControlStatements != 3 || r.Measurements == nil {
			t.Fatalf("native_inventory_incomplete:%s/%s", r.ErrorCategory, r.FailureStage)
		}
		c := r.Measurements.Counts
		for family, expected := range map[string]map[string]uint64{
			"testee_tags_partition":      {"total": 5, "sql_null": 1, "non_null": 4, "json_null": 1, "empty_array": 1, "non_empty_array": 1, "other": 1},
			"operators_role_duplication": {"total": 4, "roles_sql_null": 1, "effective_sql_null": 1, "both_arrays": 3, "same_canonical_json_bytes": 3, "different_canonical_json_bytes": 1},
			"testee_manifest_states":     {"applied": 1, "rolled_back": 1, "unknown": 1},
			"testee_item_dispositions":   {"candidate": 1, "deferred": 1, "already_assigned": 1, "unknown": 1},
			"operator_manifest_stages":   {"pending": 1, "executing": 1, "applied_unchanged": 0, "applied_drifted": 0, "invalid": 0, "unknown": 1},
			"historical_reference_gaps":  {"items_without_manifest": 1, "reversions_without_item": 1, "archives_without_manifest": 1},
			"ai_backfill_scope":          {"total": 6, "actual_backfill_pending": 1, "subject_index_null": 1, "testee_index_null": 1, "nonempty_assessment_arrays": 4, "invalid_assessment_array_shape": 2, "invalid_actor_field_shape": 3, "indexed_identity_mismatch": 1},
			"ai_expected_relation_gaps":  {"payload_relation_entries": 10, "invalid_expected_id_entries": 3, "valid_expected_entries_without_relation": 3},
			"ai_actual_relation_gaps":    {"actual_relation_rows": 6, "orphan_relations": 1, "zero_assessment_relations": 1, "relation_not_in_request_payload": 2},
		} {
			for field, want := range expected {
				if c[family][field] != want {
					t.Fatalf("native_inventory_count_mismatch:%s/%s:%d:%d", family, field, c[family][field], want)
				}
			}
		}
	})
	t.Run("real_column_privilege_denial_is_unknown", func(t *testing.T) {
		r := read(t, "inventory_columns")
		if r.Complete || r.Measurements != nil || r.ErrorCategory != "query_permission_denied" || r.FailureStage != "testee_tags_partition" || r.QueryMySQLNumber == nil || *r.QueryMySQLNumber != 1143 {
			t.Fatalf("native_inventory_column_denial_misclassified:%s/%s", r.ErrorCategory, r.FailureStage)
		}
	})
	t.Run("missing_owned_object_is_not_zero", func(t *testing.T) {
		exec(t, "DROP TABLE testee_store_migration_reversions")
		r := read(t, "inventory_reader")
		exec(t, ddl["testee_store_migration_reversions"])
		exec(t, "INSERT INTO testee_store_migration_reversions VALUES('m1',1),('m1',999)")
		if r.Complete || r.Measurements != nil || r.ErrorCategory != "object_shape_rejected" || r.QueryCount != 3 {
			t.Fatal("native_inventory_missing_table_became_zero")
		}
	})
	t.Run("noninnodb_owned_object_is_rejected", func(t *testing.T) {
		exec(t, "ALTER TABLE testee_store_migration_reversions ENGINE=MyISAM")
		r := read(t, "inventory_reader")
		exec(t, "ALTER TABLE testee_store_migration_reversions ENGINE=InnoDB")
		if r.Complete || r.Measurements != nil || r.ErrorCategory != "object_shape_rejected" {
			t.Fatal("native_inventory_nonsnapshot_engine_accepted")
		}
	})
	t.Run("dirty_or_other_head_is_rejected", func(t *testing.T) {
		for _, state := range []struct{ version, dirty int }{{100, 1}, {101, 0}} {
			exec(t, "UPDATE schema_migrations SET version=?,dirty=?", state.version, state.dirty)
			r := read(t, "inventory_reader")
			if r.Complete || r.Measurements != nil || r.ErrorCategory != "migration_pair_rejected" || r.QueryCount != 2 || r.TransactionReleased != nil {
				t.Fatal("native_inventory_old_or_dirty_head_accepted")
			}
		}
		exec(t, "UPDATE schema_migrations SET version=100,dirty=false")
	})
	t.Run("same_rr_snapshot_with_concurrent_actor_and_ai_changes", func(t *testing.T) {
		db := open(t, "inventory_reader")
		defer func() {
			if db.Close() != nil {
				t.Error("owned_snapshot_database_release_failed")
			}
		}()
		conn, e := db.Conn(ctx)
		if e != nil {
			t.Fatal("owned_snapshot_connection_failed")
		}
		defer func() {
			if conn.Close() != nil {
				t.Error("owned_snapshot_connection_release_failed")
			}
		}()
		for _, statement := range []string{readinessIsolationSQL, readinessBeginSQL} {
			if _, e = conn.ExecContext(ctx, statement); e != nil {
				t.Fatal("owned_snapshot_begin_failed")
			}
		}
		active := true
		defer func() {
			if active {
				_, e = conn.ExecContext(ctx, readinessRollbackSQL)
			}
			if e != nil {
				t.Error("owned_snapshot_rollback_failed")
			}
		}()
		// Establish the RR read view before another connection commits changes.
		if _, e = readinessRows(ctx, conn, inventoryTesteeTagsPartitionSQL, 7); e != nil {
			t.Fatal("owned_snapshot_initial_read_failed")
		}
		exec(t, "UPDATE testee SET tags='[]'")
		exec(t, "UPDATE operators SET effective_roles=roles")
		exec(t, "DELETE FROM ai_bridge_request_assessments")
		exec(t, "DELETE FROM ai_bridge_requests")
		exec(t, "UPDATE schema_migrations SET version=101")
		for _, statement := range []struct {
			query  string
			fields []string
			field  string
			want   uint64
		}{
			{inventoryTesteeTagsPartitionSQL, []string{"total", "sql_null", "non_null", "json_null", "empty_array", "non_empty_array", "other"}, "non_empty_array", 1},
			{inventoryOperatorsRoleDuplicationSQL, []string{"total", "roles_sql_null", "effective_sql_null", "both_arrays", "same_canonical_json_bytes", "different_canonical_json_bytes"}, "different_canonical_json_bytes", 1},
			{inventoryAIBackfillScopeSQL, []string{"total", "actual_backfill_pending", "subject_index_null", "testee_index_null", "nonempty_assessment_arrays", "invalid_assessment_array_shape", "invalid_actor_field_shape", "indexed_identity_mismatch"}, "total", 6},
			{inventoryAIExpectedRelationGapsSQL, []string{"payload_relation_entries", "invalid_expected_id_entries", "valid_expected_entries_without_relation"}, "payload_relation_entries", 10},
			{inventoryAIActualRelationGapsSQL, []string{"actual_relation_rows", "orphan_relations", "zero_assessment_relations", "relation_not_in_request_payload"}, "actual_relation_rows", 6},
		} {
			rows, queryErr := readinessRows(ctx, conn, statement.query, len(statement.fields))
			counts, valid := inventoryScalarCounts(rows, statement.fields)
			if queryErr != nil || !valid || counts[statement.field] != statement.want {
				t.Fatal("owned_inventory_rr_view_changed")
			}
		}
		ledger, e := readinessRows(ctx, conn, readinessMigrationSQL, 2)
		if e != nil || !readinessClean100(ledger) {
			t.Fatal("owned_snapshot_did_not_keep_original_ledger_view")
		}
		active = false
		if _, e = conn.ExecContext(ctx, readinessRollbackSQL); e != nil {
			t.Fatal("owned_snapshot_explicit_release_failed")
		}
		ledger, e = readinessRows(ctx, conn, readinessMigrationSQL, 2)
		if e != nil || readinessClean100(ledger) {
			t.Fatal("owned_live_ledger_was_hidden_after_rollback")
		}
		exec(t, "UPDATE schema_migrations SET version=100")
	})
	t.Run("empty_all_objects_is_complete_observation", func(t *testing.T) {
		for _, name := range inventoryTables {
			exec(t, "DELETE FROM `"+name+"`")
		}
		r := read(t, "inventory_reader")
		if !r.Complete || r.Measurements == nil {
			t.Fatalf("native_inventory_empty_incomplete:%s/%s", r.ErrorCategory, r.FailureStage)
		}
		for _, family := range r.Measurements.Counts {
			for _, count := range family {
				if count != 0 {
					t.Fatal("native_inventory_empty_counts_not_zero")
				}
			}
		}
	})
}

// The driver sends COM_QUIT and closes the local socket without waiting for the
// server to remove its thread. Observe that independent fact within the original
// test deadline and at most queryLimit; never retry the inventory or kill a session.
func inventoryServerConnectionReleased(parent context.Context, admin *sql.DB, user string) (remaining, observations int, err error) {
	ctx, cancel := context.WithTimeout(parent, queryLimit)
	defer cancel()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		observations++
		if err = admin.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.processlist WHERE USER=?", user).Scan(&remaining); err != nil {
			return remaining, observations, err
		}
		if remaining == 0 {
			return remaining, observations, nil
		}
		select {
		case <-ctx.Done():
			return remaining, observations, ctx.Err()
		case <-tick.C:
		}
	}
}
