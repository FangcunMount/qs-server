package aibridge

import (
	"crypto/sha256"
	"fmt"
	"testing"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	"google.golang.org/protobuf/proto"
)

func TestMessagingOriginalIdentityHashAndOptionalFalse(t *testing.T) {
	id := "b1941896-e7df-4b9d-9417-b80bd05276b9"
	discard := false
	body := &pb.MessagingBody{Value: &pb.MessagingBody_EvaluationCancel{EvaluationCancel: &pb.EvaluationCancelCommand{Discard: &discard}}}
	envelope, raw, err := PrepareMessaging(pb.MessagingKind_EVALUATION_CANCEL, id, "evaluation:original", id, body)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := ParseMessagingBody(envelope, raw)
	if err != nil || decoded.GetEvaluationCancel().Discard == nil || decoded.GetEvaluationCancel().GetDiscard() {
		t.Fatal("explicit false was lost", err)
	}
	altered := proto.Clone(envelope).(*pb.MessagingEnvelope)
	altered.MessageId = "different"
	if _, err := ParseMessagingBody(altered, raw); err == nil {
		t.Fatal("identity substitution accepted")
	}
	if err := ValidateMessagingHeader(envelope, EventsTopic); err == nil {
		t.Fatal("cross-topic delivery accepted")
	}
	altered = proto.Clone(envelope).(*pb.MessagingEnvelope)
	altered.BodySha256 = fmt.Sprintf("%x", sha256.Sum256([]byte("different")))
	if _, err := ParseMessagingBody(altered, raw); err == nil {
		t.Fatal("body hash mismatch accepted")
	}
}

func TestMessagingBodyKindAndReferenceBinding(t *testing.T) {
	id := "b1941896-e7df-4b9d-9417-b80bd05276b9"
	body := &pb.MessagingBody{Value: &pb.MessagingBody_Start{Start: &pb.StartCommand{RequestId: id, Actor: &pb.Actor{OrgId: "18446744073709551615", SubjectId: "中文🙂"}}}}
	e, raw, err := PrepareMessaging(pb.MessagingKind_START, id, "interpretation:"+id, id, body)
	if err != nil {
		t.Fatal(err)
	}
	r := &pb.MessagePayloadReference{Producer: e.Producer, Destination: e.Destination, MessageId: e.MessageId, BodySha256: e.BodySha256, BodyLength: e.BodyLength, OrganizationId: "18446744073709551615"}
	e.Body = &pb.MessagingEnvelope_PayloadReference{PayloadReference: r}
	if parsed, err := ParseMessagingBody(e, raw); err != nil || parsed.GetStart().Actor.OrgId != r.OrganizationId {
		t.Fatal("immutable reference lost uint64 actor", err)
	}
	r.BodyLength++
	if err := ValidateMessagingHeader(e, CommandsTopic); err == nil {
		t.Fatal("reference substitution accepted")
	}
	if _, _, err := PrepareMessaging(pb.MessagingKind_EVALUATION_START, id, "evaluation:original", id, body); err == nil {
		t.Fatal("body kind substitution accepted")
	}
}
