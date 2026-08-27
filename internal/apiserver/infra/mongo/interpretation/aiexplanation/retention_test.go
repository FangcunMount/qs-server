package aiexplanation

import (
	"testing"
	"time"
)

func TestRetentionPolicyRejectsPartialConfiguration(t *testing.T) {
	if err := (RetentionPolicy{}).Validate(); err != nil {
		t.Fatalf("disabled retention policy: %v", err)
	}
	partial := RetentionPolicy{Version: "policy-v1", ParticipantRecordRetention: time.Hour}
	if err := partial.Validate(); err == nil {
		t.Fatal("partially configured retention policy must fail closed")
	}
	complete := RetentionPolicy{
		Version: "policy-v1", ParticipantRecordRetention: time.Hour,
		PromptEvaluationRetention: 2 * time.Hour, CapacityLedgerRetention: 3 * time.Hour,
	}
	if err := complete.Validate(); err != nil || !complete.Enabled() {
		t.Fatalf("complete retention policy: enabled=%v err=%v", complete.Enabled(), err)
	}
}

func TestExpiresAfterNormalizesTerminalTimeToUTC(t *testing.T) {
	terminalAt := time.Date(2026, time.August, 27, 18, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	expiresAt, err := expiresAfter(terminalAt, 48*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, time.August, 29, 10, 30, 0, 0, time.UTC)
	if expiresAt == nil || !expiresAt.Equal(want) || expiresAt.Location() != time.UTC {
		t.Fatalf("expires_at = %v, want %v UTC", expiresAt, want)
	}
}

func TestCapacityLedgerRetentionStartsAfterUTCDayCloses(t *testing.T) {
	budgetDay := time.Date(2026, time.August, 27, 0, 0, 0, 0, time.UTC)
	expiresAt, err := capacityLedgerExpiresAt(budgetDay, 7*24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, time.September, 4, 0, 0, 0, 0, time.UTC)
	if expiresAt == nil || !expiresAt.Equal(want) {
		t.Fatalf("expires_at = %v, want %v", expiresAt, want)
	}
	if _, err := capacityLedgerExpiresAt(budgetDay.Add(time.Hour), time.Hour); err == nil {
		t.Fatal("non-midnight budget day must fail closed")
	}
}
