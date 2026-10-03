package answersheetgap

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/FangcunMount/reliable-messaging/wire/domain"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	"go.mongodb.org/mongo-driver/bson"
)

func recoveryFixture(t *testing.T) (bson.Raw, sheetRow, outboxRow, recoveryConfirmation, time.Time) {
	t.Helper()
	sheet, row := validOriginal(t)
	now := time.Now().Truncate(time.Millisecond)
	at := now.Add(-time.Minute)
	sheet.DurableAcceptance.AcceptedAt = at
	wire, _, err := legacy.Decode(row.Payload)
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(wire.Payload, &envelope); err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(envelope["data"], &data); err != nil {
		t.Fatal(err)
	}
	data["filler_type"], data["submitted_at"] = "self", at
	admission := data["admission"].(map[string]any)
	admission["questionnaire_code"], admission["questionnaire_version"], admission["model_title"] = sheet.QuestionnaireCode, sheet.QuestionnaireVer, "frozen title"
	envelope["data"], err = json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	wire.Payload, err = json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	row.Payload, err = legacy.Encode(wire, legacy.Revision2)
	if err != nil {
		t.Fatal(err)
	}
	row.Fingerprint = fingerprint(t, row)
	raw, err := bson.Marshal(bson.M{"domain_id": sheet.DomainID, "org_id": sheet.OrgID, "filler_type": "self", "filled_at": at,
		"admission":          bson.M{"purpose": "assessment", "questionnaire_code": sheet.QuestionnaireCode, "questionnaire_version": sheet.QuestionnaireVer, "model_title": "frozen title"},
		"durable_acceptance": bson.M{"schema_version": 1, "accepted_at": at, "event_id": sheet.DurableAcceptance.EventID}})
	if err != nil {
		t.Fatal(err)
	}
	return raw, sheet, row, recoveryConfirmation{Version: 2, Attempts: 1, TransportConfirmedAt: &at}, now
}

func TestOriginalRecoveryPreservesWireAndReviewChangesWithSource(t *testing.T) {
	raw, sheet, row, confirmed, now := recoveryFixture(t)
	plan, err := validateRecovery(raw, sheet, row, confirmed, sheet.OrgID, now, now)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plan.Message().Input().Payload, row.Payload) || plan.EventID != sheet.DurableAcceptance.EventID {
		t.Fatal("original wire or event ID changed")
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("payload")) || bytes.Contains(encoded, []byte("frozen title")) {
		t.Fatal("review exposes original body")
	}
	confirmed.Version++
	changed, err := validateRecovery(raw, sheet, row, confirmed, sheet.OrgID, now, now)
	if err != nil || changed.SourceFingerprint == plan.SourceFingerprint {
		t.Fatal("changed original version bypassed review")
	}
}

func TestOriginalRecoveryRejectsUnsafeAuthority(t *testing.T) {
	cases := map[string]func(*sheetRow, *outboxRow, *recoveryConfirmation){
		"foreign scope": func(s *sheetRow, _ *outboxRow, _ *recoveryConfirmation) { s.OrgID++ },
		"independent questionnaire": func(s *sheetRow, _ *outboxRow, _ *recoveryConfirmation) {
			s.Admission.Purpose = "independent_questionnaire"
		},
		"missing frozen model": func(s *sheetRow, _ *outboxRow, _ *recoveryConfirmation) { s.Admission.ModelCode = "" },
		"acceptance after cutoff": func(s *sheetRow, _ *outboxRow, _ *recoveryConfirmation) {
			s.DurableAcceptance.AcceptedAt = time.Now().Add(time.Hour)
		},
		"pending relay":        func(_ *sheetRow, r *outboxRow, _ *recoveryConfirmation) { r.State = "pending" },
		"missing confirmation": func(_ *sheetRow, _ *outboxRow, c *recoveryConfirmation) { c.TransportConfirmedAt = nil },
		"corrupt fingerprint":  func(_ *sheetRow, r *outboxRow, _ *recoveryConfirmation) { r.Fingerprint = []byte("bad") },
		"changed frozen model": func(s *sheetRow, _ *outboxRow, _ *recoveryConfirmation) { s.Admission.ModelVersion = "changed" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			raw, s, r, c, now := recoveryFixture(t)
			change(&s, &r, &c)
			if _, err := validateRecovery(raw, s, r, c, 501, now, now); err == nil {
				t.Fatal("unsafe recovery accepted")
			}
		})
	}
}

func TestOriginalRecoveryRejectsReencodedFrozenMetadata(t *testing.T) {
	raw, sheet, row, confirmed, now := recoveryFixture(t)
	wire, _, err := legacy.Decode(row.Payload)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := domain.DecodeEnvelope(wire.Payload)
	if err != nil {
		t.Fatal(err)
	}
	var data map[string]any
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		t.Fatal(err)
	}
	data["admission"].(map[string]any)["model_title"] = "changed title"
	var full map[string]json.RawMessage
	if err := json.Unmarshal(wire.Payload, &full); err != nil {
		t.Fatal(err)
	}
	full["data"], _ = json.Marshal(data)
	wire.Payload, _ = json.Marshal(full)
	row.Payload, err = legacy.Encode(wire, legacy.Revision2)
	if err != nil {
		t.Fatal(err)
	}
	row.Fingerprint = fingerprint(t, row)
	if _, err := validateRecovery(raw, sheet, row, confirmed, 501, now, now); err == nil {
		t.Fatal("valid fingerprint replaced frozen metadata")
	}
}
