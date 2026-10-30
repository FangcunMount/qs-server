package answersheetgap

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventevidencebinding"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	payload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func currentGapFixture(t *testing.T) (bson.Raw, sheetRow, sourceExecution, transaction.FrozenPlan) {
	t.Helper()
	sheet, _ := validOriginal(t)
	at := time.Now().UTC().Add(-time.Minute).Truncate(time.Millisecond)
	data := payload.AnswerSheetSubmittedData{AnswerSheetID: "42", OrgID: sheet.OrgID, TesteeID: sheet.TesteeID, FillerID: uint64(sheet.FillerID), FillerType: "self", QuestionnaireCode: sheet.QuestionnaireCode, QuestionnaireVersion: sheet.QuestionnaireVer, SubmittedAt: at,
		Admission: &payload.AssessmentAdmission{Purpose: payload.AdmissionPurposeAssessment, QuestionnaireCode: sheet.QuestionnaireCode, QuestionnaireVersion: sheet.QuestionnaireVer, ModelKind: sheet.Admission.ModelKind, ModelCode: sheet.Admission.ModelCode, ModelVersion: sheet.Admission.ModelVersion, ModelTitle: "frozen title"}}
	evt := event.New(eventType, "AnswerSheet", "42", data)
	evt.OccurredAtValue = at
	wire, err := standard.EncodeWire(evt, "api-server")
	require.NoError(t, err)
	msg, err := message.New(message.Input{Producer: producer, ID: evt.EventID(), Destination: destination, EventType: eventType, SchemaVersion: "v1", Scope: "org:501", ContentType: "application/json", OccurredAt: at.Format(time.RFC3339Nano), Payload: []byte(`{"version":1,"fixture":"private submission artifact"}`)})
	require.NoError(t, err)
	plan, err := transaction.NewPlan(transaction.PlanInput{Owner: "qs.mongo", Scope: "org:501", OperationID: evt.EventID(), IdempotencyKey: evt.EventID(), InputHash: sha256.Sum256([]byte("gap-fixture")), Branches: []transaction.BranchInput{{ID: eventType, PhysicalTopic: "owned-gap-topic", RouteRevision: "m3-1", Message: msg, Wire: wire}}})
	require.NoError(t, err)
	ref, err := plan.Reference(eventType, 1, "fixture")
	require.NoError(t, err)
	binding, err := eventevidencebinding.AnswerSheet(data)
	require.NoError(t, err)
	proof, err := evidence.NewTransactional(standard.ReferenceFromMessage(msg), ref, binding, at)
	require.NoError(t, err)
	sheet.DurableAcceptance.SchemaVersion = 2
	sheet.DurableAcceptance.EventID = evt.EventID()
	sheet.DurableAcceptance.AcceptedAt = at
	sheet.DurableAcceptance.TransactionEvidence = proof
	encoded, err := transaction.EncodePlan(plan)
	require.NoError(t, err)
	digest := plan.Digest()
	key := transaction.ExecutionKey(ref.Owner, ref.OperationID, ref.WriterEpoch, ref.Fence)
	row := sourceExecution{ID: key[:], Owner: ref.Owner, Plan: encoded, Digest: digest[:], Fence: 1, Epoch: "fixture", State: transaction.Committed}
	row.Witnesses = append(row.Witnesses, struct {
		Reference []byte            `bson:"reference"`
		State     transaction.State `bson:"state"`
	}{proof.TransactionReference, transaction.Committed})
	raw, err := bson.Marshal(sheet)
	require.NoError(t, err)
	var doc bson.M
	require.NoError(t, bson.Unmarshal(raw, &doc))
	doc["filler_type"], doc["filled_at"] = data.FillerType, at
	doc["admission"] = bson.M{"purpose": string(data.Admission.Purpose), "questionnaire_code": data.QuestionnaireCode, "questionnaire_version": data.QuestionnaireVersion, "model_kind": data.Admission.ModelKind, "model_code": data.Admission.ModelCode, "model_version": data.Admission.ModelVersion, "model_title": data.Admission.ModelTitle}
	raw, err = bson.Marshal(doc)
	require.NoError(t, err)
	return raw, sheet, row, plan
}

func TestCurrentGapRecoveryUsesCommittedOriginalWireWithoutBrokerClaim(t *testing.T) {
	raw, sheet, row, plan := currentGapFixture(t)
	intent, confirmation, err := currentIntent(sheet, row)
	require.NoError(t, err)
	require.Nil(t, confirmation.TransportConfirmedAt)
	now := time.Now()
	review, err := validateRecovery(raw, sheet, intent, confirmation, sheet.OrgID, now, now)
	require.NoError(t, err)
	require.Equal(t, sheet.DurableAcceptance.EventID, review.EventID)
	require.True(t, bytes.Equal(plan.Input().Branches[0].Wire, review.Message().Input().Payload))
	encoded, err := json.Marshal(review)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "private submission artifact")
	require.NotContains(t, string(encoded), "frozen title")
	// A committed branch is necessary even when the source wire is still intact.
	confirmation.BranchState = transaction.Prepared
	_, err = validateRecovery(raw, sheet, intent, confirmation, sheet.OrgID, now, now)
	require.ErrorIs(t, err, ErrUnsafeRecoverySource)
}

func TestCurrentGapRejectsConflictingOrUncommittedEvidence(t *testing.T) {
	for _, name := range []string{"prepared", "aborted_witness", "missing_witness", "duplicate_witness", "wrong_owner", "wrong_epoch", "wrong_fence", "wrong_key", "wrong_digest", "corrupt_plan", "missing_proof", "mixed_versions", "downgraded_version", "wrong_scope", "wrong_binding", "changed_admission"} {
		t.Run(name, func(t *testing.T) {
			_, sheet, row, _ := currentGapFixture(t)
			switch name {
			case "prepared":
				row.State = transaction.Prepared
			case "aborted_witness":
				row.Witnesses[0].State = transaction.Aborted
			case "missing_witness":
				row.Witnesses = nil
			case "duplicate_witness":
				row.Witnesses = append(row.Witnesses, row.Witnesses[0])
			case "wrong_owner":
				row.Owner = "qs.mysql"
			case "wrong_epoch":
				row.Epoch = "other"
			case "wrong_fence":
				row.Fence++
			case "wrong_key":
				row.ID = make([]byte, 32)
			case "wrong_digest":
				row.Digest = make([]byte, 32)
			case "corrupt_plan":
				row.Plan = []byte(`{}`)
			case "missing_proof":
				sheet.DurableAcceptance.TransactionEvidence = nil
			case "mixed_versions":
				sheet.DurableAcceptance.EventEvidence = &evidence.EventEvidenceV1{}
			case "downgraded_version":
				sheet.DurableAcceptance.SchemaVersion = 1
			case "wrong_scope":
				sheet.OrgID++
			case "wrong_binding":
				sheet.DurableAcceptance.TransactionEvidence.BusinessBindingSHA256 = string(bytes.Repeat([]byte("a"), 64))
			case "changed_admission":
				sheet.Admission.ModelVersion = "changed"
			}
			_, _, err := currentIntent(sheet, row)
			require.ErrorIs(t, err, ErrUnsafeRecoverySource)
		})
	}
}

func TestCurrentGapRecoveryRejectsChangedFrozenContext(t *testing.T) {
	for _, name := range []string{"request", "attribution"} {
		t.Run(name, func(t *testing.T) {
			raw, sheet, row, _ := currentGapFixture(t)
			intent, confirmation, err := currentIntent(sheet, row)
			require.NoError(t, err)
			var doc bson.M
			require.NoError(t, bson.Unmarshal(raw, &doc))
			if name == "request" {
				doc["durable_acceptance"].(bson.M)["request_id"] = "another-request"
			} else {
				doc["attribution"] = bson.M{"origin_type": "plan", "origin_id": "other", "captured_at": time.Now(), "version": 1, "mode": "self"}
			}
			raw, err = bson.Marshal(doc)
			require.NoError(t, err)
			_, err = validateRecovery(raw, sheet, intent, confirmation, sheet.OrgID, time.Now(), time.Now())
			require.ErrorIs(t, err, ErrUnsafeRecoverySource)
		})
	}
}
