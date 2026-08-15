package main

import (
	"testing"
	"time"
)

func TestMongoAdmissionDomainRejectsFingerprintDrift(t *testing.T) {
	at := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	source := mongoAdmissionFailure{
		DomainID: 1, OutcomeID: 2, EventID: "evt-1", Kind: "catalog_not_found",
		Code: "catalog_not_found", SafeMessage: "missing", Fingerprint: "event:wrong",
		Attempt: 1, Decision: "manual_required", FirstFailedAt: at, LastFailedAt: at, OccurredAt: at,
	}
	if _, err := source.domain(); err == nil {
		t.Fatal("fingerprint drift was accepted")
	}
}

func TestConvertOrgCountsRejectsInvalidOrganizationKey(t *testing.T) {
	if _, err := convertOrgCounts(map[string]mongoDriftCounts{"invalid": {Missing: 1}}); err == nil {
		t.Fatal("invalid organization key was accepted")
	}
}
