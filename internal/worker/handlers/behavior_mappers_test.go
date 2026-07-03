package handlers

import (
	"testing"
	"time"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
)

func TestBehaviorEventMappers_RegistersAllFootprintEventTypes(t *testing.T) {
	want := []string{
		domainStatistics.EventTypeFootprintEntryOpened,
		domainStatistics.EventTypeFootprintIntakeConfirmed,
		domainStatistics.EventTypeFootprintTesteeProfileCreated,
		domainStatistics.EventTypeFootprintCareRelationshipEstablished,
		domainStatistics.EventTypeFootprintCareRelationshipTransferred,
		domainStatistics.EventTypeFootprintAnswerSheetSubmitted,
		domainStatistics.EventTypeFootprintAssessmentCreated,
		domainStatistics.EventTypeFootprintReportGenerated,
	}
	for _, eventType := range want {
		if _, ok := behaviorEventMappers[eventType]; !ok {
			t.Fatalf("missing mapper for %q", eventType)
		}
	}
}

func TestBehaviorEventMappers_IntakeConfirmedMapsFields(t *testing.T) {
	occurredAt := time.Date(2026, 6, 6, 8, 30, 0, 0, time.UTC)
	mapper := behaviorEventMappers[domainStatistics.EventTypeFootprintIntakeConfirmed]
	req, err := mapper(mustBuildBehaviorEventPayload(t, "evt-intake", domainStatistics.EventTypeFootprintIntakeConfirmed, map[string]any{
		"org_id":       int64(5),
		"clinician_id": uint64(12),
		"entry_id":     uint64(34),
		"testee_id":    uint64(56),
		"occurred_at":  occurredAt,
	}))
	if err != nil {
		t.Fatalf("mapper returned error: %v", err)
	}
	if req.GetEventId() != "evt-intake" || req.GetTesteeId() != 56 || req.GetEntryId() != 34 {
		t.Fatalf("unexpected mapped request: %#v", req)
	}
}
