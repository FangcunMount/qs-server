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
