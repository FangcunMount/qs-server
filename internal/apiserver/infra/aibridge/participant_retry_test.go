package aibridge

import (
	"context"
	"errors"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"testing"
	"time"
)

type participantReceiptReadRPC struct {
	pb.ParticipantManagementClient
	t       *testing.T
	calls   int
	request *pb.ParticipantRetryReceiptQuery
	reply   *pb.Receipt
	err     error
}

func (s *participantReceiptReadRPC) GetRetryReceipt(ctx context.Context, request *pb.ParticipantRetryReceiptQuery, _ ...grpc.CallOption) (*pb.Receipt, error) {
	s.calls++
	s.request = request
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 5*time.Second {
		s.t.Fatal("missing read deadline")
	}
	return s.reply, s.err
}
func TestParticipantRetryReceiptQueriesOriginalIdentityWithoutWriting(t *testing.T) {
	const commandID = "00000000-0000-4000-8000-000000000004"
	rpc := &participantReceiptReadRPC{t: t, reply: &pb.Receipt{SessionId: "00000000-0000-4000-8000-000000000001", RunId: "00000000-0000-4000-8000-000000000003", Version: 5, Status: "queued"}}
	client := &ParticipantClient{RPC: rpc}
	scope := app.DraftScope{OrganizationID: 12, OperatorUserID: 34}
	value, err := client.GetParticipantRetryReceipt(context.Background(), scope, commandID)
	if err != nil || value.Version != 5 || rpc.calls != 1 || rpc.request.Scope.OrganizationId != 12 || rpc.request.Scope.OperatorUserId != 34 || rpc.request.CommandId != commandID {
		t.Fatal(value, err, rpc)
	}
	rpc.err = context.DeadlineExceeded
	if _, err = client.GetParticipantRetryReceipt(context.Background(), scope, commandID); !errors.Is(err, context.DeadlineExceeded) || rpc.calls != 2 {
		t.Fatal(err, rpc.calls)
	}
	rpc.err = nil
	for _, reply := range []*pb.Receipt{nil, {SessionId: rpc.reply.SessionId, RunId: rpc.reply.RunId, Version: 1, Status: "queued"}, {SessionId: "wrong", RunId: rpc.reply.RunId, Version: 5, Status: "queued"}, {SessionId: rpc.reply.SessionId, RunId: rpc.reply.RunId, Version: 5, Status: "completed"}} {
		rpc.reply = reply
		if _, err = client.GetParticipantRetryReceipt(context.Background(), scope, commandID); !errors.Is(err, app.ErrConflict) {
			t.Fatal("malformed read receipt accepted", err)
		}
	}
}
