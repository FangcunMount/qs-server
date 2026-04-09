package main

import (
	"strings"
	"testing"
	"time"
)

func TestNewPlanQuestionnaireVersionMismatchError(t *testing.T) {
	err := newPlanQuestionnaireVersionMismatchError("SAS-TEST", "QNR-001", "1.0.1", "6.0.1")
	if err == nil {
		t.Fatal("expected mismatch error")
	}

	msg := err.Error()
	for _, expected := range []string{
		"scale_code=SAS-TEST",
		"questionnaire_code=QNR-001",
		"scale_questionnaire_version=1.0.1",
		"loaded_questionnaire_version=6.0.1",
		"scale:sas-test",
		"<cache.namespace>:scale:sas-test",
	} {
		if !strings.Contains(msg, expected) {
			t.Fatalf("expected error message to contain %q, got %q", expected, msg)
		}
	}
}

func TestNewExplicitPlanZeroCreatedAtError(t *testing.T) {
	err := newExplicitPlanZeroCreatedAtError("614210295354634798")
	if err == nil {
		t.Fatal("expected explicit created_at error")
	}

	msg := err.Error()
	for _, expected := range []string{
		"explicit plan backfill requires non-zero created_at",
		"testee_id=614210295354634798",
		"--plan-testee-ids",
		"/api/v1/testees/614210295354634798",
		"testee:info:614210295354634798",
		"<cache.namespace>:testee:info:614210295354634798",
	} {
		if !strings.Contains(msg, expected) {
			t.Fatalf("expected error message to contain %q, got %q", expected, msg)
		}
	}
}

func TestPlanStartDateFromAuditTimes(t *testing.T) {
	now := time.Date(2026, 4, 8, 10, 0, 0, 0, time.UTC)
	createdAt := time.Date(2026, 4, 1, 9, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 4, 5, 9, 0, 0, 0, time.UTC)

	date, source, err := planStartDateFromAuditTimes(createdAt, updatedAt, now)
	if err != nil {
		t.Fatalf("unexpected error for created_at: %v", err)
	}
	if source != "created_at" || date != "2026-04-01" {
		t.Fatalf("unexpected created_at fallback result: date=%s source=%s", date, source)
	}

	date, source, err = planStartDateFromAuditTimes(time.Time{}, updatedAt, now)
	if err != nil {
		t.Fatalf("unexpected error for updated_at fallback: %v", err)
	}
	if source != "updated_at" || date != "2026-04-05" {
		t.Fatalf("unexpected updated_at fallback result: date=%s source=%s", date, source)
	}

	date, source, err = planStartDateFromAuditTimes(time.Time{}, time.Time{}, now)
	if err != nil {
		t.Fatalf("unexpected error for now fallback: %v", err)
	}
	if source != "now" || date != "2026-04-08" {
		t.Fatalf("unexpected now fallback result: date=%s source=%s", date, source)
	}
}

func TestNormalizePlanWorkers(t *testing.T) {
	tests := []struct {
		name      string
		workers   int
		testeeCnt int
		expected  int
	}{
		{name: "default to one", workers: 0, testeeCnt: 10, expected: 1},
		{name: "cap by testee count", workers: 8, testeeCnt: 3, expected: 3},
		{name: "keep explicit worker count", workers: 4, testeeCnt: 10, expected: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizePlanWorkers(tt.workers, tt.testeeCnt); got != tt.expected {
				t.Fatalf("normalizePlanWorkers(%d, %d)=%d, want=%d", tt.workers, tt.testeeCnt, got, tt.expected)
			}
		})
	}
}

func TestNormalizePlanExpireRate(t *testing.T) {
	tests := []struct {
		name     string
		input    float64
		expected float64
	}{
		{name: "negative to zero", input: -0.2, expected: 0},
		{name: "keep middle value", input: 0.35, expected: 0.35},
		{name: "cap at one", input: 1.5, expected: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizePlanExpireRate(tt.input); got != tt.expected {
				t.Fatalf("normalizePlanExpireRate(%v)=%v, want=%v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestShouldExpirePlanTask(t *testing.T) {
	task := TaskResponse{ID: "614186929759466030"}

	if shouldExpirePlanTask(task, 0) {
		t.Fatal("expected zero expire rate to never expire")
	}
	if !shouldExpirePlanTask(task, 1) {
		t.Fatal("expected full expire rate to always expire")
	}
	if got1, got2 := shouldExpirePlanTask(task, 0.2), shouldExpirePlanTask(task, 0.2); got1 != got2 {
		t.Fatalf("expected deterministic expire decision, got %v and %v", got1, got2)
	}
}

func TestApplyTesteeLimitToIDs(t *testing.T) {
	ids := []string{"1001", "1002", "1003"}

	if got := applyTesteeLimitToIDs(ids, 0); len(got) != 3 {
		t.Fatalf("expected no limit to keep all ids, got %v", got)
	}
	if got := applyTesteeLimitToIDs(ids, 2); len(got) != 2 || got[0] != "1001" || got[1] != "1002" {
		t.Fatalf("expected limit to keep first two ids, got %v", got)
	}
	if got := applyTesteeLimitToIDs(ids, 5); len(got) != 3 {
		t.Fatalf("expected large limit to keep all ids, got %v", got)
	}
}

func TestSummarizePlanTaskStatuses(t *testing.T) {
	stats := summarizePlanTaskStatuses([]TaskResponse{
		{Status: "pending"},
		{Status: "opened"},
		{Status: "completed"},
		{Status: "expired"},
		{Status: "canceled"},
		{Status: "weird"},
	})

	if stats.Total != 6 {
		t.Fatalf("expected total=6, got %d", stats.Total)
	}
	if stats.Pending != 1 || stats.Opened != 1 || stats.Completed != 1 || stats.Expired != 1 || stats.Canceled != 1 || stats.Unknown != 1 {
		t.Fatalf("unexpected task stats: %+v", stats)
	}
}
