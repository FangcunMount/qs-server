package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

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
	statements := mysqlOutboxScopeStatements(config{})
	required := map[string]string{
		"mysql outbox ids from assessment aggregate":           "o.aggregate_type = 'Assessment'",
		"mysql outbox ids from report aggregate":               "o.aggregate_type = 'Report'",
		"mysql outbox ids from answersheet aggregate":          "o.aggregate_type = 'AnswerSheet'",
		"mysql outbox ids from behavior testee aggregate":      "o.aggregate_type = 'BehaviorFootprint'",
		"mysql outbox ids from behavior answersheet aggregate": "o.aggregate_type = 'BehaviorFootprint'",
		"mysql outbox ids from behavior assessment aggregate":  "o.aggregate_type = 'BehaviorFootprint'",
		"mysql outbox ids from behavior report aggregate":      "o.aggregate_type = 'BehaviorFootprint'",
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

func TestMySQLOutboxScopePayloadScanIsExplicitOptIn(t *testing.T) {
	defaultStatements := mysqlOutboxScopeStatements(config{})
	for _, statement := range defaultStatements {
		if strings.Contains(statement.name, "payload_json") {
			t.Fatalf("payload_json statement %q should not be enabled by default", statement.name)
		}
	}

	optInStatements := mysqlOutboxScopeStatements(config{scanEventPayloads: true})
	var outboxPayload, pendingPayload bool
	for _, statement := range optInStatements {
		outboxPayload = outboxPayload || statement.name == "mysql outbox ids from payload_json"
		pendingPayload = pendingPayload || statement.name == "analytics pending ids from payload_json"
	}
	if !outboxPayload || !pendingPayload {
		t.Fatalf("scanEventPayloads should add both payload_json statements; outbox=%v pending=%v", outboxPayload, pendingPayload)
	}
}

func TestIsMySQLUnknownTable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "mysql error 1146",
			err:  &mysql.MySQLError{Number: 1146, Message: "Table 'qs.statistics_daily' doesn't exist"},
			want: true,
		},
		{
			name: "wrapped text error",
			err:  fmt.Errorf("delete: %w", errors.New("Error 1146 (42S02): Table 'qs.statistics_daily' doesn't exist")),
			want: true,
		},
		{
			name: "other mysql error",
			err:  &mysql.MySQLError{Number: 1064, Message: "syntax error"},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMySQLUnknownTable(tt.err); got != tt.want {
				t.Fatalf("isMySQLUnknownTable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateMySQLTableName(t *testing.T) {
	if err := validateMySQLTableName("statistics_daily"); err != nil {
		t.Fatalf("validateMySQLTableName() = %v, want nil", err)
	}
	if err := validateMySQLTableName("stats;drop"); err == nil {
		t.Fatal("validateMySQLTableName() should reject unsafe table names")
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
		"statistics_daily_testee",
		"statistics_accumulated_testee",
		"runtime_checkpoint",
		"analytics_pending_event",
		"domain_event_outbox",
		"behavior_footprint",
		"assessment_episode",
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
	for _, name := range []string{"behavior_footprint", "assessment_episode", "assessment_task", "assessment_score"} {
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

func makeUint64Range(start uint64, count int) []uint64 {
	out := make([]uint64, count)
	for i := range out {
		out[i] = start + uint64(i)
	}
	return out
}
