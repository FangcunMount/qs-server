package handlers

import (
	"testing"
	"time"

	domainStatistics "github.com/FangcunMount/qs-server/internal/apiserver/domain/statistics"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
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

func TestBehaviorEventMappers_FieldMappingMatrix(t *testing.T) {
	occurredAt := time.Date(2026, 6, 6, 8, 30, 0, 0, time.UTC)
	base := map[string]any{
		"org_id":      int64(5),
		"occurred_at": occurredAt,
	}

	cases := []struct {
		name      string
		eventType string
		data      map[string]any
		assert    func(t *testing.T, req *pb.ProjectBehaviorEventRequest)
	}{
		{
			name:      "entry_opened",
			eventType: domainStatistics.EventTypeFootprintEntryOpened,
			data: mergeMaps(base, map[string]any{
				"clinician_id": uint64(12),
				"entry_id":     uint64(34),
			}),
			assert: func(t *testing.T, req *pb.ProjectBehaviorEventRequest) {
				if req.GetClinicianId() != 12 || req.GetEntryId() != 34 || req.GetOrgId() != 5 {
					t.Fatalf("unexpected request: %#v", req)
				}
			},
		},
		{
			name:      "intake_confirmed",
			eventType: domainStatistics.EventTypeFootprintIntakeConfirmed,
			data: mergeMaps(base, map[string]any{
				"clinician_id": uint64(12),
				"entry_id":     uint64(34),
				"testee_id":    uint64(56),
			}),
			assert: func(t *testing.T, req *pb.ProjectBehaviorEventRequest) {
				if req.GetTesteeId() != 56 || req.GetEntryId() != 34 {
					t.Fatalf("unexpected request: %#v", req)
				}
			},
		},
		{
			name:      "testee_profile_created",
			eventType: domainStatistics.EventTypeFootprintTesteeProfileCreated,
			data: mergeMaps(base, map[string]any{
				"clinician_id": uint64(1),
				"entry_id":     uint64(2),
				"testee_id":    uint64(3),
			}),
			assert: func(t *testing.T, req *pb.ProjectBehaviorEventRequest) {
				if req.GetTesteeId() != 3 {
					t.Fatalf("unexpected request: %#v", req)
				}
			},
		},
		{
			name:      "care_relationship_established",
			eventType: domainStatistics.EventTypeFootprintCareRelationshipEstablished,
			data: mergeMaps(base, map[string]any{
				"clinician_id": uint64(7),
				"entry_id":     uint64(8),
				"testee_id":    uint64(9),
			}),
			assert: func(t *testing.T, req *pb.ProjectBehaviorEventRequest) {
				if req.GetClinicianId() != 7 || req.GetTesteeId() != 9 {
					t.Fatalf("unexpected request: %#v", req)
				}
			},
		},
		{
			name:      "care_relationship_transferred",
			eventType: domainStatistics.EventTypeFootprintCareRelationshipTransferred,
			data: mergeMaps(base, map[string]any{
				"from_clinician_id": uint64(10),
				"to_clinician_id":   uint64(11),
				"testee_id":         uint64(12),
			}),
			assert: func(t *testing.T, req *pb.ProjectBehaviorEventRequest) {
				if req.GetSourceClinicianId() != 10 || req.GetClinicianId() != 11 || req.GetTesteeId() != 12 {
					t.Fatalf("unexpected request: %#v", req)
				}
			},
		},
		{
			name:      "answersheet_submitted",
			eventType: domainStatistics.EventTypeFootprintAnswerSheetSubmitted,
			data: mergeMaps(base, map[string]any{
				"testee_id":      uint64(20),
				"answersheet_id": uint64(21),
			}),
			assert: func(t *testing.T, req *pb.ProjectBehaviorEventRequest) {
				if req.GetAnswersheetId() != 21 || req.GetTesteeId() != 20 {
					t.Fatalf("unexpected request: %#v", req)
				}
			},
		},
		{
			name:      "assessment_created",
			eventType: domainStatistics.EventTypeFootprintAssessmentCreated,
			data: mergeMaps(base, map[string]any{
				"testee_id":      uint64(30),
				"answersheet_id": uint64(31),
				"assessment_id":  uint64(32),
			}),
			assert: func(t *testing.T, req *pb.ProjectBehaviorEventRequest) {
				if req.GetAssessmentId() != 32 || req.GetAnswersheetId() != 31 {
					t.Fatalf("unexpected request: %#v", req)
				}
			},
		},
		{
			name:      "report_generated",
			eventType: domainStatistics.EventTypeFootprintReportGenerated,
			data: mergeMaps(base, map[string]any{
				"testee_id":     uint64(40),
				"assessment_id": uint64(41),
				"report_id":     uint64(42),
			}),
			assert: func(t *testing.T, req *pb.ProjectBehaviorEventRequest) {
				if req.GetReportId() != 42 || req.GetAssessmentId() != 41 {
					t.Fatalf("unexpected request: %#v", req)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mapper := behaviorEventMappers[tc.eventType]
			req, err := mapper(mustBuildBehaviorEventPayload(t, "evt-"+tc.name, tc.eventType, tc.data))
			if err != nil {
				t.Fatalf("mapper returned error: %v", err)
			}
			if req.GetEventId() != "evt-"+tc.name || req.GetEventType() != tc.eventType {
				t.Fatalf("unexpected envelope fields: %#v", req)
			}
			tc.assert(t, req)
		})
	}
}

func mergeMaps(base, extra map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}
