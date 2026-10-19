package answersheet

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	submit "github.com/FangcunMount/qs-server/internal/apiserver/port/answersheetsubmit"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"go.mongodb.org/mongo-driver/bson"
)

func submissionPlanForTest(t *testing.T) transaction.FrozenPlan {
	t.Helper()
	sheet := newMapperSubmittedSheet(t)
	fingerprint, err := submit.Fingerprint(sheet)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := freezeSubmission(context.Background(), sheet, submit.DurableSubmitMeta{WriterID: 301, IdempotencyKey: "same-key", Fingerprint: fingerprint, RequestID: "original-request"}, "answersheet.submitted")
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestTransactionSubmissionPlanFreezesBSONAndOriginalWire(t *testing.T) {
	plan := submissionPlanForTest(t)
	encoded, err := transaction.EncodePlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := transaction.DecodePlan(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plan.Input().Branches[0].Wire, restored.Input().Branches[0].Wire) {
		t.Fatal("first wire changed")
	}
	po, _, err := readSubmissionPlan(restored)
	if err != nil {
		t.Fatal(err)
	}
	if po.SubmitMeta.RequestID != "original-request" || po.DurableAcceptance.EventEvidence != nil || po.DurableAcceptance.TransactionEvidence != nil {
		t.Fatal("private artifact confused with commit proof")
	}
	e := transaction.Execution{Plan: restored, Fence: 1, WriterEpoch: "m3-1", LeaseUntil: time.Now().Add(time.Minute), Token: [32]byte{1}}
	first, sheet, err := materializeSubmission(e)
	if err != nil || sheet == nil {
		t.Fatalf("materialize: %v", err)
	}
	second, _, err := materializeSubmission(e)
	if err != nil || !bytes.Equal(first, second) {
		t.Fatal("callback reentry would change BSON identity, audit clocks or witness")
	}
	var committed AnswerSheetPO
	if err := bson.Unmarshal(first, &committed); err != nil {
		t.Fatal(err)
	}
	proof := committed.DurableAcceptance.TransactionEvidence
	if proof == nil || proof.Validate() != nil || proof.Reference.EventID != plan.Input().OperationID || committed.ID != po.ID {
		t.Fatal("V2 witness lost original identity")
	}
	if committed.DurableAcceptance.EventEvidence != nil {
		t.Fatal("V1 Outbox reference fabricated")
	}
	// The SDK identity includes the complete private artifact; the consumer
	// body retains the original Revision2 envelope with its independent hash.
	in := restored.Input().Branches[0].Message.Input()
	in.Payload = restored.Input().Branches[0].Wire
	original, err := message.New(in)
	if err != nil {
		t.Fatal(err)
	}
	originalRef := standard.ReferenceFromMessage(original)
	if _, err := standard.VerifyReference(in, originalRef.Fingerprint, originalRef); err != nil {
		t.Fatal(err)
	}
	if originalRef.Fingerprint == proof.Reference.Fingerprint {
		t.Fatal("private artifact must be bound separately from original wire")
	}
}

func TestTransactionSubmissionDuplicateCandidateUsesStableInput(t *testing.T) {
	winner, loser := submissionPlanForTest(t), submissionPlanForTest(t)
	if winner.Digest() == loser.Digest() || winner.Input().OperationID == loser.Input().OperationID {
		t.Fatal("fixture needs separately generated event identities")
	}
	if err := transaction.RequestFor(winner).Match(loser); err != nil {
		t.Fatalf("generated identity incorrectly conflicts with business input: %v", err)
	}
	if submissionKey(301, "same-key", "op") == submissionKey(302, "same-key", "op") || submissionKey(301, "", "op1") == submissionKey(301, "", "op2") {
		t.Fatal("writer or unkeyed operation not isolated")
	}
}

func TestTransactionSubmissionRejectsArtifactAndWireDrift(t *testing.T) {
	for _, scenario := range []string{"scope", "wire", "identity", "input", "request", "v1"} {
		t.Run(scenario, func(t *testing.T) {
			in := submissionPlanForTest(t).Input()
			b := &in.Branches[0]
			var artifact submissionArtifact
			if err := json.Unmarshal(b.Message.Input().Payload, &artifact); err != nil {
				t.Fatal(err)
			}
			var po AnswerSheetPO
			if err := bson.Unmarshal(artifact.Document, &po); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "scope":
				po.OrgID++
			case "wire":
				b.Wire = []byte("{}")
			case "identity":
				po.DomainID++
			case "input":
				po.Answers[0].Value.Value = "changed answer"
			case "request":
				po.DurableAcceptance.RequestID = "changed"
			case "v1":
				po.DurableAcceptance.SchemaVersion = 1
			}
			var err error
			artifact.Document, err = bson.Marshal(po)
			if err != nil {
				t.Fatal(err)
			}
			mi := b.Message.Input()
			mi.Payload, err = json.Marshal(artifact)
			if err != nil {
				t.Fatal(err)
			}
			b.Message, err = message.New(mi)
			if err != nil {
				t.Fatal(err)
			}
			changed, err := transaction.NewPlan(in)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := readSubmissionPlan(changed); err == nil {
				t.Fatal("accepted a newly hashed but inconsistent business artifact")
			}
		})
	}
}
