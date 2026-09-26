package systemgovernance

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/component-base/pkg/eventcodec"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/systemgovernance"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/outcome"
)

type reportResolutionMetadataStub struct {
	metadata interpretationreadmodel.CurrentReportMetadata
	row      interpretationreadmodel.ReportRow
}

func (s *reportResolutionMetadataStub) GetCurrentReportMetadataByAssessmentIDs(_ context.Context, _ []uint64) (map[uint64]interpretationreadmodel.CurrentReportMetadata, error) {
	return map[uint64]interpretationreadmodel.CurrentReportMetadata{s.metadata.AssessmentID: s.metadata}, nil
}

func (s *reportResolutionMetadataStub) GetReportByAssessmentID(_ context.Context, _ uint64) (*interpretationreadmodel.ReportRow, error) {
	row := s.row
	return &row, nil
}

func TestReportGeneratedResolutionRequiresEveryDurableEffectMySQL(t *testing.T) {
	db := openConcurrentResolutionMySQL(t)
	if err := db.Exec(`CREATE TABLE assessment (
		id BIGINT UNSIGNED PRIMARY KEY, org_id BIGINT NOT NULL, testee_id BIGINT UNSIGNED NOT NULL,
		status VARCHAR(32) NOT NULL, deleted_at DATETIME(3) NULL
	) ENGINE=InnoDB`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE interpretation_attention_projection (
		event_id VARCHAR(128) PRIMARY KEY, report_id VARCHAR(64) NOT NULL,
		assessment_id VARCHAR(64) NOT NULL, testee_id BIGINT UNSIGNED NOT NULL,
		risk_level VARCHAR(32) NOT NULL, mark_key_focus TINYINT(1) NOT NULL,
		status VARCHAR(32) NOT NULL, attempt INT NOT NULL, last_error TEXT NULL,
		created_at DATETIME(3) NOT NULL, updated_at DATETIME(3) NOT NULL
	) ENGINE=InnoDB`).Error; err != nil {
		t.Fatal(err)
	}
	data := eventoutcome.ReportGeneratedPayload{
		OrgID: 7, ReportID: "101", AssessmentID: "42", OutcomeID: "201",
		GenerationID: "301", RunID: "401", TesteeID: 1000,
		Level: &eventoutcome.ResultLevel{Code: "high", Severity: "high"},
	}
	evt := event.New("interpretation.report.generated", "InterpretationGeneration", data.GenerationID, data)
	payload, err := eventcodec.EncodeDomainEvent(evt)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	original := actionRunPO{
		RequestID: "original-1", ActionID: "events.replay_delivery", OrgID: 7,
		ActorUserID: 11, InputJSON: `{"targets":[{"id":41,"expected_delivery_attempts":8}]}`,
		Status: "failed", ResultJSON: "null", StartedAt: now,
	}
	if err := db.Create(&original).Error; err != nil {
		t.Fatal(err)
	}
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO assessment (id,org_id,testee_id,status) VALUES (42,7,1000,'evaluated')`, nil},
		{`INSERT INTO interpretation_attention_projection
			(event_id,report_id,assessment_id,testee_id,risk_level,mark_key_focus,status,attempt,created_at,updated_at)
			VALUES (?,'101','42',1000,'high',1,'succeeded',0,?,?)`, []any{evt.EventID(), now, now}},
		{`INSERT INTO event_delivery_dead_letter
			(id,message_id,event_id,org_id,provider,topic_name,channel_name,delivery_attempts,payload_json,retry_disposition,replay_request_id,failed_at)
			VALUES (41,'message-41',?,7,'nsq','topic','channel',8,?,'automatic','original-1',?)`, []any{evt.EventID(), string(payload), now}},
	} {
		if err := db.Exec(statement.query, statement.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	reports := &reportResolutionMetadataStub{metadata: interpretationreadmodel.CurrentReportMetadata{
		AssessmentID: 42, Status: interpretationreadmodel.CurrentReportMetadataFound,
		SourceKind: "artifact", SourceID: 101, OrgID: 7, TesteeID: 1000,
		OutcomeID: 201, GenerationID: 301, RunID: 401,
	}, row: interpretationreadmodel.ReportRow{
		ReportID: 101, AssessmentID: 42,
		Level: &interpretationreadmodel.ResultLevelRow{Code: "high", Severity: "high"},
	}}
	store := NewActionAuditStore(db)
	req := DeliveryResolutionRequest{
		OrgID: 7, ActorUserID: 22, RequestID: "resolution-1", OriginalReplayRequestID: "original-1",
		DeadLetterID: 41, EventID: evt.EventID(), ExpectedDeliveryAttempts: 8, Reason: "verified effects",
	}
	verify := NewReportGeneratedResolutionVerifier(reports)
	if err := store.ResolveDelivery(t.Context(), req, NewReportGeneratedResolutionVerifier(nil)); err == nil {
		t.Fatal("missing Mongo reader must reject resolution")
	}
	var otherEvent map[string]json.RawMessage
	if err := json.Unmarshal(payload, &otherEvent); err != nil {
		t.Fatal(err)
	}
	otherEvent["eventType"] = json.RawMessage(`"task.completed"`)
	otherPayload, err := json.Marshal(otherEvent)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE event_delivery_dead_letter SET payload_json = ? WHERE id = 41`, string(otherPayload)).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.ResolveDelivery(t.Context(), req, verify); err == nil {
		t.Fatal("another event type must not borrow report evidence")
	}
	if err := db.Exec(`UPDATE event_delivery_dead_letter SET payload_json = ? WHERE id = 41`, string(payload)).Error; err != nil {
		t.Fatal(err)
	}
	reports.metadata.SourceID = 102
	if err := store.ResolveDelivery(t.Context(), req, verify); err == nil {
		t.Fatal("a different current report must not close the original event")
	}
	reports.metadata.SourceID = 101
	reports.metadata.OrgID = 8
	if err := store.ResolveDelivery(t.Context(), req, verify); err == nil {
		t.Fatal("a foreign organization report must not close the original event")
	}
	reports.metadata.OrgID = 7
	reports.row.ReportID = 102
	if err := store.ResolveDelivery(t.Context(), req, verify); err == nil {
		t.Fatal("a different participant-visible report must not close the original event")
	}
	reports.row.ReportID = 101
	reports.row.Model.Code = "SDS"
	if err := store.ResolveDelivery(t.Context(), req, verify); err == nil {
		t.Fatal("a participant report missing its frozen presentation profile must not close the event")
	}
	reports.row.Model.Code = ""
	if err := db.Exec(`UPDATE interpretation_attention_projection SET risk_level = 'low' WHERE event_id = ?`, evt.EventID()).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.ResolveDelivery(t.Context(), req, verify); err == nil {
		t.Fatal("a mismatched attention effect must not close the original event")
	}
	if err := db.Exec(`UPDATE interpretation_attention_projection SET risk_level = 'high' WHERE event_id = ?`, evt.EventID()).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE assessment SET status = 'failed' WHERE id = 42`).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.ResolveDelivery(t.Context(), req, verify); err == nil {
		t.Fatal("an assessment that cannot show completion must not close the event")
	}
	if err := db.Exec(`UPDATE assessment SET status = 'evaluated' WHERE id = 42`).Error; err != nil {
		t.Fatal(err)
	}
	assertResolutionState(t, db, "automatic", 0)
	resolver := NewReportGeneratedDeliveryResolver(db, reports)
	governance := app.NewFacade(app.FacadeDeps{DeliveryResolver: resolver})
	input := app.DeliveryResolutionRequest{
		RequestID: req.RequestID, OriginalReplayRequestID: req.OriginalReplayRequestID,
		DeadLetterID: req.DeadLetterID, EventID: req.EventID,
		ExpectedDeliveryAttempts: req.ExpectedDeliveryAttempts, Reason: req.Reason, Confirm: true,
	}
	result, err := governance.ResolveDelivery(t.Context(), req.OrgID, req.ActorUserID, input)
	if err != nil {
		t.Fatalf("all exact durable effects should close the event: %v", err)
	}
	if result == nil || result.ActionID != deliveryResolutionActionID || result.Result["evidence_kind"] != "report_generated_current_fact_and_attention" {
		t.Fatalf("report resolution lost committed evidence: %+v", result)
	}
	assertResolutionState(t, db, deliveryResolutionDisposition, 1)
	if same, err := governance.ResolveDelivery(t.Context(), req.OrgID, req.ActorUserID, input); err != nil || same == nil || same.RequestID != result.RequestID {
		t.Fatalf("same operator request should read committed result: %+v/%v", same, err)
	}
	if receipt, err := governance.GetDeliveryResolution(t.Context(), req.OrgID, req.RequestID); err != nil || receipt == nil || receipt.Result["evidence_reference"] == "" {
		t.Fatalf("committed receipt was not readable: %+v/%v", receipt, err)
	}
	if _, err := governance.GetDeliveryResolution(t.Context(), 8, req.RequestID); err == nil {
		t.Fatal("another organization read the resolution receipt")
	}
	if _, err := governance.GetDeliveryResolution(t.Context(), req.OrgID, req.OriginalReplayRequestID); err == nil {
		t.Fatal("original replay audit was misrepresented as a resolution receipt")
	}
	var originalAfter actionRunPO
	if err := db.Where("id = ?", original.ID).Take(&originalAfter).Error; err != nil {
		t.Fatal(err)
	}
	if originalAfter.Status != "failed" {
		t.Fatal("resolution rewrote the original failed replay audit")
	}
}

var _ ReportGeneratedResolutionReader = (*reportResolutionMetadataStub)(nil)
