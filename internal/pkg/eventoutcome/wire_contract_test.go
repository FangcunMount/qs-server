package eventoutcome

import (
	"encoding/json"
	"testing"
	"time"
)

func TestReportGeneratedPayloadWireContract(t *testing.T) {
	t.Parallel()

	payload, err := json.Marshal(ReportGeneratedPayload{
		OrgID: 7, GenerationID: "generation-1", RunID: "run-1", ReportID: "report-1",
		AssessmentID: "assessment-1", OutcomeID: "outcome-1", TesteeID: 9, Attempt: 2,
		ReportType: "scale", TemplateVersion: "v1", BuilderIdentity: "builder",
		ContentSchemaVersion: "v2", Model: ModelIdentity{Kind: "scale", Code: "model-1", Version: "v3"},
		GeneratedAt: time.Date(2026, time.July, 13, 10, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	want := `{"org_id":7,"generation_id":"generation-1","run_id":"run-1","report_id":"report-1","assessment_id":"assessment-1","outcome_id":"outcome-1","testee_id":9,"attempt":2,"report_type":"scale","template_version":"v1","builder_identity":"builder","content_schema_version":"v2","model":{"kind":"scale","code":"model-1","version":"v3"},"generated_at":"2026-07-13T10:30:00Z"}`
	if got := string(payload); got != want {
		t.Fatalf("wire JSON = %s, want %s", got, want)
	}
}
