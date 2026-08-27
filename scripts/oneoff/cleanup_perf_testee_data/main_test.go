package main

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
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

func TestTemporaryAssessmentScopeUsesAuthoritativeOriginType(t *testing.T) {
	statements := mysqlScopePopulateStatements()
	if len(statements) == 0 {
		t.Fatal("mysql scope statements are empty")
	}
	first := statements[0]
	if first.name != "temporary assessment ids" {
		t.Fatalf("first scope statement = %q, want temporary assessment ids", first.name)
	}
	if !strings.Contains(first.sql, "a.origin_type = 'adhoc'") {
		t.Fatalf("temporary assessment scope must use assessment.origin_type='adhoc'; sql=%s", first.sql)
	}
	for _, statement := range statements[1:] {
		if strings.Contains(statement.sql, "tmp_cleanup_testee_ids") {
			t.Fatalf("derived scope %q must expand only from temporary assessment ids, not all testee data; sql=%s", statement.name, statement.sql)
		}
		if strings.Contains(statement.sql, "SELECT id FROM tmp_cleanup_assessment_ids") {
			t.Fatalf("report ids are independent from assessment ids and must be discovered by references; sql=%s", statement.sql)
		}
	}
}

func TestTemporaryAssessmentCleanupPreservesTesteePlanAccessAndIAMData(t *testing.T) {
	forbidden := map[string]struct{}{
		"testee": {}, "assessment_task": {}, "plan_enrollment": {}, "clinician_relation": {},
		"assessment_entry_resolve_log": {}, "assessment_entry_intake_log": {},
		"statistics_access_fact": {}, "statistics_access_daily": {}, "statistics_plan_fact": {},
		"statistics_plan_activity_daily": {}, "statistics_plan_fulfillment_daily": {},
		"profile_links": {}, "profiles": {},
	}
	assertAbsent := func(kind string, names []string) {
		t.Helper()
		for _, name := range names {
			if _, exists := forbidden[name]; exists {
				t.Fatalf("%s unexpectedly includes preserved table %s", kind, name)
			}
		}
	}

	countNames := make([]string, 0, len(mysqlCountItems()))
	for _, item := range mysqlCountItems() {
		countNames = append(countNames, item.name)
	}
	assertAbsent("count", countNames)

	backupNames := make([]string, 0, len(mysqlBackupItems()))
	for _, item := range mysqlBackupItems() {
		backupNames = append(backupNames, item.table)
	}
	assertAbsent("backup", backupNames)

	deleteItems, err := mysqlDeleteItems(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	deleteNames := make([]string, 0, len(deleteItems))
	for _, item := range deleteItems {
		deleteNames = append(deleteNames, item.name)
		if strings.Contains(item.stmt, "tmp_cleanup_testee_ids") {
			t.Fatalf("delete %s must not scope by all rows of a testee; sql=%s", item.name, item.stmt)
		}
	}
	assertAbsent("delete", deleteNames)
}

func TestTemporaryAssessmentScopeRejectsPlanTaskLinks(t *testing.T) {
	if err := validateTemporaryAssessmentScopeCounts(0, 0); err != nil {
		t.Fatalf("zero plan-task links should be valid: %v", err)
	}
	if err := validateTemporaryAssessmentScopeCounts(1, 0); err == nil || !strings.Contains(err.Error(), "scope changed") {
		t.Fatalf("invalid origin error = %v, want scope-changed error", err)
	}
	err := validateTemporaryAssessmentScopeCounts(0, 1)
	if err == nil || !strings.Contains(err.Error(), "preserve plan data") {
		t.Fatalf("linked plan task error = %v, want fail-closed preservation error", err)
	}
}

func TestMongoAnswerSheetScopeUsesOnlyScopedAnswerSheetIDs(t *testing.T) {
	filters := answersheetFilters(scopeIDs{TesteeIDs: []uint64{7}, AnswerSheetIDs: []uint64{11}})
	if len(filters) != 1 {
		t.Fatalf("answersheet filter count = %d, want 1", len(filters))
	}
	if _, exists := filters[0]["testee_id"]; exists {
		t.Fatalf("answersheet filters must not select every answer sheet for a testee: %#v", filters[0])
	}
	if _, exists := filters[0]["domain_id"]; !exists {
		t.Fatalf("answersheet filters must use scoped domain ids: %#v", filters[0])
	}
}

func TestCleanupCoversMigratedInterpretationLedgersButPreservesGlobalCheckpoint(t *testing.T) {
	want := map[string]bool{
		"interpretation_admission_failure":    false,
		"interpretation_attention_projection": false,
	}
	assertCoverage := func(kind string, names []string) {
		t.Helper()
		covered := make(map[string]bool, len(want))
		for _, name := range names {
			if name == "interpretation_catalog_audit_checkpoint" {
				t.Fatalf("%s must preserve the global catalog audit checkpoint", kind)
			}
			if _, ok := want[name]; ok {
				covered[name] = true
			}
		}
		for name := range want {
			if !covered[name] {
				t.Fatalf("%s does not cover migrated table %s", kind, name)
			}
		}
	}

	countNames := make([]string, 0, len(mysqlCountItems()))
	for _, item := range mysqlCountItems() {
		countNames = append(countNames, item.name)
	}
	assertCoverage("count", countNames)

	backupNames := make([]string, 0, len(mysqlBackupItems()))
	for _, item := range mysqlBackupItems() {
		backupNames = append(backupNames, item.table)
	}
	assertCoverage("backup", backupNames)

	deleteItems, err := mysqlDeleteItems(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	deleteNames := make([]string, 0, len(deleteItems))
	for _, item := range deleteItems {
		deleteNames = append(deleteNames, item.name)
	}
	assertCoverage("delete", deleteNames)
}

func TestValidateBackupSuffixChecksGeneratedMySQLIdentifiers(t *testing.T) {
	if err := validateBackupSuffix("seeddata_dup_20260812_v1"); err != nil {
		t.Fatalf("documented backup suffix should be valid: %v", err)
	}
	if err := validateBackupSuffix(strings.Repeat("x", mysqlIdentifierMaxLength)); err == nil || !strings.Contains(err.Error(), "exceeds MySQL identifier limit") {
		t.Fatalf("oversized backup suffix error = %v, want MySQL identifier limit error", err)
	}
	for _, item := range mysqlBackupItems() {
		name, err := mysqlBackupTableName(item.table, "seeddata_dup_20260812_v1")
		if err != nil {
			t.Fatalf("backup table name for %s: %v", item.table, err)
		}
		if len(name) > mysqlIdentifierMaxLength {
			t.Fatalf("backup table name %q length=%d, want <=%d", name, len(name), mysqlIdentifierMaxLength)
		}
	}
}

func TestBackupMySQLTableOmitsGeneratedColumnsFromInsert(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	item := mysqlBackupItem{
		table:     "assessment",
		selectSQL: `SELECT a.* FROM assessment a`,
	}
	backupTable := "cbpt_assessment_s812v3"
	mock.ExpectExec(regexp.QuoteMeta("CREATE TABLE IF NOT EXISTS `cbpt_assessment_s812v3` LIKE `assessment`")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta(mysqlBackupColumnsQuery)).
		WithArgs("assessment").
		WillReturnRows(sqlmock.NewRows([]string{"column_name", "generation_expression"}).
			AddRow("id", "").
			AddRow("status", "").
			AddRow("active_slot", "case when (`status` = _utf8mb4'active') then 1 else NULL end"))
	mock.ExpectExec(regexp.QuoteMeta(
		"INSERT IGNORE INTO `cbpt_assessment_s812v3` (`id`, `status`) " +
			"SELECT `backup_source`.`id`, `backup_source`.`status` " +
			"FROM (SELECT a.* FROM assessment a) AS `backup_source`",
	)).WillReturnResult(sqlmock.NewResult(0, 1))

	if err := backupMySQLTable(ctx, conn, item, backupTable); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMySQLBackupInsertSQLRejectsUnsafeColumnName(t *testing.T) {
	_, err := mysqlBackupInsertSQL("cbpt_assessment_s812v3", "SELECT a.* FROM assessment a", []string{"id`, active_slot"})
	if err == nil || !strings.Contains(err.Error(), "unsafe MySQL identifier") {
		t.Fatalf("error = %v, want unsafe identifier error", err)
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
		GenerationIDs:  []uint64{25_000},
		TesteeIDs:      []uint64{30_000},
	}

	filters := mongoOutboxFilters(ids)
	if len(filters) < 6 {
		t.Fatalf("filter count = %d, want chunked filters", len(filters))
	}
	foundGeneration := false
	for _, filter := range filters {
		if filter["aggregate_type"] == "ReportGeneration" {
			foundGeneration = true
		}
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
	if !foundGeneration {
		t.Fatal("mongo outbox scope must include ReportGeneration aggregate ids")
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
		"statistics_assessment_fact",
		"interpretation_attention_projection",
		"interpretation_admission_failure",
		"assessment_score",
		"evaluation_outcome",
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

	for _, preserved := range []string{"testee", "assessment_task", "clinician_relation"} {
		if _, ok := mysqlChunkedDeleteSpecFor(preserved); ok {
			t.Fatalf("preserved table %s must not have a cleanup delete path", preserved)
		}
	}
}

func TestMySQLChunkedDeleteUsesStagingTablesForMultiSourceTables(t *testing.T) {
	for _, name := range []string{
		"statistics_assessment_fact",
		"interpretation_attention_projection",
		"interpretation_admission_failure",
		"assessment_score",
	} {
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

func TestAttentionProjectionChunkedDeleteUsesEventIDPrimaryKey(t *testing.T) {
	spec, ok := mysqlChunkedDeleteSpecFor("interpretation_attention_projection")
	if !ok {
		t.Fatal("interpretation_attention_projection should use chunked delete")
	}
	for label, sql := range map[string]string{
		"fill":   spec.fillBatchTable,
		"delete": spec.deleteBatch,
		"prune":  spec.pruneStagingTable,
	} {
		if !strings.Contains(sql, "event_id") {
			t.Fatalf("%s SQL should use event_id; sql=%s", label, sql)
		}
	}
	if strings.Contains(spec.deleteBatch, "assessment_id") || strings.Contains(spec.deleteBatch, "report_id") {
		t.Fatalf("delete batch must join only materialized event_id values; sql=%s", spec.deleteBatch)
	}
}

func TestAttentionProjectionMaterializationKeepsTemporaryIDPrimaryKeysUsable(t *testing.T) {
	var query string
	for _, item := range mysqlScopedDeleteIDMaterializationStatements() {
		if item.name == "interpretation_attention_projection event ids" {
			query = item.sql
			break
		}
	}
	if query == "" {
		t.Fatal("attention projection event-id materialization is missing")
	}
	for _, want := range []string{
		"a.id = CAST(p.assessment_id AS UNSIGNED)",
		"r.id = CAST(p.report_id AS UNSIGNED)",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("materialization query should preserve temporary-table PK lookup %q; sql=%s", want, query)
		}
	}
	for _, unsafe := range []string{"CAST(a.id", "CAST(r.id", "DELETE"} {
		if strings.Contains(query, unsafe) {
			t.Fatalf("materialization query should not contain %q; sql=%s", unsafe, query)
		}
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
