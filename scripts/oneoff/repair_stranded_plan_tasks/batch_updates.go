package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type enrollmentCloseCandidate struct {
	before   enrollmentState
	terminal time.Time
}

func updateScheduleBatch(ctx context.Context, tx *sql.Tx, cfg config, batch []scheduleCandidate, inferences []scheduleInference) error {
	if len(batch) == 0 {
		return nil
	}
	if len(batch) != len(inferences) {
		return fmt.Errorf("backfill_schedule batch mismatch: candidates=%d inferences=%d", len(batch), len(inferences))
	}

	var query strings.Builder
	args := make([]any, 0, len(batch)*13+1)
	query.WriteString("UPDATE assessment_task SET schedule_revision=CASE id ")
	for index, candidate := range batch {
		query.WriteString("WHEN ? THEN ? ")
		args = append(args, candidate.ID, inferences[index].Revision)
	}
	query.WriteString("ELSE schedule_revision END,schedule_defined_at=CASE id ")
	for index, candidate := range batch {
		query.WriteString("WHEN ? THEN ? ")
		args = append(args, candidate.ID, inferences[index].DefinedAt)
	}
	query.WriteString("ELSE schedule_defined_at END,version=version+1 WHERE org_id=? AND deleted_at IS NULL AND (")
	args = append(args, cfg.orgID)
	for index, candidate := range batch {
		if index > 0 {
			query.WriteString(" OR ")
		}
		query.WriteString("(id=? AND version=? AND schedule_revision=? AND schedule_defined_at IS NULL AND status=? AND planned_at=? AND open_at <=> ? AND completed_at <=> ? AND expired_at <=> ? AND canceled_at <=> ?)")
		args = append(args,
			candidate.ID,
			candidate.Version,
			candidate.ScheduleRevision,
			candidate.Status,
			candidate.PlannedAt,
			nullableValue(candidate.OpenAt),
			nullableValue(candidate.CompletedAt),
			nullableValue(candidate.ExpiredAt),
			nullableValue(candidate.CanceledAt),
		)
	}
	query.WriteByte(')')
	return executeBatchUpdate(ctx, tx, "backfill_schedule", len(batch), query.String(), args)
}

func updateDueBatch(ctx context.Context, tx *sql.Tx, cfg config, batch []taskState) error {
	if len(batch) == 0 {
		return nil
	}

	var query strings.Builder
	args := make([]any, 0, len(batch)*8+1)
	query.WriteString("UPDATE assessment_task SET due_at=CASE id ")
	for _, before := range batch {
		due := before.PlannedAt.In(shanghai).AddDate(0, 0, 7)
		query.WriteString("WHEN ? THEN ? ")
		args = append(args, before.ID, due)
	}
	query.WriteString("ELSE due_at END,version=version+1,updated_at=updated_at WHERE org_id=? AND deleted_at IS NULL AND (")
	args = append(args, cfg.orgID)
	for index, before := range batch {
		if index > 0 {
			query.WriteString(" OR ")
		}
		query.WriteString("(id=? AND version=? AND due_at IS NULL AND status=? AND planned_at=? AND schedule_revision=? AND schedule_defined_at <=> ?)")
		args = append(args,
			before.ID,
			before.Version,
			before.Status,
			before.PlannedAt,
			before.ScheduleRevision,
			nullableValue(before.ScheduleDefinedAt),
		)
	}
	query.WriteByte(')')
	return executeBatchUpdate(ctx, tx, "backfill_due", len(batch), query.String(), args)
}

func updateMissedBatch(ctx context.Context, tx *sql.Tx, cfg config, batch []taskState) error {
	if len(batch) == 0 {
		return nil
	}

	var query strings.Builder
	args := make([]any, 0, len(batch)*8+1)
	query.WriteString("UPDATE assessment_task SET status='expired',expiration_reason='missed_open_window',expired_at=CASE id ")
	for _, before := range batch {
		expiredAt := before.PlannedAt.In(shanghai).Add(openWindow)
		query.WriteString("WHEN ? THEN ? ")
		args = append(args, before.ID, expiredAt)
	}
	query.WriteString("ELSE expired_at END,version=version+1 WHERE org_id=? AND status='pending' AND open_at IS NULL AND expire_at IS NULL AND completed_at IS NULL AND expired_at IS NULL AND canceled_at IS NULL AND assessment_id IS NULL AND COALESCE(entry_token,'')='' AND COALESCE(entry_url,'')='' AND COALESCE(expiration_reason,'')='' AND deleted_at IS NULL AND (")
	args = append(args, cfg.orgID)
	for index, before := range batch {
		if index > 0 {
			query.WriteString(" OR ")
		}
		query.WriteString("(id=? AND version=? AND planned_at=? AND due_at <=> ? AND schedule_revision=? AND schedule_defined_at <=> ?)")
		args = append(args,
			before.ID,
			before.Version,
			before.PlannedAt,
			nullableValue(before.DueAt),
			before.ScheduleRevision,
			nullableValue(before.ScheduleDefinedAt),
		)
	}
	query.WriteByte(')')
	return executeBatchUpdate(ctx, tx, "expire_missed", len(batch), query.String(), args)
}

func updateEnrollmentBatch(ctx context.Context, tx *sql.Tx, cfg config, batch []enrollmentCloseCandidate) error {
	if len(batch) == 0 {
		return nil
	}

	var query strings.Builder
	args := make([]any, 0, len(batch)*4+1)
	query.WriteString("UPDATE plan_enrollment SET status='closed',closed_at=CASE id ")
	for _, candidate := range batch {
		query.WriteString("WHEN ? THEN ? ")
		args = append(args, candidate.before.ID, candidate.terminal)
	}
	query.WriteString("ELSE closed_at END,version=version+1 WHERE org_id=? AND status='active' AND deleted_at IS NULL AND (")
	args = append(args, cfg.orgID)
	for index, candidate := range batch {
		if index > 0 {
			query.WriteString(" OR ")
		}
		query.WriteString("(id=? AND version=?)")
		args = append(args, candidate.before.ID, candidate.before.Version)
	}
	query.WriteString(") AND NOT EXISTS (SELECT 1 FROM assessment_task t WHERE t.enrollment_id=plan_enrollment.id AND t.org_id=plan_enrollment.org_id AND t.deleted_at IS NULL AND t.status NOT IN ('completed','expired','canceled'))")
	return executeBatchUpdate(ctx, tx, "close_enrollments", len(batch), query.String(), args)
}

func executeBatchUpdate(ctx context.Context, tx *sql.Tx, phase string, expected int, query string, args []any) error {
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s rows affected: %w", phase, err)
	}
	if changed != int64(expected) {
		return fmt.Errorf("%s CAS conflict: expected=%d changed=%d", phase, expected, changed)
	}
	return nil
}
