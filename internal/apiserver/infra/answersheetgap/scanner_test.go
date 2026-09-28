package answersheetgap

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
)

func TestValidateOriginalRejectsChangedFrozenAdmissionEvenWithValidFingerprint(t *testing.T) {
	sheet, row := validOriginal(t)
	if err := validateOriginal(sheet, row); err != nil {
		t.Fatalf("valid original rejected: %v", err)
	}
	changed := row
	changed.Payload = makeWire(t, sheet, "MODEL-NEW")
	changed.Fingerprint = fingerprint(t, changed)
	if err := validateOriginal(sheet, changed); err == nil {
		t.Fatal("changed frozen model must be quarantined, not used for repair")
	}
}

func TestValidateOriginalRejectsChangedScopeOrCorruptPayload(t *testing.T) {
	sheet, row := validOriginal(t)
	changedScope := row
	changedScope.Scope = "org:999"
	changedScope.Fingerprint = fingerprint(t, changedScope)
	if err := validateOriginal(sheet, changedScope); err == nil {
		t.Fatal("changed organization scope must be rejected")
	}
	corruptWire := row
	corruptWire.Payload = []byte(`{"type":"component-base.messaging.message.v1","uuid":"evt-42","payload":"e30="}`)
	corruptWire.Fingerprint = fingerprint(t, corruptWire)
	if err := validateOriginal(sheet, corruptWire); err == nil {
		t.Fatal("corrupt transport envelope must be rejected")
	}
}

func validOriginal(t *testing.T) (sheetRow, outboxRow) {
	t.Helper()
	sheet := sheetRow{DomainID: 42, OrgID: 501, TesteeID: 601, FillerID: 701, QuestionnaireCode: "QNR", QuestionnaireVer: "1.0.0"}
	sheet.DurableAcceptance.EventID = "evt-42"
	sheet.Admission = &struct {
		Purpose        string `bson:"purpose"`
		ModelKind      string `bson:"model_kind"`
		ModelCode      string `bson:"model_code"`
		ModelVersion   string `bson:"model_version"`
		ModelAlgorithm string `bson:"model_algorithm"`
	}{Purpose: "assessment", ModelKind: "scale", ModelCode: "MODEL-1", ModelVersion: "1.0.0"}
	row := outboxRow{
		Producer: producer, MessageID: sheet.DurableAcceptance.EventID, Destination: destination,
		EventType: eventType, SchemaVersion: "v1", Scope: "org:501",
		ContentType: "application/json", OccurredAt: time.Date(2026, 9, 28, 10, 0, 0, 0, time.FixedZone("UTC+8", 8*3600)).Format(time.RFC3339Nano),
		State: "published", Payload: makeWire(t, sheet, "MODEL-1"),
	}
	row.Fingerprint = fingerprint(t, row)
	return sheet, row
}

func makeWire(t *testing.T, sheet sheetRow, modelCode string) []byte {
	t.Helper()
	data, err := json.Marshal(map[string]any{
		"answersheet_id":     strconv.FormatUint(sheet.DomainID, 10),
		"questionnaire_code": sheet.QuestionnaireCode, "questionnaire_version": sheet.QuestionnaireVer,
		"testee_id": sheet.TesteeID, "org_id": sheet.OrgID, "filler_id": sheet.FillerID,
		"admission": map[string]any{"purpose": "assessment", "model_kind": "scale", "model_code": modelCode, "model_version": "1.0.0"},
	})
	if err != nil {
		t.Fatal(err)
	}
	env, err := json.Marshal(map[string]any{
		"id": sheet.DurableAcceptance.EventID, "eventType": eventType,
		"aggregateType": "AnswerSheet", "aggregateID": strconv.FormatUint(sheet.DomainID, 10), "data": json.RawMessage(data),
	})
	if err != nil {
		t.Fatal(err)
	}
	wire := legacy.Envelope{UUID: sheet.DurableAcceptance.EventID, Metadata: map[string]string{"event_type": eventType}, Payload: env}
	encoded, err := legacy.Encode(wire, legacy.Revision2)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func fingerprint(t *testing.T, row outboxRow) []byte {
	t.Helper()
	m, err := message.New(message.Input{
		Producer: row.Producer, ID: row.MessageID, Destination: row.Destination,
		EventType: row.EventType, SchemaVersion: row.SchemaVersion, Scope: row.Scope,
		ContentType: row.ContentType, OccurredAt: row.OccurredAt, Payload: row.Payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	hash := m.Fingerprint()
	return hash[:]
}
