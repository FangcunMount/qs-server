package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestCleanupPreservesStatisticsRunAuditLedger(t *testing.T) {
	items, err := mysqlDeleteItems(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.name == "statistics_sync_run" || strings.Contains(item.stmt, "statistics_sync_run") {
			t.Fatalf("cleanup must preserve statistics_sync_run audit rows: %+v", item)
		}
	}
}

func TestValidateBackupSuffixChecksGeneratedMySQLIdentifiers(t *testing.T) {
	if err := validateBackupSuffix("seeddata_dup_20260812_v1"); err != nil {
		t.Fatalf("documented backup suffix should be valid: %v", err)
	}
	if err := validateBackupSuffix(strings.Repeat("x", mysqlIdentifierMaxLength)); err == nil || !strings.Contains(err.Error(), "exceeds MySQL identifier limit") {
		t.Fatalf("oversized backup suffix error = %v, want MySQL identifier limit error", err)
	}
	for _, item := range append(mysqlBackupItems(), iamBackupItems()...) {
		name, err := mysqlBackupTableName(item.table, "seeddata_dup_20260812_v1")
		if err != nil {
			t.Fatalf("backup table name for %s: %v", item.table, err)
		}
		if len(name) > mysqlIdentifierMaxLength {
			t.Fatalf("backup table name %q length=%d, want <=%d", name, len(name), mysqlIdentifierMaxLength)
		}
	}
}

func TestIsMongoUnauthorized(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "command error code",
			err:  mongo.CommandError{Code: 13, Name: "Unauthorized", Message: "requires authentication"},
			want: true,
		},
		{
			name: "wrapped text error",
			err:  errors.New("(Unauthorized) Command find requires authentication"),
			want: true,
		},
		{
			name: "ordinary error",
			err:  errors.New("server selection timeout"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMongoUnauthorized(tt.err); got != tt.want {
				t.Fatalf("isMongoUnauthorized() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMongoOutboxFiltersAreChunked(t *testing.T) {
	ids := scopeIDs{
		AnswerSheetIDs: makeUint64Range(1, mongoIDChunkSize+1),
		AssessmentIDs:  makeUint64Range(10_000, mongoIDChunkSize+1),
		ReportIDs:      makeUint64Range(20_000, 2),
		TesteeIDs:      []uint64{30_000},
	}

	filters := mongoOutboxFilters(ids)
	if len(filters) < 5 {
		t.Fatalf("filter count = %d, want chunked filters", len(filters))
	}
	for _, filter := range filters {
		idsFilter, ok := filter["aggregate_id"].(bson.M)
		if !ok {
			t.Fatalf("aggregate_id filter = %#v, want bson.M", filter["aggregate_id"])
		}
		values, ok := idsFilter["$in"].([]string)
		if !ok {
			t.Fatalf("$in = %#v, want []string", idsFilter["$in"])
		}
		if len(values) > mongoIDChunkSize {
			t.Fatalf("chunk size = %d, want <= %d", len(values), mongoIDChunkSize)
		}
	}
}

func TestMySQLOutboxScopeStatementsConstrainAggregateType(t *testing.T) {
	statements := mysqlOutboxScopeStatements()
	required := map[string]string{
		"mysql outbox ids from assessment aggregate":  "o.aggregate_type = 'Assessment'",
		"mysql outbox ids from report aggregate":      "o.aggregate_type = 'Report'",
		"mysql outbox ids from answersheet aggregate": "o.aggregate_type = 'AnswerSheet'",
	}

	seen := map[string]struct{}{}
	for _, statement := range statements {
		want, ok := required[statement.name]
		if !ok {
			continue
		}
		seen[statement.name] = struct{}{}
		if !strings.Contains(statement.sql, want) {
			t.Fatalf("%s SQL must constrain aggregate type with %q; sql=%s", statement.name, want, statement.sql)
		}
		if !strings.Contains(statement.sql, "BINARY o.aggregate_id = BINARY CAST(") {
			t.Fatalf("%s SQL must keep binary aggregate_id comparison; sql=%s", statement.name, statement.sql)
		}
	}
	for name := range required {
		if _, ok := seen[name]; !ok {
			t.Fatalf("missing mysql outbox scope statement %q", name)
		}
	}
}

func TestMySQLOutboxScopeNeverUsesPerTesteePayloadRegexpJoin(t *testing.T) {
	for _, statement := range mysqlOutboxScopeStatements() {
		if strings.Contains(statement.sql, "payload_json") || strings.Contains(statement.sql, "REGEXP") {
			t.Fatalf("outbox scope statement %q must not scan payload_json in SQL: %s", statement.name, statement.sql)
		}
	}
}

func TestPayloadContainsTesteeID(t *testing.T) {
	targets := map[uint64]struct{}{631727129519731246: {}}
	tests := []struct {
		name    string
		payload string
		want    bool
	}{
		{name: "numeric nested", payload: `{"data":{"testee_id":631727129519731246}}`, want: true},
		{name: "string nested in array", payload: `{"items":[{"testee_id":"631727129519731246"}]}`, want: true},
		{name: "other testee", payload: `{"testee_id":631727129519731247}`},
		{name: "similar key", payload: `{"canonical_testee_id":631727129519731246}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := payloadContainsTesteeID([]byte(tt.payload), targets)
			if err != nil {
				t.Fatalf("payloadContainsTesteeID() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("payloadContainsTesteeID() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestPayloadContainsTesteeIDFailsClosedOnInvalidJSON(t *testing.T) {
	if _, err := payloadContainsTesteeID([]byte(`{"testee_id":`), map[uint64]struct{}{1: {}}); err == nil {
		t.Fatal("payloadContainsTesteeID() error = nil, want invalid JSON error")
	}
}

func TestIsMySQLLockError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "lock wait timeout", err: &mysql.MySQLError{Number: 1205}, want: true},
		{name: "deadlock", err: &mysql.MySQLError{Number: 1213}, want: true},
		{name: "other", err: &mysql.MySQLError{Number: 1064}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMySQLLockError(tt.err); got != tt.want {
				t.Fatalf("isMySQLLockError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMySQLChunkedDeleteSpecForLargeDeleteTables(t *testing.T) {
	for _, name := range []string{
		"domain_event_outbox",
		"assessment_entry_intake_log",
		"clinician_relation",
		"assessment_task",
		"assessment_score",
		"assessment",
	} {
		t.Run(name, func(t *testing.T) {
			spec, ok := mysqlChunkedDeleteSpecFor(name)
			if !ok {
				t.Fatalf("mysqlChunkedDeleteSpecFor(%q) ok = false, want true", name)
			}
			for label, sql := range map[string]string{
				"create": spec.createBatchTable,
				"clear":  spec.clearBatchTable,
				"fill":   spec.fillBatchTable,
				"delete": spec.deleteBatch,
			} {
				if strings.TrimSpace(sql) == "" {
					t.Fatalf("%s %s SQL is empty", name, label)
				}
			}
			if !strings.Contains(spec.fillBatchTable, "LIMIT ?") {
				t.Fatalf("%s fill SQL should limit each batch; sql=%s", name, spec.fillBatchTable)
			}
		})
	}

	if _, ok := mysqlChunkedDeleteSpecFor("testee"); ok {
		t.Fatal("testee should keep the ordinary delete path")
	}
}

func TestMySQLChunkedDeleteUsesStagingTablesForMultiSourceTables(t *testing.T) {
	for _, name := range []string{"assessment_task", "assessment_score"} {
		t.Run(name, func(t *testing.T) {
			spec, ok := mysqlChunkedDeleteSpecFor(name)
			if !ok {
				t.Fatalf("mysqlChunkedDeleteSpecFor(%q) ok = false, want true", name)
			}
			if strings.Contains(spec.fillBatchTable, "UNION") {
				t.Fatalf("%s fill SQL should read from staging table, not UNION scans; sql=%s", name, spec.fillBatchTable)
			}
			if spec.pruneStagingTable == "" {
				t.Fatalf("%s should prune staging ids after each batch", name)
			}
			if strings.Count(spec.fillBatchTable, "?") != 1 {
				t.Fatalf("%s fill SQL should accept one batch size placeholder; sql=%s", name, spec.fillBatchTable)
			}
		})
	}
}

func TestProgressPhaseElapsedSurvivesRunStep(t *testing.T) {
	initProgress(true)
	prog.phaseStarted = time.Now().Add(-2 * time.Second)
	prog.phase = "phase"
	if err := prog.RunStep("step", 1, 1, func() error { return nil }); err != nil {
		t.Fatalf("RunStep() = %v", err)
	}
	if prog.phaseStarted.IsZero() {
		t.Fatal("phaseStarted should survive RunStep")
	}
	if time.Since(prog.phaseStarted) < time.Second {
		t.Fatal("phaseStarted should keep phase timing")
	}
}

func TestScopeIDsEqualNormalizesOrderDuplicatesAndZero(t *testing.T) {
	left := scopeIDs{
		TesteeIDs:      []uint64{2, 1, 1, 0},
		AssessmentIDs:  []uint64{10, 11},
		AnswerSheetIDs: []uint64{20, 20},
		ReportIDs:      []uint64{30},
	}
	right := scopeIDs{
		TesteeIDs:      []uint64{1, 2},
		AssessmentIDs:  []uint64{11, 10},
		AnswerSheetIDs: []uint64{20},
		ReportIDs:      []uint64{30, 0},
	}
	if !scopeIDsEqual(left, right) {
		t.Fatal("scopeIDsEqual should normalize order, duplicates, and zero values")
	}

	right.ReportIDs = append(right.ReportIDs, 31)
	if scopeIDsEqual(left, right) {
		t.Fatal("scopeIDsEqual should detect changed report scope")
	}
}

func TestScopeTouchedDateSQLDoesNotReopenTemporaryTables(t *testing.T) {
	for _, table := range []string{
		"tmp_cleanup_assessment_ids",
		"tmp_cleanup_statistics_dates",
		"tmp_cleanup_testee_ids",
	} {
		if got := strings.Count(scopeTouchedDateSQL, table); got != 1 {
			t.Fatalf("scope touched-date SQL references %s %d times, want exactly once; MySQL cannot reopen a temporary table in one statement", table, got)
		}
	}
	for _, table := range []string{
		"statistics_access_fact",
		"statistics_assessment_fact",
		"statistics_plan_fact",
	} {
		if strings.Contains(scopeTouchedDateSQL, table) {
			t.Fatalf("scope touched-date SQL should reuse tmp_cleanup_statistics_dates instead of rescanning %s", table)
		}
	}
}

func makeUint64Range(start uint64, count int) []uint64 {
	out := make([]uint64, count)
	for i := range out {
		out[i] = start + uint64(i)
	}
	return out
}
