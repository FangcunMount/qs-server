package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRollbackWithCausePreservesPrimaryAndRollbackErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	primaryErr := errors.New("write failed")
	rollbackErr := errors.New("rollback failed")
	mock.ExpectRollback().WillReturnError(rollbackErr)

	got := rollbackWithCause(tx, primaryErr)
	if !errors.Is(got, primaryErr) || !errors.Is(got, rollbackErr) {
		t.Fatalf("rollback error=%v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBatchUpdatesUseOneStatementPerPhase(t *testing.T) {
	plannedAt := time.Date(2026, 8, 1, 19, 0, 0, 0, shanghai)
	definedAt := plannedAt.Add(-time.Hour)
	dueAt := plannedAt.AddDate(0, 0, 7)
	tasks := []taskState{
		{ID: 10, Version: 3, Status: "pending", PlannedAt: plannedAt, ScheduleRevision: 1, ScheduleDefinedAt: &definedAt},
		{ID: 11, Version: 5, Status: "pending", PlannedAt: plannedAt.AddDate(0, 0, 1), ScheduleRevision: 2, ScheduleDefinedAt: &definedAt},
	}
	scheduleCandidates := []scheduleCandidate{
		{ID: 10, Version: 3, ScheduleRevision: 1, Status: "pending", PlannedAt: plannedAt},
		{ID: 11, Version: 5, ScheduleRevision: 1, Status: "pending", PlannedAt: plannedAt.AddDate(0, 0, 1)},
	}
	scheduleInferences := []scheduleInference{
		{Revision: 1, DefinedAt: definedAt},
		{Revision: 2, DefinedAt: definedAt.Add(time.Hour)},
	}
	enrollments := []enrollmentCloseCandidate{
		{before: enrollmentState{ID: 20, Version: 2, Status: "active"}, terminal: dueAt},
		{before: enrollmentState{ID: 21, Version: 4, Status: "active"}, terminal: dueAt.Add(time.Hour)},
	}

	tests := []struct {
		name    string
		pattern string
		run     func(context.Context, *sql.Tx) error
	}{
		{
			name:    "backfill schedule",
			pattern: `UPDATE assessment_task SET schedule_revision=CASE id.*schedule_defined_at IS NULL.*open_at <=> \?.*canceled_at <=> \?`,
			run: func(ctx context.Context, tx *sql.Tx) error {
				return updateScheduleBatch(ctx, tx, config{orgID: 1}, scheduleCandidates, scheduleInferences)
			},
		},
		{
			name:    "backfill due",
			pattern: `UPDATE assessment_task SET due_at=CASE id.*due_at IS NULL.*schedule_defined_at <=> \?`,
			run: func(ctx context.Context, tx *sql.Tx) error {
				return updateDueBatch(ctx, tx, config{orgID: 1}, tasks)
			},
		},
		{
			name:    "expire missed",
			pattern: `UPDATE assessment_task SET status='expired',expiration_reason='missed_open_window',expired_at=CASE id.*status='pending'.*assessment_id IS NULL.*schedule_defined_at <=> \?`,
			run: func(ctx context.Context, tx *sql.Tx) error {
				return updateMissedBatch(ctx, tx, config{orgID: 1}, tasks)
			},
		},
		{
			name:    "close enrollments",
			pattern: `UPDATE plan_enrollment SET status='closed',closed_at=CASE id.*status='active'.*NOT EXISTS`,
			run: func(ctx context.Context, tx *sql.Tx) error {
				return updateEnrollmentBatch(ctx, tx, config{orgID: 1}, enrollments)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			mock.ExpectBegin()
			tx, err := db.Begin()
			if err != nil {
				t.Fatal(err)
			}
			mock.ExpectExec(tt.pattern).WillReturnResult(sqlmock.NewResult(0, 2))
			mock.ExpectCommit()
			if err := tt.run(context.Background(), tx); err != nil {
				t.Fatal(err)
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDueBatchSupportsConfiguredBatchSize(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	plannedAt := time.Date(2026, 8, 1, 19, 0, 0, 0, shanghai)
	batch := make([]taskState, batchSize)
	for index := range batch {
		batch[index] = taskState{
			ID:               uint64(index + 1),
			Version:          uint64(index + 1),
			Status:           "pending",
			PlannedAt:        plannedAt.AddDate(0, 0, index),
			ScheduleRevision: 1,
		}
	}
	mock.ExpectExec(`UPDATE assessment_task SET due_at=CASE id`).WillReturnResult(sqlmock.NewResult(0, batchSize))
	mock.ExpectCommit()
	if err := updateDueBatch(context.Background(), tx, config{orgID: 1}, batch); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestScheduleBatchRejectsMismatchedInferenceCount(t *testing.T) {
	err := updateScheduleBatch(context.Background(), nil, config{orgID: 1}, []scheduleCandidate{{ID: 1}}, nil)
	if err == nil || !strings.Contains(err.Error(), "candidates=1 inferences=0") {
		t.Fatalf("err=%v", err)
	}
}

func TestBatchUpdateRejectsPartialCAS(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	plannedAt := time.Date(2026, 8, 1, 19, 0, 0, 0, shanghai)
	batch := []taskState{
		{ID: 10, Version: 3, Status: "pending", PlannedAt: plannedAt, ScheduleRevision: 1},
		{ID: 11, Version: 5, Status: "pending", PlannedAt: plannedAt.AddDate(0, 0, 1), ScheduleRevision: 1},
	}
	mock.ExpectExec(`UPDATE assessment_task SET due_at=CASE id`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	err = updateDueBatch(context.Background(), tx, config{orgID: 1}, batch)
	if err == nil || !strings.Contains(err.Error(), "backfill_due CAS conflict: expected=2 changed=1") {
		t.Fatalf("err=%v", err)
	}
	if rollbackErr := tx.Rollback(); rollbackErr != nil {
		t.Fatal(rollbackErr)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func requiredArgs(mode string) []string {
	return []string{"--mode", mode, "--mysql-dsn", "user:pass@tcp(localhost:3306)/qs?parseTime=true&loc=Asia%2FShanghai", "--org-id", "1", "--cutoff-at", "2026-08-03T00:00:00+08:00", "--checkpoint-file", filepath.Join("tmp", "checkpoint.json"), "--audit-file", filepath.Join("tmp", "audit.jsonl.gz")}
}

func TestParseConfigDefaultsToReadOnlyAudit(t *testing.T) {
	cfg, err := parseConfig(requiredArgs("audit"), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.mode != "audit" || cfg.confirm || cfg.orgID != 1 {
		t.Fatalf("config=%+v", cfg)
	}
	if cfg.cutoff.Location().String() != "Asia/Shanghai" {
		t.Fatalf("cutoff location=%s", cfg.cutoff.Location())
	}
}

func TestParseConfigRequiresConfirmationForWrites(t *testing.T) {
	for _, mode := range []string{"apply", "rollback"} {
		if _, err := parseConfig(requiredArgs(mode), &bytes.Buffer{}); err == nil {
			t.Fatalf("%s without --confirm must fail", mode)
		}
		args := append(requiredArgs(mode), "--confirm")
		if _, err := parseConfig(args, &bytes.Buffer{}); err != nil {
			t.Fatalf("%s with confirmation: %v", mode, err)
		}
	}
}

func TestParseCutoffUsesShanghaiForLocalTimestamp(t *testing.T) {
	got, err := parseCutoff("2026-08-03 12:30:00.000")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 8, 3, 12, 30, 0, 0, shanghai)
	if !got.Equal(want) {
		t.Fatalf("cutoff=%s want=%s", got, want)
	}
}

func TestTaskBackupChecksumChangesWithAfterState(t *testing.T) {
	cfg := config{orgID: 1, cutoff: time.Now()}
	cp := &checkpoint{RunID: "run"}
	before := []byte(`{"id":1,"version":1}`)
	first := baseAudit(cfg, cp, "task", "backfill_due", 1, before, []byte(`{"id":1,"version":2}`), nil)
	second := baseAudit(cfg, cp, "task", "backfill_due", 1, before, []byte(`{"id":1,"version":3}`), nil)
	if first.Checksum == second.Checksum {
		t.Fatal("checksum must cover before and after state")
	}
}

func TestNormalizeDSNRequiresShanghaiTimeParsing(t *testing.T) {
	for _, dsn := range []string{
		"user:pass@tcp(localhost:3306)/qs?loc=Asia%2FShanghai",
		"user:pass@tcp(localhost:3306)/qs?parseTime=true",
	} {
		if _, err := normalizeDSN(dsn); err == nil {
			t.Fatalf("unsafe DSN must fail: %s", dsn)
		}
	}
	normalized, err := normalizeDSN("user:pass@tcp(localhost:3306)/qs?parseTime=true&loc=Asia%2FShanghai")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(normalized, "time_zone=%27%2B08%3A00%27") {
		t.Fatalf("normalized DSN does not pin session time zone: %s", normalized)
	}
}

func TestRollbackTaskSkipsDurableAuditRecordWhoseTransactionDidNotCommit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	planned := time.Date(2026, 8, 1, 19, 0, 0, 0, shanghai)
	definedAt := planned.Add(-time.Hour)
	before := taskState{ID: 10, Version: 3, Status: "pending", PlannedAt: planned, ScheduleRevision: 1, ScheduleDefinedAt: &definedAt}
	due := planned.AddDate(0, 0, 7)
	after := before
	after.Version = 4
	after.DueAt = &due

	mock.ExpectExec(regexp.QuoteMeta("UPDATE assessment_task SET")).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id,version,status,planned_at,due_at,schedule_revision,schedule_defined_at,open_at,expire_at,completed_at,expired_at,canceled_at,expiration_reason,assessment_id,COALESCE(entry_token,''),COALESCE(entry_url,'') FROM assessment_task WHERE org_id=? AND id=? AND deleted_at IS NULL")).
		WithArgs(int64(1), uint64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "version", "status", "planned_at", "due_at", "schedule_revision", "schedule_defined_at", "open_at", "expire_at", "completed_at", "expired_at", "canceled_at", "expiration_reason", "assessment_id", "entry_token", "entry_url"}).
			AddRow(10, 3, "pending", planned, nil, 1, definedAt, nil, nil, nil, nil, nil, nil, nil, "", ""))

	changed, err := rollbackTask(context.Background(), db, config{orgID: 1}, before, after)
	if err != nil || changed {
		t.Fatalf("uncommitted audited row should be skipped: changed=%v err=%v", changed, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRollbackTaskScheduleStopsWhenRowChangedAfterRepair(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	definedAt := time.Date(2026, 8, 1, 10, 0, 0, 0, shanghai)
	before := taskScheduleState{ID: 10, Version: 3, ScheduleRevision: 1}
	after := taskScheduleState{ID: 10, Version: 4, ScheduleRevision: 2, ScheduleDefinedAt: &definedAt}

	mock.ExpectExec(regexp.QuoteMeta("UPDATE assessment_task SET schedule_revision=?,schedule_defined_at=?,version=?")).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT id,version,schedule_revision,schedule_defined_at FROM assessment_task WHERE org_id=? AND id=? AND deleted_at IS NULL")).
		WithArgs(int64(1), uint64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "version", "schedule_revision", "schedule_defined_at"}).AddRow(10, 5, 3, definedAt.Add(time.Hour)))

	changed, err := rollbackTaskSchedule(context.Background(), db, config{orgID: 1}, before, after)
	if err == nil || changed || !strings.Contains(err.Error(), "changed after repair") {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRollbackPreflightRefusesPublishedImmutableScheduleFacts(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	records := []auditRecord{{Kind: "task_schedule", After: json.RawMessage(`{"id":10}`)}}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COUNT(*) FROM statistics_plan_fact WHERE org_id=? AND task_id IN (?) AND fact_type IN ('task_schedule_defined','task_schedule_terminal')")).
		WithArgs(int64(1), uint64(10)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	err = ensureScheduleFactsNotPublished(context.Background(), db, 1, records)
	if err == nil || !strings.Contains(err.Error(), "rollback refused") {
		t.Fatalf("err=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
