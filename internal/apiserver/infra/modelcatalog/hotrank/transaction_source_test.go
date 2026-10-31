package hotrank

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventevidencebinding"

	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/eventmessage"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	payload "github.com/FangcunMount/qs-server/internal/pkg/eventing/payload"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/stretchr/testify/require"
)

func rankTransactionFixture(t *testing.T) (sourceSheet, rankExecution, transaction.FrozenPlan) {
	t.Helper()
	return rankTransactionFixtureAt(t, 100, "Q-A", time.Now().UTC().Truncate(time.Millisecond))
}

func rankTransactionFixtureAt(t *testing.T, id uint64, code string, at time.Time) (sourceSheet, rankExecution, transaction.FrozenPlan) {
	t.Helper()
	data := payload.AnswerSheetSubmittedData{AnswerSheetID: strconv.FormatUint(id, 10), OrgID: 1, TesteeID: 7, FillerID: 8, QuestionnaireCode: code, QuestionnaireVersion: "v1", SubmittedAt: at}
	evt := event.New("answersheet.submitted", "AnswerSheet", data.AnswerSheetID, data)
	evt.OccurredAtValue = at
	wire, err := eventmessage.EncodeWire(evt, "api-server")
	require.NoError(t, err)
	msg, err := message.New(message.Input{Producer: "qs-server", ID: evt.EventID(), Destination: "qs.evaluation.lifecycle", EventType: evt.EventType(), SchemaVersion: "v1", Scope: "org:1", ContentType: "application/json", OccurredAt: at.Format(time.RFC3339Nano), Payload: []byte(`{"version":1,"fixture":"frozen business artifact"}`)})
	require.NoError(t, err)
	plan, err := transaction.NewPlan(transaction.PlanInput{Owner: "qs.mongo", Scope: "org:1", OperationID: evt.EventID(), IdempotencyKey: evt.EventID(), InputHash: sha256.Sum256([]byte("fixture")), Branches: []transaction.BranchInput{{ID: "answersheet.submitted", PhysicalTopic: "fixture-transaction-topic", RouteRevision: "m3-1", Message: msg, Wire: wire}}})
	require.NoError(t, err)
	ref, err := plan.Reference("answersheet.submitted", 1, "fixture")
	require.NoError(t, err)
	binding, err := eventevidencebinding.AnswerSheet(data)
	require.NoError(t, err)
	proof, err := evidence.NewTransactional(eventmessage.ReferenceFromMessage(msg), ref, binding, at)
	require.NoError(t, err)
	sheet := sourceSheet{ID: id, OrgID: 1, TesteeID: 7, FillerID: 8, QuestionnaireCode: code, QuestionnaireVersion: "v1", FilledAt: at}
	sheet.DurableAcceptance.SchemaVersion = 2
	sheet.DurableAcceptance.EventID = evt.EventID()
	sheet.DurableAcceptance.AcceptedAt = at
	sheet.DurableAcceptance.TransactionEvidence = proof
	encoded, err := transaction.EncodePlan(plan)
	require.NoError(t, err)
	digest := plan.Digest()
	key := transaction.ExecutionKey(ref.Owner, ref.OperationID, ref.WriterEpoch, ref.Fence)
	row := rankExecution{ID: key[:], Owner: "qs.mongo", Plan: encoded, Digest: digest[:], Fence: 1, Epoch: "fixture", State: transaction.Committed}
	row.Witnesses = append(row.Witnesses, struct {
		Reference []byte            `bson:"reference"`
		State     transaction.State `bson:"state"`
	}{proof.TransactionReference, transaction.Committed})
	return sheet, row, plan
}

func TestHotRankCurrentTransactionUsesOriginalWireAndCommittedWitness(t *testing.T) {
	sheet, row, _ := rankTransactionFixture(t)
	intent, err := transactionIntent(sheet, row)
	require.NoError(t, err)
	fact, err := validateHotRankOriginal(sheet, intent)
	require.NoError(t, err)
	require.Equal(t, sheet.DurableAcceptance.EventID, fact.EventID)
	require.Equal(t, "Q-A", fact.QuestionnaireCode)
	// The transaction artifact fingerprint and consumer-wire fingerprint differ.
	require.NotEqual(t, sheet.DurableAcceptance.TransactionEvidence.Reference.Fingerprint, hex.EncodeToString(intent.Fingerprint))
}

func TestHotRankRejectsUncommittedMissingOrConflictingTransactionEvidence(t *testing.T) {
	for _, name := range []string{"prepared", "aborted_execution", "aborted", "prepared_witness", "missing_witness", "duplicate_witness", "wrong_witness", "wrong_fence", "wrong_epoch", "wrong_owner", "wrong_execution_id", "changed_plan", "changed_digest", "wrong_binding", "missing_proof", "mixed_versions", "wrong_version", "wrong_event", "wrong_org", "wrong_recorded_at", "wrong_reference"} {
		t.Run(name, func(t *testing.T) {
			sheet, row, _ := rankTransactionFixture(t)
			switch name {
			case "prepared":
				row.State = transaction.Prepared
			case "aborted_execution":
				row.State = transaction.Aborted
			case "aborted":
				row.Witnesses[0].State = transaction.Aborted
			case "prepared_witness":
				row.Witnesses[0].State = transaction.Prepared
			case "missing_witness":
				row.Witnesses = nil
			case "duplicate_witness":
				row.Witnesses = append(row.Witnesses, row.Witnesses[0])
			case "wrong_witness":
				row.Witnesses[0].Reference = []byte("another reference")
			case "wrong_fence":
				row.Fence++
			case "wrong_epoch":
				row.Epoch = "another-epoch"
			case "wrong_owner":
				row.Owner = "qs.mysql"
			case "wrong_execution_id":
				row.ID = make([]byte, 32)
			case "changed_plan":
				row.Plan[0] ^= 1
			case "changed_digest":
				row.Digest = make([]byte, 32)
			case "wrong_binding":
				sheet.DurableAcceptance.TransactionEvidence.BusinessBindingSHA256 = hex.EncodeToString(make([]byte, 32))
			case "missing_proof":
				sheet.DurableAcceptance.TransactionEvidence = nil
			case "mixed_versions":
				sheet.DurableAcceptance.EventEvidence = &evidence.EventEvidenceV1{}
			case "wrong_version":
				sheet.DurableAcceptance.SchemaVersion = 99
			case "wrong_event":
				sheet.DurableAcceptance.EventID = "another-event"
			case "wrong_org":
				sheet.OrgID++
			case "wrong_recorded_at":
				sheet.DurableAcceptance.AcceptedAt = sheet.DurableAcceptance.AcceptedAt.Add(time.Millisecond)
			case "wrong_reference":
				sheet.DurableAcceptance.TransactionEvidence.Reference.Fingerprint = hex.EncodeToString(make([]byte, 32))
			}
			_, err := transactionIntent(sheet, row)
			require.Error(t, err)
		})
	}
}
