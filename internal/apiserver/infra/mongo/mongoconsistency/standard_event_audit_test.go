package mongoconsistency

import (
	"context"
	"errors"
	standard "github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	"github.com/FangcunMount/qs-server/internal/pkg/event"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/evidence"
	"github.com/FangcunMount/reliable-messaging/message"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"strings"
	"testing"
	"time"
)

func TestOutboxCursorRequiresExactSDKDocumentOrderAndTypes(t *testing.T) {
	cases := []struct {
		name  string
		value any
		valid bool
	}{
		{"sdk", bson.D{{Key: "producer", Value: "qs-server"}, {Key: "message_id", Value: "msg:1"}, {Key: "destination", Value: "topic"}}, true},
		{"wrong_order", bson.D{{Key: "message_id", Value: "msg:1"}, {Key: "producer", Value: "qs-server"}, {Key: "destination", Value: "topic"}}, false},
		{"extra", bson.D{{Key: "producer", Value: "qs-server"}, {Key: "message_id", Value: "msg:1"}, {Key: "destination", Value: "topic"}, {Key: "state", Value: "pending"}}, false},
		{"numeric", bson.D{{Key: "producer", Value: "qs-server"}, {Key: "message_id", Value: 1}, {Key: "destination", Value: "topic"}}, false},
		{"objectid", bson.D{{Key: "_id", Value: primitive.NewObjectID()}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := bson.Marshal(tc.value)
			if err != nil {
				t.Fatal(err)
			}
			token, err := decodeOutboxToken(raw)
			if (err == nil) != tc.valid {
				t.Fatalf("cursor=%#v err=%v", token, err)
			}
		})
	}
	for _, raw := range [][]byte{nil, []byte("not BSON"), make([]byte, 1025)} {
		if _, err := decodeOutboxToken(raw); err == nil {
			t.Fatal("accepted malformed BSON")
		}
	}
}

type storedRowTestResolver struct{}

func (storedRowTestResolver) GetTopicForEvent(kind string) (string, bool) {
	return "test." + kind, true
}

func storedRowFixture(t *testing.T, kind string) standardEventRow {
	t.Helper()
	evt := event.New(kind, "Test", "1", struct {
		OrgID int64 `json:"org_id"`
	}{1})
	intents, err := standard.PrepareIntents([]event.DomainEvent{evt}, storedRowTestResolver{}, "api-server")
	if err != nil {
		t.Fatal(err)
	}
	msg := intents[0].Message
	in, fingerprint := msg.Input(), msg.Fingerprint()
	token, err := bson.Marshal(identity(standard.ReferenceFromMessage(msg)))
	if err != nil {
		t.Fatal(err)
	}
	return standardEventRow{ID: token, Producer: in.Producer, MessageID: in.ID, Destination: in.Destination, EventType: in.EventType, SchemaVersion: in.SchemaVersion, Scope: in.Scope, ContentType: in.ContentType, OccurredAt: in.OccurredAt, Payload: in.Payload, Fingerprint: fingerprint[:]}
}

func TestStoredRowTypeCannotHideInnerEventEvenWithRecomputedSDKFingerprint(t *testing.T) {
	for _, recompute := range []bool{false, true} {
		row := storedRowFixture(t, "answersheet.submitted")
		row.EventType = "uncovered.type"
		if recompute {
			msg, err := message.New(row.input())
			if err != nil {
				t.Fatal(err)
			}
			fingerprint := msg.Fingerprint()
			row.Fingerprint = fingerprint[:]
		}
		if _, err := verifyStoredRow(row); err == nil || !isEvidenceDrift(err) {
			t.Fatalf("corrupt outer type escaped dual-layer verification: recomputed=%t err=%v", recompute, err)
		}
	}
	row := storedRowFixture(t, "uncovered.type")
	inner, err := verifyStoredRow(row)
	if err != nil || coveredEventType(inner.EventType) {
		t.Fatalf("valid noncovered message acquired new business auditing: %v", err)
	}
}
func TestHistoricalEvidenceDoesNotRequireOrClaimCurrentMessage(t *testing.T) {
	binding := strings.Repeat("a", 64)
	for _, class := range []evidence.Class{evidence.RetiredVerified, evidence.Unverifiable} {
		proof := &evidence.EventEvidenceV1{Version: 1, Class: class, EventID: "old-event", Digest: evidence.SourceDigest("mongo-selected-source-bson-v1", []byte("source")), BusinessBindingSHA256: binding, Origin: "retirement", Verification: evidence.Verification{Method: "verified-business-source", Version: "v1", OperationID: "op", Reason: "source_absent", VerifiedAt: time.Now(), BusinessTerminal: true, OwnershipVerified: true, ResponsibilityClosed: true}}
		if err := (*Scanner)(nil).checkEvidence(context.Background(), proof, "old-event", "type", "aggregate", "1", binding, nil); err != nil {
			t.Fatal(err)
		}
		proof.Verification.ResponsibilityClosed = false
		if err := (*Scanner)(nil).checkEvidence(context.Background(), proof, "old-event", "type", "aggregate", "1", binding, nil); err == nil {
			t.Fatal("open historical responsibility was treated as verified")
		}
	}
}
func TestInfrastructureErrorCannotBeConvertedToBusinessDrift(t *testing.T) {
	if isEvidenceDrift(context.DeadlineExceeded) || isEvidenceDrift(errors.New("SQL connection failed")) {
		t.Fatal("infrastructure failure was swallowed as drift")
	}
	if !isEvidenceDrift(driftf("business mismatch")) {
		t.Fatal("conflict was not classified")
	}
	_, err := (&Scanner{}).outcome(context.Background(), 1)
	if err == nil || isEvidenceDrift(err) {
		t.Fatal("missing evaluation capability was converted to business drift")
	}
}
