package statistics

import (
	"testing"
	"time"
)

func TestActivityFactsFreezeKnownAndUnknownAttribution(t *testing.T) {
	at := time.Date(2026, 9, 30, 16, 0, 0, 0, time.UTC)
	legacy, err := activityFact(1, "answersheet_submitted", 9, at, nil)
	if err != nil {
		t.Fatal(err)
	}
	if legacy.Values["unknown_reason"] != "legacy_start_not_captured" {
		t.Fatal("legacy must remain unknown")
	}
	if date := legacy.Values["stat_date"].(time.Time).Format("2006-01-02"); date != "2026-10-01" {
		t.Fatalf("Shanghai date=%s", date)
	}
	start := &activityStart{ID: 99, StartedAt: at.Add(-48 * time.Hour), OwnershipVersion: 2, Version: 1}
	unassigned, err := activityFact(1, "answersheet_submitted", 9, at, start)
	if err != nil || unassigned.Values["unknown_reason"] != "unassigned_at_start" {
		t.Fatal("missing store cause lost")
	}
	store := uint64(10)
	start.StoreID = &store
	first, err := activityFact(1, "answersheet_submitted", 9, at, start)
	if err != nil || first.Values["unknown_reason"] != "" {
		t.Fatal("captured store lost")
	}
	duplicate, _ := activityFact(1, "answersheet_submitted", 9, at, start)
	if hashCore(first.Values) != hashCore(duplicate.Values) {
		t.Fatal("replay changes immutable hash")
	}
	otherCompany, _ := activityFact(2, "answersheet_submitted", 9, at, start)
	success, _ := activityFact(1, "assessment_completed", 9, at.AddDate(0, 1, 0), start)
	if first.Values["fact_key"] == otherCompany.Values["fact_key"] || first.Values["fact_key"] == success.Values["fact_key"] {
		t.Fatal("fact identity conflates company or event")
	}
	if success.Values["stat_date"].(time.Time).Format("2006-01-02") != "2026-10-31" {
		t.Fatal("success must use success event time")
	}
	start.Version = 2
	if _, err := activityFact(1, "answersheet_submitted", 9, at, start); err == nil {
		t.Fatal("invalid start converted to unknown")
	}
}
