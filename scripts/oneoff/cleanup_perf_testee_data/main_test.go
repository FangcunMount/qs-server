package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestHistoricalManifestScopesOnlyCreatedTestees(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	data := `{"batch_id":"hist-20250101-20260727-v1","scenarios":{"a":{"testee_id":"10","testee_created":true},"b":{"testee_id":"11","testee_created":false},"c":{}}}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	ids, err := loadHistoricalManifestTesteeIDs(path, "hist-20250101-20260727-v1")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != 10 {
		t.Fatalf("ids=%v, want [10]", ids)
	}
	if _, err := loadHistoricalManifestTesteeIDs(path, "other-batch"); err == nil {
		t.Fatal("batch mismatch must fail")
	}
}

func TestHistoricalManifestScopeIncludesEveryPersistedResourceID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	data := `{"batch_id":"hist-batch","scenarios":{"a":{"testee_id":"10","testee_created":true,"enrollment_id":"20","task_ids":["30"],"answersheet_id":"40","answersheet_ids":["40","41"],"assessment_id":"50","assessment_ids":["50","51"],"outcome_id":"60","report_id":"70","report_generation_id":"80","report_run_id":"90"}}}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	scope, err := loadHistoricalManifestScope(path, "hist-batch")
	if err != nil {
		t.Fatal(err)
	}
	for name, got := range map[string][]uint64{
		"testee": scope.TesteeIDs, "enrollment": scope.EnrollmentIDs, "task": scope.TaskIDs,
		"answersheet": scope.AnswerSheetIDs, "assessment": scope.AssessmentIDs, "outcome": scope.OutcomeIDs,
		"report": scope.ReportIDs, "generation": scope.GenerationIDs, "run": scope.ReportRunIDs,
	} {
		if len(got) == 0 {
			t.Fatalf("%s scope is empty", name)
		}
	}
	if len(scope.AnswerSheetIDs) != 2 || len(scope.AssessmentIDs) != 2 {
		t.Fatalf("scope=%+v, want singular and plural IDs merged without duplicates", scope)
	}
}

func TestHistoricalBatchScopeUsesLedgerIdentityAndExactLogs(t *testing.T) {
	statements := historicalBatchScopeStatements("hist-20250101-20260727-v1")
	joined := ""
	for _, statement := range statements {
		joined += statement.name + "\n" + statement.sql + "\n"
	}
	for _, required := range []string{
		"tmp_cleanup_seed_stage_ids", "$.resolve_log_id", "$.intake_log_id",
		"tmp_cleanup_resolve_log_ids", "tmp_cleanup_intake_log_ids", "tmp_cleanup_plan_enrollment_ids",
		"tmp_cleanup_assessment_task_ids", "tmp_cleanup_seed_attempt_ids", "tmp_cleanup_outcome_ids",
		"tmp_cleanup_relation_ids", "tmp_cleanup_statistics_dates",
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("historical scope missing %q", required)
		}
	}
}

func TestRollbackPreservesStatisticsRunAuditLedger(t *testing.T) {
	items, err := mysqlDeleteItems(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.name == "statistics_sync_run" || strings.Contains(item.stmt, "statistics_sync_run") {
			t.Fatalf("rollback must preserve statistics_sync_run audit history: %+v", item)
		}
	}
}

func TestHistoricalMongoScopeDoesNotExpandByTestee(t *testing.T) {
	ids := scopeIDs{TesteeIDs: []uint64{10}, AnswerSheetIDs: []uint64{20}}
	got := mongoScopeIDs(ids, config{seedBatchID: "hist-batch"})
	if len(got.TesteeIDs) != 0 || len(got.AnswerSheetIDs) != 1 || got.AnswerSheetIDs[0] != 20 {
		t.Fatalf("historical mongo scope = %+v, want exact resource IDs without testee IDs", got)
	}
	ordinary := mongoScopeIDs(ids, config{})
	if len(ordinary.TesteeIDs) != 1 || ordinary.TesteeIDs[0] != 10 {
		t.Fatalf("ordinary mongo scope = %+v, want original testee scope", ordinary)
	}
}

func TestHistoricalAttemptLedgerIsDeletedBeforeCompletionLedger(t *testing.T) {
	items, err := mysqlDeleteItems(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	positions := map[string]int{}
	for i, item := range items {
		positions[item.name] = i
	}
	attempt, attemptOK := positions["seed_backfill_stage_attempt"]
	stage, stageOK := positions["seed_backfill_stage"]
	if !attemptOK || !stageOK || attempt >= stage {
		t.Fatalf("delete order=%v, attempt ledger must be deleted immediately before completion ledger", positions)
	}
}

func TestExecuteRollbackStatisticsRunUsesProtectedScopeAndRequiresSucceeded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v2/statistics/runs" || r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("X-Org-ID") != "42" {
			t.Fatalf("request path=%s auth=%q org=%q", r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("X-Org-ID"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["mode"] != "repair" || body["confirm"] != true || body["reason"] != "historical_batch_rollback:hist-batch" {
			t.Fatalf("body=%v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"id":7,"status":"succeeded","stage":"completed","as_of_date":"2025-01-31"}}`))
	}))
	defer server.Close()
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2025, 1, 31, 0, 0, 0, 0, time.UTC)
	result, err := executeRollbackStatisticsRun(t.Context(), server.Client(), config{statisticsBaseURL: server.URL, statisticsToken: "secret", seedBatchID: "hist-batch"}, 42, "repair", from, to, true)
	if err != nil || result.ID != 7 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestHistoricalDryRunReceiptV2RejectsScopeAndManifestDrift(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollback-receipt.json")
	receipt := historicalDryRunReceipt{
		Version: 2, OperationID: 17, BatchID: "hist-batch", OrgID: 42,
		ScopeHash: "scope-a", ManifestHash: "manifest-a", GeneratedAt: time.Now().UTC(),
	}
	if err := writeHistoricalDryRunReceipt(path, receipt); err != nil {
		t.Fatal(err)
	}
	loaded, err := readHistoricalDryRunReceipt(path)
	if err != nil {
		t.Fatal(err)
	}
	op := historicalRollbackOperation{ID: 17, BatchID: "hist-batch", OrgID: 42, ScopeHash: "scope-a", ManifestHash: "manifest-a"}
	if err := validateHistoricalReceipt(loaded, op, "hist-batch", "manifest-a"); err != nil {
		t.Fatalf("matching receipt should verify: %v", err)
	}
	op.ScopeHash = "scope-b"
	if err := validateHistoricalReceipt(loaded, op, "hist-batch", "manifest-a"); err == nil {
		t.Fatal("changed persisted scope must invalidate receipt")
	}
	op.ScopeHash = "scope-a"
	if err := validateHistoricalReceipt(loaded, op, "hist-batch", "manifest-b"); err == nil {
		t.Fatal("changed manifest must invalidate receipt")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("receipt mode=%o want=600", info.Mode().Perm())
	}
}

func TestHistoricalDryRunReceiptV1RequiresNewDryRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollback-receipt-v1.json")
	if err := os.WriteFile(path, []byte(`{"version":1,"batch_id":"hist-batch"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readHistoricalDryRunReceipt(path); err == nil || !strings.Contains(err.Error(), "receipt v2") {
		t.Fatalf("read v1 err=%v, want rerun guidance", err)
	}
}

func TestHistoricalResourceScopeHashIsDeterministicAndManifestBound(t *testing.T) {
	left := []historicalRollbackResource{
		{Storage: "mongo", ResourceType: "answersheets", ResourceID: "oid:abc"},
		{Storage: "mysql", ResourceType: "assessment", ResourceID: "2"},
		{Storage: "mysql", ResourceType: "assessment", ResourceID: "2"},
	}
	right := []historicalRollbackResource{left[1], left[0]}
	a, err := historicalResourceScopeHash("hist-batch", "manifest-a", left)
	if err != nil {
		t.Fatal(err)
	}
	b, err := historicalResourceScopeHash("hist-batch", "manifest-a", right)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("normalized hashes differ: %s %s", a, b)
	}
	c, err := historicalResourceScopeHash("hist-batch", "manifest-b", right)
	if err != nil {
		t.Fatal(err)
	}
	if a == c {
		t.Fatal("manifest drift must change scope hash")
	}
}

func TestParseMongoDocumentIDKeyRoundTripsSupportedIDs(t *testing.T) {
	ids := []any{"abc", int32(2), int64(3), int(4)}
	for _, id := range ids {
		raw := mongoDocumentIDKey(id)
		got, err := parseMongoDocumentIDKey(raw)
		if err != nil {
			t.Fatalf("parse %q: %v", raw, err)
		}
		if mongoDocumentIDKey(got) != raw {
			t.Fatalf("roundtrip %q -> %#v -> %q", raw, got, mongoDocumentIDKey(got))
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
	statements := mysqlOutboxScopeStatements(config{})
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

func TestMySQLOutboxScopePayloadScanIsExplicitOptIn(t *testing.T) {
	defaultStatements := mysqlOutboxScopeStatements(config{})
	for _, statement := range defaultStatements {
		if strings.Contains(statement.name, "payload_json") {
			t.Fatalf("payload_json statement %q should not be enabled by default", statement.name)
		}
	}

	optInStatements := mysqlOutboxScopeStatements(config{scanEventPayloads: true})
	var outboxPayload bool
	for _, statement := range optInStatements {
		outboxPayload = outboxPayload || statement.name == "mysql outbox ids from payload_json"
	}
	if !outboxPayload {
		t.Fatal("scanEventPayloads should add the outbox payload_json statement")
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

func makeUint64Range(start uint64, count int) []uint64 {
	out := make([]uint64, count)
	for i := range out {
		out[i] = start + uint64(i)
	}
	return out
}
