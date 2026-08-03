package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type scheduleCandidate struct {
	ID               uint64
	Version          uint64
	ScheduleRevision uint32
	Status           string
	PlannedAt        time.Time
	OpenAt           *time.Time
	CompletedAt      *time.Time
	ExpiredAt        *time.Time
	CanceledAt       *time.Time
	SourceCreatedAt  time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
	LegacyPlannedAt  *time.Time
	LegacyCompleted  *time.Time
	LegacyExpired    *time.Time
	LegacyCanceled   *time.Time
}

type scheduleInference struct {
	Revision   uint32     `json:"revision"`
	DefinedAt  time.Time  `json:"defined_at,omitempty"`
	Reason     string     `json:"inference_reason"`
	Evidence   string     `json:"inference_evidence,omitempty"`
	LowerBound *time.Time `json:"lower_bound,omitempty"`
	UpperBound *time.Time `json:"upper_bound,omitempty"`
	Ambiguity  string     `json:"ambiguity,omitempty"`
}

type taskScheduleState struct {
	ID                uint64     `json:"id"`
	Version           uint64     `json:"version"`
	ScheduleRevision  uint32     `json:"schedule_revision"`
	ScheduleDefinedAt *time.Time `json:"schedule_defined_at,omitempty"`
}

type scheduleTerminalEvidence struct {
	kind string
	at   *time.Time
}

func loadScheduleCandidatesPage(ctx context.Context, db *sql.DB, orgID int64, lastID uint64) ([]scheduleCandidate, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT t.id,t.version,t.schedule_revision,t.status,t.planned_at,
		       t.open_at,t.completed_at,t.expired_at,t.canceled_at,
		       COALESCE(t.business_created_at,t.created_at),t.created_at,t.updated_at,
		       MAX(CASE WHEN fact.fact_type='task_created' THEN fact.planned_at END),
		       MAX(CASE WHEN fact.fact_type='task_completed' THEN fact.occurred_at END),
		       MAX(CASE WHEN fact.fact_type='task_expired' THEN fact.occurred_at END),
		       MAX(CASE WHEN fact.fact_type='task_canceled' THEN fact.occurred_at END)
		FROM assessment_task t
		LEFT JOIN statistics_plan_fact fact ON fact.org_id=t.org_id AND fact.task_id=t.id
		 AND fact.fact_type IN ('task_created','task_completed','task_expired','task_canceled')
		WHERE t.org_id=? AND t.deleted_at IS NULL AND t.schedule_defined_at IS NULL AND t.id>?
		GROUP BY t.id,t.version,t.schedule_revision,t.status,t.planned_at,t.open_at,t.completed_at,t.expired_at,t.canceled_at,
		         t.business_created_at,t.created_at,t.updated_at
		ORDER BY t.id LIMIT ?`, orgID, lastID, batchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	batch := make([]scheduleCandidate, 0, batchSize)
	for rows.Next() {
		var item scheduleCandidate
		var openAt, completedAt, expiredAt, canceledAt sql.NullTime
		var legacyPlanned, legacyCompleted, legacyExpired, legacyCanceled sql.NullTime
		if err := rows.Scan(
			&item.ID, &item.Version, &item.ScheduleRevision, &item.Status, &item.PlannedAt,
			&openAt, &completedAt, &expiredAt, &canceledAt,
			&item.SourceCreatedAt, &item.CreatedAt, &item.UpdatedAt,
			&legacyPlanned, &legacyCompleted, &legacyExpired, &legacyCanceled,
		); err != nil {
			return nil, err
		}
		item.OpenAt = nullTime(openAt)
		item.CompletedAt = nullTime(completedAt)
		item.ExpiredAt = nullTime(expiredAt)
		item.CanceledAt = nullTime(canceledAt)
		item.LegacyPlannedAt = nullTime(legacyPlanned)
		item.LegacyCompleted = nullTime(legacyCompleted)
		item.LegacyExpired = nullTime(legacyExpired)
		item.LegacyCanceled = nullTime(legacyCanceled)
		batch = append(batch, item)
	}
	return batch, rows.Err()
}

func inferSchedule(candidate scheduleCandidate) scheduleInference {
	if problem := validateCurrentLifecycle(candidate); problem != "" {
		return scheduleInference{Ambiguity: problem}
	}
	if candidate.LegacyCompleted != nil && candidate.Status != "completed" {
		return scheduleInference{
			LowerBound: candidate.LegacyCompleted,
			Ambiguity:  "legacy task_completed fact exists but current task is no longer completed",
		}
	}

	plannedChanged := candidate.LegacyPlannedAt != nil && !candidate.PlannedAt.Equal(*candidate.LegacyPlannedAt)
	oldTerminals := make([]scheduleTerminalEvidence, 0, 2)
	if candidate.LegacyExpired != nil && candidate.Status != "expired" {
		oldTerminals = append(oldTerminals, scheduleTerminalEvidence{kind: "expired", at: candidate.LegacyExpired})
	}
	if candidate.LegacyCanceled != nil && candidate.Status != "canceled" {
		oldTerminals = append(oldTerminals, scheduleTerminalEvidence{kind: "canceled", at: candidate.LegacyCanceled})
	}
	if !plannedChanged && len(oldTerminals) == 0 {
		return scheduleInference{Revision: 1, DefinedAt: candidate.SourceCreatedAt, Reason: "original_schedule"}
	}

	inference := scheduleInference{Revision: 2, Reason: "collapsed_legacy_revisions"}
	switch {
	case len(oldTerminals) > 1:
		inference.Evidence = "multiple_legacy_terminals"
	case len(oldTerminals) == 1 && plannedChanged:
		inference.Evidence = "planned_changed_after_legacy_terminal"
	case len(oldTerminals) == 1:
		inference.Evidence = "legacy_" + oldTerminals[0].kind + "_terminal"
	default:
		inference.Evidence = "planned_at_changed"
	}

	currentFirst := earliestTime(candidate.OpenAt, candidate.CompletedAt, candidate.ExpiredAt, candidate.CanceledAt)
	if len(oldTerminals) > 0 {
		oldTerminal := latestTerminal(oldTerminals)
		inference.LowerBound = oldTerminal
		definedAt := oldTerminal.Add(time.Millisecond)
		if currentFirst != nil {
			inference.UpperBound = currentFirst
			if !definedAt.Before(*currentFirst) {
				inference.Ambiguity = "no legal millisecond boundary between legacy terminal and current lifecycle"
				return inference
			}
		} else if candidate.Status == "pending" {
			inference.UpperBound = &candidate.UpdatedAt
			if definedAt.After(candidate.UpdatedAt) {
				inference.Ambiguity = "legacy terminal occurs after the pending schedule update"
				return inference
			}
		}
		inference.DefinedAt = definedAt
		return inference
	}

	if currentFirst != nil {
		inference.UpperBound = currentFirst
		definedAt := currentFirst.Add(-time.Millisecond)
		if definedAt.Before(candidate.SourceCreatedAt) {
			inference.LowerBound = &candidate.SourceCreatedAt
			inference.Ambiguity = "current lifecycle leaves no schedule boundary at or after task creation"
			return inference
		}
		inference.DefinedAt = definedAt
		return inference
	}
	if candidate.UpdatedAt.Before(candidate.SourceCreatedAt) {
		inference.LowerBound = &candidate.SourceCreatedAt
		inference.Ambiguity = "updated_at precedes task creation"
		return inference
	}
	inference.DefinedAt = candidate.UpdatedAt
	return inference
}

func validateCurrentLifecycle(candidate scheduleCandidate) string {
	terminalCount := 0
	for _, value := range []*time.Time{candidate.CompletedAt, candidate.ExpiredAt, candidate.CanceledAt} {
		if value != nil {
			terminalCount++
		}
	}
	if terminalCount > 1 {
		return "multiple current terminal timestamps"
	}
	switch candidate.Status {
	case "pending":
		if candidate.OpenAt != nil || terminalCount != 0 {
			return "pending task contains current lifecycle timestamps"
		}
	case "opened":
		if candidate.OpenAt == nil || terminalCount != 0 {
			return "opened task lifecycle fields are inconsistent"
		}
	case "completed":
		if candidate.CompletedAt == nil || terminalCount != 1 {
			return "completed task lifecycle fields are inconsistent"
		}
	case "expired":
		if candidate.ExpiredAt == nil || terminalCount != 1 {
			return "expired task lifecycle fields are inconsistent"
		}
	case "canceled":
		if candidate.CanceledAt == nil || terminalCount != 1 {
			return "canceled task lifecycle fields are inconsistent"
		}
	default:
		return fmt.Sprintf("unknown task status %q", candidate.Status)
	}
	if candidate.OpenAt != nil {
		for _, terminal := range []*time.Time{candidate.CompletedAt, candidate.ExpiredAt, candidate.CanceledAt} {
			if terminal != nil && terminal.Before(*candidate.OpenAt) {
				return "terminal timestamp precedes open_at"
			}
		}
	}
	return ""
}

func earliestTime(values ...*time.Time) *time.Time {
	var result *time.Time
	for _, value := range values {
		if value != nil && (result == nil || value.Before(*result)) {
			copyValue := *value
			result = &copyValue
		}
	}
	return result
}

func latestTerminal(values []scheduleTerminalEvidence) *time.Time {
	var result *time.Time
	for _, value := range values {
		if value.at != nil && (result == nil || value.at.After(*result)) {
			copyValue := *value.at
			result = &copyValue
		}
	}
	return result
}
