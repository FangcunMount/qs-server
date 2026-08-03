//go:build integration

package main

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"
)

func TestBatchUpdatesAgainstMySQL(t *testing.T) {
	dsn := os.Getenv("REPAIR_PLAN_TASK_MYSQL_DSN")
	if dsn == "" {
		t.Skip("REPAIR_PLAN_TASK_MYSQL_DSN is required")
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	ctx := context.Background()
	createBatchUpdateTables(t, ctx, db)

	plannedAt := time.Date(2026, 8, 1, 19, 0, 0, 0, shanghai)
	if _, err := db.ExecContext(ctx, `INSERT INTO plan_enrollment (id,org_id,version,status) VALUES (20,1,1,'active'),(21,1,2,'active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO assessment_task (id,enrollment_id,org_id,version,schedule_revision,status,planned_at) VALUES (10,20,1,1,1,'pending',?),(11,21,1,2,1,'pending',?)`, plannedAt, plannedAt.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}

	scheduleBatch := []scheduleCandidate{
		{ID: 10, Version: 1, ScheduleRevision: 1, Status: "pending", PlannedAt: plannedAt},
		{ID: 11, Version: 2, ScheduleRevision: 1, Status: "pending", PlannedAt: plannedAt.AddDate(0, 0, 1)},
	}
	definedAt := plannedAt.Add(-time.Hour)
	inferences := []scheduleInference{
		{Revision: 1, DefinedAt: definedAt},
		{Revision: 2, DefinedAt: definedAt.Add(time.Hour)},
	}
	withCommittedTx(t, db, func(tx *sql.Tx) error {
		return updateScheduleBatch(ctx, tx, config{orgID: 1}, scheduleBatch, inferences)
	})

	dueBatch := []taskState{
		{ID: 10, Version: 2, Status: "pending", PlannedAt: plannedAt, ScheduleRevision: 1, ScheduleDefinedAt: &inferences[0].DefinedAt},
		{ID: 11, Version: 3, Status: "pending", PlannedAt: plannedAt.AddDate(0, 0, 1), ScheduleRevision: 2, ScheduleDefinedAt: &inferences[1].DefinedAt},
	}
	withCommittedTx(t, db, func(tx *sql.Tx) error {
		return updateDueBatch(ctx, tx, config{orgID: 1}, dueBatch)
	})
	dueOne := plannedAt.AddDate(0, 0, 7)
	dueTwo := plannedAt.AddDate(0, 0, 8)
	dueBatch[0].Version, dueBatch[0].DueAt = 3, &dueOne
	dueBatch[1].Version, dueBatch[1].DueAt = 4, &dueTwo
	withCommittedTx(t, db, func(tx *sql.Tx) error {
		return updateMissedBatch(ctx, tx, config{orgID: 1}, dueBatch)
	})

	enrollmentBatch := []enrollmentCloseCandidate{
		{before: enrollmentState{ID: 20, Version: 1, Status: "active"}, terminal: plannedAt.Add(openWindow)},
		{before: enrollmentState{ID: 21, Version: 2, Status: "active"}, terminal: plannedAt.AddDate(0, 0, 1).Add(openWindow)},
	}
	withCommittedTx(t, db, func(tx *sql.Tx) error {
		return updateEnrollmentBatch(ctx, tx, config{orgID: 1}, enrollmentBatch)
	})

	assertBatchUpdateResults(t, ctx, db, plannedAt, inferences)
	assertPartialBatchRollsBack(t, ctx, db, plannedAt)
	assertConfiguredBatchSize(t, ctx, db, plannedAt)
}

func createBatchUpdateTables(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	statements := []string{
		`CREATE TEMPORARY TABLE plan_enrollment (
            id BIGINT UNSIGNED PRIMARY KEY, org_id BIGINT NOT NULL, version INT UNSIGNED NOT NULL,
            status VARCHAR(32) NOT NULL, closed_at DATETIME(3) NULL, deleted_at DATETIME(3) NULL
        ) ENGINE=InnoDB`,
		`CREATE TEMPORARY TABLE assessment_task (
            id BIGINT UNSIGNED PRIMARY KEY, enrollment_id BIGINT UNSIGNED NOT NULL, org_id BIGINT NOT NULL,
            version INT UNSIGNED NOT NULL, schedule_revision INT UNSIGNED NOT NULL,
            schedule_defined_at DATETIME(3) NULL, status VARCHAR(50) NOT NULL, planned_at DATETIME(3) NOT NULL,
            due_at DATETIME(3) NULL, open_at DATETIME(3) NULL, expire_at DATETIME(3) NULL,
            completed_at DATETIME(3) NULL, expired_at DATETIME(3) NULL, canceled_at DATETIME(3) NULL,
            expiration_reason VARCHAR(32) NULL, assessment_id BIGINT UNSIGNED NULL,
            entry_token VARCHAR(255) NULL, entry_url VARCHAR(500) NULL,
            updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3), deleted_at DATETIME(3) NULL
        ) ENGINE=InnoDB`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
}

func withCommittedTx(t *testing.T, db *sql.DB, run func(*sql.Tx) error) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := run(tx); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func assertBatchUpdateResults(t *testing.T, ctx context.Context, db *sql.DB, plannedAt time.Time, inferences []scheduleInference) {
	t.Helper()
	rows, err := db.QueryContext(ctx, `SELECT id,version,schedule_revision,schedule_defined_at,due_at,status,expired_at,expiration_reason FROM assessment_task WHERE id IN (10,11) ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	index := 0
	for rows.Next() {
		var id, version, revision uint64
		var defined, due, expired time.Time
		var status, reason string
		if err := rows.Scan(&id, &version, &revision, &defined, &due, &status, &expired, &reason); err != nil {
			t.Fatal(err)
		}
		wantID := uint64(10 + index)
		wantVersion := uint64(4 + index)
		wantDue := plannedAt.AddDate(0, 0, 7+index)
		wantExpired := plannedAt.AddDate(0, 0, index).Add(openWindow)
		if id != wantID || version != wantVersion || revision != uint64(inferences[index].Revision) || !defined.Equal(inferences[index].DefinedAt) || !due.Equal(wantDue) || status != "expired" || !expired.Equal(wantExpired) || reason != "missed_open_window" {
			t.Fatalf("task result id=%d version=%d revision=%d defined=%s due=%s status=%s expired=%s reason=%s", id, version, revision, defined, due, status, expired, reason)
		}
		index++
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if index != 2 {
		t.Fatalf("task rows=%d", index)
	}
	var closed int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM plan_enrollment WHERE status='closed' AND closed_at IS NOT NULL`).Scan(&closed); err != nil {
		t.Fatal(err)
	}
	if closed != 2 {
		t.Fatalf("closed enrollments=%d", closed)
	}
}

func assertPartialBatchRollsBack(t *testing.T, ctx context.Context, db *sql.DB, plannedAt time.Time) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `INSERT INTO assessment_task (id,enrollment_id,org_id,version,schedule_revision,schedule_defined_at,status,planned_at) VALUES (12,20,1,1,1,?,'pending',?),(13,21,1,1,1,?,'pending',?)`, plannedAt, plannedAt, plannedAt, plannedAt.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
	batch := []taskState{
		{ID: 12, Version: 1, Status: "pending", PlannedAt: plannedAt, ScheduleRevision: 1, ScheduleDefinedAt: &plannedAt},
		{ID: 13, Version: 99, Status: "pending", PlannedAt: plannedAt.AddDate(0, 0, 1), ScheduleRevision: 1, ScheduleDefinedAt: &plannedAt},
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	err = updateDueBatch(ctx, tx, config{orgID: 1}, batch)
	if err == nil || !strings.Contains(err.Error(), "expected=2 changed=1") {
		_ = tx.Rollback()
		t.Fatalf("partial batch err=%v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var changed int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM assessment_task WHERE id IN (12,13) AND due_at IS NOT NULL`).Scan(&changed); err != nil {
		t.Fatal(err)
	}
	if changed != 0 {
		t.Fatalf("partial batch left %d committed rows", changed)
	}
}

func assertConfiguredBatchSize(t *testing.T, ctx context.Context, db *sql.DB, plannedAt time.Time) {
	t.Helper()
	var insert strings.Builder
	insert.WriteString("INSERT INTO assessment_task (id,enrollment_id,org_id,version,schedule_revision,schedule_defined_at,status,planned_at) VALUES ")
	args := make([]any, 0, batchSize*2)
	batch := make([]taskState, batchSize)
	scheduleBatch := make([]scheduleCandidate, batchSize)
	inferences := make([]scheduleInference, batchSize)
	for index := range batch {
		if index > 0 {
			insert.WriteByte(',')
		}
		insert.WriteString("(?,20,1,1,1,NULL,'pending',?)")
		id := uint64(1000 + index)
		itemPlannedAt := plannedAt.Add(time.Duration(index) * time.Millisecond)
		args = append(args, id, itemPlannedAt)
		scheduleBatch[index] = scheduleCandidate{
			ID:               id,
			Version:          1,
			ScheduleRevision: 1,
			Status:           "pending",
			PlannedAt:        itemPlannedAt,
		}
		inferences[index] = scheduleInference{Revision: 1, DefinedAt: plannedAt}
		batch[index] = taskState{
			ID:                id,
			Version:           2,
			Status:            "pending",
			PlannedAt:         itemPlannedAt,
			ScheduleRevision:  1,
			ScheduleDefinedAt: &plannedAt,
		}
	}
	if _, err := db.ExecContext(ctx, insert.String(), args...); err != nil {
		t.Fatal(err)
	}
	withCommittedTx(t, db, func(tx *sql.Tx) error {
		return updateScheduleBatch(ctx, tx, config{orgID: 1}, scheduleBatch, inferences)
	})
	withCommittedTx(t, db, func(tx *sql.Tx) error {
		return updateDueBatch(ctx, tx, config{orgID: 1}, batch)
	})
	var changed int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM assessment_task WHERE id BETWEEN 1000 AND 1499 AND schedule_defined_at IS NOT NULL AND due_at IS NOT NULL AND version=3`).Scan(&changed); err != nil {
		t.Fatal(err)
	}
	if changed != batchSize {
		t.Fatalf("full batch changed=%d want=%d", changed, batchSize)
	}
}
