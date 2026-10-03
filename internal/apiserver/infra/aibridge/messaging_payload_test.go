package aibridge

import (
	"context"
	"errors"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

type payloadRPCFunc func(context.Context, *pb.MessagePayloadReference, ...grpc.CallOption) (*pb.MessagePayload, error)

func (f payloadRPCFunc) Get(ctx context.Context, r *pb.MessagePayloadReference, o ...grpc.CallOption) (*pb.MessagePayload, error) {
	return f(ctx, r, o...)
}

func TestMessagingPayloadReferenceAndExactBody(t *testing.T) {
	id := uuid.NewString()
	body := &pb.MessagingBody{Value: &pb.MessagingBody_EvaluationState{EvaluationState: &pb.EvaluationRuntimeState{RunId: uuid.NewString(), OrganizationId: "18446744073709551615", EventSequence: 1}}}
	e, raw, err := app.PrepareMessaging(pb.MessagingKind_EVALUATION_STATE, id, body.GetEvaluationState().RunId, "", body)
	if err != nil {
		t.Fatal(err)
	}
	ref := &pb.MessagePayloadReference{Producer: e.Producer, Destination: e.Destination, MessageId: id, BodySha256: e.BodySha256, BodyLength: e.BodyLength, OrganizationId: body.GetEvaluationState().OrganizationId}
	e.Body = &pb.MessagingEnvelope_PayloadReference{PayloadReference: ref}
	for _, scenario := range []string{"original", "wrong_org", "wrong_length", "wrong_bytes", "unavailable", "nil_response"} {
		t.Run(scenario, func(t *testing.T) {
			client := &MessagingPayloadClient{RPC: payloadRPCFunc(func(ctx context.Context, r *pb.MessagePayloadReference, _ ...grpc.CallOption) (*pb.MessagePayload, error) {
				deadline, ok := ctx.Deadline()
				if !ok || time.Until(deadline) > 5*time.Second {
					t.Fatal("unbounded payload read")
				}
				if !proto.Equal(ref, r) {
					t.Fatal("reference changed")
				}
				if scenario == "unavailable" {
					return nil, errors.New("sensitive transport details")
				}
				if scenario == "nil_response" {
					return nil, nil
				}
				response := &pb.MessagePayload{Reference: proto.Clone(r).(*pb.MessagePayloadReference), Body: append([]byte(nil), raw...)}
				switch scenario {
				case "wrong_org":
					response.Reference.OrganizationId = "1"
				case "wrong_length":
					response.Reference.BodyLength++
				case "wrong_bytes":
					response.Body[0] ^= 1
				}
				return response, nil
			})}
			got, err := client.Read(context.Background(), e)
			if scenario == "original" {
				if err != nil || string(got) != string(raw) {
					t.Fatal("exact body lost", err)
				}
				got[0] ^= 1
				if string(raw) == string(got) {
					t.Fatal("returned body aliases stored bytes")
				}
			} else if err == nil {
				t.Fatal("invalid response accepted")
			}
		})
	}
	e.Destination = "qs-ai"
	if _, err := (&MessagingPayloadClient{}).Read(context.Background(), e); !errors.Is(err, app.ErrMessagingContract) {
		t.Fatal("wrong direction accepted", err)
	}
}
