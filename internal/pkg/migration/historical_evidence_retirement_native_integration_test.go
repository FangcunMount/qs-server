//go:build integration

package migration

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	drivermysql "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
)

const historicalEvidenceUp = "migrations/mysql/000101_retire_historical_event_evidence.up.sql"

// This is a real clean 100/39 fixture, built by running the unchanged historical
// resources. No force or direct migration metadata write is used to create it.
func nativeHistoricalEvidencePair(t *testing.T) nativeSchemaPair {
	t.Helper()
	t.Setenv("QS_M3_REQUIRE_DATABASE", "1")
	r := migrationResourcesThrough(t, 100, 39)
	n := openNativeSchemaPairWithPrefix(t, "qs_m4_test")
	n.approve(t, r)
	n.runBoth(t, n.preflight(t, r))
	return n
}

func seedHistoricalEvidenceRows(t *testing.T, n nativeSchemaPair) {
	t.Helper()
	for _, id := range []uint64{4101, 4102} {
		if _, err := n.db.ExecContext(t.Context(), "INSERT INTO assessment (id,org_id,testee_id,questionnaire_code,questionnaire_version,answer_sheet_id,origin_type,status,evaluation_model_kind,evaluation_model_code,evaluation_model_version,primary_score_value) VALUES(?,1,2,'m4-test','1',?,'direct','evaluated','scale','m4-model','1',11.5)", id, id); err != nil {
			t.Fatal(err)
		}
		payload := fmt.Sprintf(`{"schema_version":2,"assessment_id":%d,"score":11.5}`, id)
		report := `{"schema_version":3,"InterpretationAssets":{"frozen":"保留历史报告"},"ReportMap":{"summary":"canonical"}}`
		if _, err := n.db.ExecContext(t.Context(), "INSERT INTO evaluation_outcome (id,org_id,assessment_id,testee_id,evaluation_run_id,model_kind,model_code,model_version,payload_json,report_input_json,evaluated_at,committed_event_id,committed_event_evidence) VALUES(?,1,?,2,?,'scale','m4-model','1',?,?,'2026-10-13 00:00:00',?,?)", id+1000, id, fmt.Sprintf("m4-run-%d", id), payload, report, fmt.Sprintf("m4-current-event-%d", id), `{"version":1,"canonical":"keep"}`); err != nil {
			t.Fatal(err)
		}
	}
}

func historicalQuery(t *testing.T, db *sql.DB, query string) [][]sql.NullString {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var result [][]sql.NullString
	for rows.Next() {
		row := make([]sql.NullString, len(columns))
		args := make([]any, len(columns))
		for i := range row {
			args[i] = &row[i]
		}
		if err := rows.Scan(args...); err != nil {
			t.Fatal(err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}

func canonicalEvidenceRows(t *testing.T, n nativeSchemaPair) [][][]sql.NullString {
	t.Helper()
	return [][][]sql.NullString{
		historicalQuery(t, n.db, "SELECT id,org_id,testee_id,answer_sheet_id,status,primary_score_value,version FROM assessment ORDER BY id"),
		historicalQuery(t, n.db, "SELECT id,assessment_id,evaluation_run_id,payload_json,report_input_json,committed_event_id,CAST(committed_event_evidence AS CHAR) FROM evaluation_outcome ORDER BY id"),
	}
}

func assertHistoricalObjects(t *testing.T, n nativeSchemaPair, assessment, outcome bool) {
	t.Helper()
	for _, item := range []struct {
		table, column, check string
		present              bool
	}{
		{"assessment", "historical_lifecycle_evidence", "chk_assessment_historical_lifecycle", assessment},
		{"evaluation_outcome", "historical_committed_evidence", "chk_outcome_historical_committed", outcome},
	} {
		for _, query := range []string{
			fmt.Sprintf("SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='%s' AND column_name='%s'", item.table, item.column),
			fmt.Sprintf("SELECT COUNT(*) FROM information_schema.table_constraints WHERE constraint_schema=DATABASE() AND table_name='%s' AND constraint_name='%s' AND constraint_type='CHECK'", item.table, item.check),
		} {
			var count int
			if err := n.db.QueryRowContext(t.Context(), query).Scan(&count); err != nil || (count == 1) != item.present {
				t.Fatalf("%s.%s present=%v count=%d error=%v", item.table, item.column, item.present, count, err)
			}
		}
	}
}

func TestHistoricalEvidenceNativeSQLOnlyUpgradeAndRestart(t *testing.T) {
	n := nativeHistoricalEvidencePair(t)
	seedHistoricalEvidenceRows(t, n)
	before := canonicalEvidenceRows(t, n)
	mongoBefore, err := n.mongoDB.Collection("schema_migrations").FindOne(t.Context(), bson.D{}).DecodeBytes()
	if err != nil {
		t.Fatal(err)
	}
	assertHistoricalObjects(t, n, true, true)
	r := migrationResourcesThrough(t, 101, 39)
	p := n.preflight(t, r)
	if version, changed, err := NewMigrator(n.db, p.MySQLConfig(false)).Run(); err != nil || !changed || version != 101 {
		t.Fatalf("SQL-only upgrade version=%d changed=%v error=%v", version, changed, err)
	}
	if version, changed, err := NewMongoMigrator(n.client, p.MongoConfig(false)).Run(); err != nil || changed || version != 39 {
		t.Fatalf("SQL-only release changed Mongo: version=%d changed=%v error=%v", version, changed, err)
	}
	assertHistoricalObjects(t, n, false, false)
	if err := p.VerifyComplete(t.Context()); err != nil {
		t.Fatal(err)
	}
	restart := n.preflight(t, r)
	for _, migrator := range []*Migrator{NewMigrator(n.db, restart.MySQLConfig(false)), NewMongoMigrator(n.client, restart.MongoConfig(false))} {
		if _, changed, err := migrator.Run(); err != nil || changed {
			t.Fatal("clean SQL-only restart must run no DDL", changed, err)
		}
	}
	mongoAfter, err := n.mongoDB.Collection("schema_migrations").FindOne(t.Context(), bson.D{}).DecodeBytes()
	if err != nil || !bytes.Equal(mongoBefore, mongoAfter) || !reflect.DeepEqual(before, canonicalEvidenceRows(t, n)) {
		t.Fatal("SQL-only retirement changed Mongo metadata or canonical business/report/event evidence", err)
	}
	if sql, dirty, mongo, mongoDirty := n.heads(t); sql != 101 || mongo != 39 || dirty || mongoDirty {
		t.Fatalf("heads=%d/%d dirty=%v/%v", sql, mongo, dirty, mongoDirty)
	}
}

func TestHistoricalEvidenceNativePartialDDLRemainsDirtyAndRefusesDown(t *testing.T) {
	n := nativeHistoricalEvidencePair(t)
	seedHistoricalEvidenceRows(t, n)
	before := canonicalEvidenceRows(t, n)
	release := migrationResourcesThrough(t, 101, 39)
	source := release.source.(fstest.MapFS)
	// The first real ALTER succeeds. A missing CHECK makes the second real ALTER
	// fail atomically, demonstrating the cross-table partial DDL boundary.
	source[historicalEvidenceUp].Data = bytes.Replace(source[historicalEvidenceUp].Data, []byte("DROP CHECK `chk_outcome_historical_committed`"), []byte("DROP CHECK `m4_missing_check`"), 1)
	r, err := readMigrationResources(source, release.pairs)
	if err != nil {
		t.Fatal(err)
	}
	p := n.preflight(t, r)
	if _, _, err := NewMigrator(n.db, p.MySQLConfig(false)).Run(); err == nil {
		t.Fatal("partial DDL did not fail")
	}
	assertHistoricalObjects(t, n, false, true)
	if _, _, err := NewMongoMigrator(n.client, p.MongoConfig(false)).Run(); err == nil {
		t.Fatal("failed pair continued into Mongo")
	}
	if _, err := preflightCompatibilityPair(t.Context(), n.db, n.client, n.cfg, testCurrentResources(t)); err == nil {
		t.Fatal("fresh process adopted dirty partial DDL")
	}
	down, err := migrations.ReadFile("migrations/mysql/000101_retire_historical_event_evidence.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	_, downErr := n.db.ExecContext(t.Context(), string(down))
	var refusal *drivermysql.MySQLError
	if !errors.As(downErr, &refusal) || string(refusal.SQLState[:]) != "45000" {
		t.Fatal("down must explicitly signal its forward-only refusal", downErr)
	}
	driver := NewMySQLDriver(n.db)
	instance, err := driver.CreateInstance(r.source, ensureConfigDefaults(p.MySQLConfig(false)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := driver.finishRun(); err != nil {
			t.Error(err)
		}
	})
	for _, version := range []int{100, 101} {
		if err := instance.Force(version); err == nil {
			t.Fatalf("force%d cleared partial DDL metadata", version)
		}
	}
	if sql, dirty, mongo, mongoDirty := n.heads(t); sql != 101 || mongo != 39 || !dirty || mongoDirty {
		t.Fatalf("partial DDL must remain101 dirty/39 clean: heads=%d/%d dirty=%v/%v", sql, mongo, dirty, mongoDirty)
	}
	if !reflect.DeepEqual(before, canonicalEvidenceRows(t, n)) {
		t.Fatal("failed historical DDL changed canonical business/report/event evidence")
	}
}

func syntheticHistoricalEvidence(t *testing.T, eventType string) string {
	t.Helper()
	digest := map[string]any{"kind": "m4_synthetic_fixture", "sha256": strings.Repeat("a", 64)}
	raw, err := json.Marshal(map[string]any{"version": 1, "entries": []any{map[string]any{
		"event_id": "m4-synthetic-event", "event_type": eventType,
		"source": map[string]any{"database": "mysql", "object": "domain_event_outbox", "primary_key_kind": "mysql_uint64", "primary_key_sha256": strings.Repeat("b", 64), "digest": digest},
		"proof":  map[string]any{"version": 1, "class": "retired_verified", "event_id": "m4-synthetic-event", "digest": digest, "business_binding_sha256": strings.Repeat("c", 64), "origin": "retirement", "verification": map[string]any{"method": "isolated_restore_fixture", "version": "1", "operation_id": "1-1", "verified_at": "2026-10-13T00:00:00Z", "business_terminal": true, "ownership_verified": true, "responsibility_closed": true}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// This deliberately restores only the two schema objects and their synthetic
// values in a disposable database. It does not validate a complete database
// backup, a production backup pair, or a production recovery procedure.
func TestHistoricalEvidenceNativeSchemaObjectRestoreWithSyntheticNonNull(t *testing.T) {
	n := nativeHistoricalEvidencePair(t)
	seedHistoricalEvidenceRows(t, n)
	for _, item := range []struct{ table, column, eventType string }{
		{"assessment", "historical_lifecycle_evidence", "evaluation.requested"},
		{"evaluation_outcome", "historical_committed_evidence", "evaluation.outcome.committed"},
	} {
		if _, err := n.db.ExecContext(t.Context(), "UPDATE `"+item.table+"` SET `"+item.column+"`=? ORDER BY id LIMIT 1", syntheticHistoricalEvidence(t, item.eventType)); err != nil {
			t.Fatal("synthetic non-NULL must satisfy the original JSON CHECK", err)
		}
	}
	before := canonicalEvidenceRows(t, n)
	ddlBefore := [][][]sql.NullString{historicalQuery(t, n.db, "SHOW CREATE TABLE assessment"), historicalQuery(t, n.db, "SHOW CREATE TABLE evaluation_outcome")}
	values := [][][]sql.NullString{historicalQuery(t, n.db, "SELECT id,CAST(historical_lifecycle_evidence AS CHAR) FROM assessment ORDER BY id"), historicalQuery(t, n.db, "SELECT id,CAST(historical_committed_evidence AS CHAR) FROM evaluation_outcome ORDER BY id")}
	up, err := migrations.ReadFile(historicalEvidenceUp)
	if err != nil {
		t.Fatal(err)
	}
	// Object-only rehearsal: version metadata remains the genuine100/39 fixture
	// throughout; no application process runs against this temporary structure.
	if _, err := n.db.ExecContext(t.Context(), string(up)); err != nil {
		t.Fatal(err)
	}
	assertHistoricalObjects(t, n, false, false)
	restore, err := migrations.ReadFile("migrations/mysql/000099_business_historical_references.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := n.db.ExecContext(t.Context(), string(restore)); err != nil {
		t.Fatal(err)
	}
	for i, item := range []struct{ table, column string }{{"assessment", "historical_lifecycle_evidence"}, {"evaluation_outcome", "historical_committed_evidence"}} {
		if len(values[i]) != 2 || !values[i][0][1].Valid || values[i][1][1].Valid {
			t.Fatal("restore fixture must include one valid non-NULL and one NULL", values[i])
		}
		for _, row := range values[i] {
			if _, err := n.db.ExecContext(t.Context(), "UPDATE `"+item.table+"` SET `"+item.column+"`=? WHERE id=?", row[1], row[0].String); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := n.db.ExecContext(t.Context(), "UPDATE `"+item.table+"` SET `"+item.column+"`='{}' ORDER BY id LIMIT 1"); err == nil {
			t.Fatal("restored JSON CHECK accepted invalid evidence")
		}
		if got := historicalQuery(t, n.db, "SELECT id,CAST(`"+item.column+"` AS CHAR) FROM `"+item.table+"` ORDER BY id"); !reflect.DeepEqual(got, values[i]) {
			t.Fatal("restored synthetic values or NULL distribution changed", got)
		}
	}
	assertHistoricalObjects(t, n, true, true)
	ddlAfter := [][][]sql.NullString{historicalQuery(t, n.db, "SHOW CREATE TABLE assessment"), historicalQuery(t, n.db, "SHOW CREATE TABLE evaluation_outcome")}
	if !reflect.DeepEqual(ddlBefore, ddlAfter) || !reflect.DeepEqual(before, canonicalEvidenceRows(t, n)) {
		t.Fatal("object restore changed full table DDL or canonical rows")
	}
	if sql, dirty, mongo, mongoDirty := n.heads(t); sql != 100 || mongo != 39 || dirty || mongoDirty {
		t.Fatalf("object-only rehearsal changed clean fixture metadata: heads=%d/%d dirty=%v/%v", sql, mongo, dirty, mongoDirty)
	}
}
