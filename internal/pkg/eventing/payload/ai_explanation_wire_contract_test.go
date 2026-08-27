package eventpayload

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAIExplanationEventPayloadsExcludeSensitiveContent(t *testing.T) {
	now := time.Date(2026, 8, 27, 1, 2, 3, 0, time.UTC)
	values := []any{
		AIExplanationRequestedData{OrgID: 1, GenerationID: "701", AssessmentID: "501", TesteeID: 601, SourceReportID: "201", Audience: "participant", RequestedAt: now},
		AIExplanationRetryRequestedData{OrgID: 1, GenerationID: "701", FailedRunID: "801", AssessmentID: "501", TesteeID: 601, SourceReportID: "201", Audience: "participant", ExpectedAttempt: 1, NextAttempt: 2, AttemptOrigin: "manual", ActionRequestID: "retry-1", RequestedAt: now},
		AIExplanationLeaseRecoveryRequestedData{OrgID: 1, GenerationID: "701", RunID: "801", Attempt: 1, ExpectedLeaseExpiresAt: now.Add(-time.Minute), InvocationPhase: "prepared", RequestedAt: now},
		AIExplanationGeneratedData{OrgID: 1, GenerationID: "701", RunID: "801", ArtifactID: "901", AssessmentID: "501", TesteeID: 601, SourceReportID: "201", Audience: "participant", GeneratedAt: now},
		AIExplanationFailedData{OrgID: 1, GenerationID: "701", RunID: "801", AssessmentID: "501", TesteeID: 601, SourceReportID: "201", Audience: "participant", Attempt: 1, FailureKind: "provider_timeout", FailureCode: "provider_timeout", Retryable: true, SafeReason: "AI 解读暂时不可用", FailedAt: now},
	}
	for _, value := range values {
		payload, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		wire := string(payload)
		for _, forbidden := range []string{"prompt", "input_json", "raw_output", "content", "provider_request_id", "requested_by"} {
			if strings.Contains(wire, forbidden) {
				t.Fatalf("%T leaked forbidden field %q: %s", value, forbidden, wire)
			}
		}
	}
}
