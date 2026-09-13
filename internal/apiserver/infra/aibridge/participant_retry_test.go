package aibridge

import (
	"context"
	"errors"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
)

type participantRetryRPC struct {
	pb.ParticipantManagementClient
	t       *testing.T
	calls   int
	request *pb.ParticipantRetryCommand
	reply   *pb.Receipt
	err     error
}

func (s *participantRetryRPC) Retry(ctx context.Context, request *pb.ParticipantRetryCommand, _ ...grpc.CallOption) (*pb.Receipt, error) {
	s.calls++
	s.request = request
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 5*time.Second {
		s.t.Fatal("missing deadline")
	}
	return s.reply, s.err
}
func TestParticipantRetryForwardsOnceAndTreatsBadReceiptAsUnknown(t *testing.T) {
	sessionID := "00000000-0000-4000-8000-000000000001"
	command := app.ParticipantRetry{CommandID: "00000000-0000-4000-8000-000000000004", ExpectedRunID: "00000000-0000-4000-8000-000000000002", ExpectedVersion: 4, Reason: "已核对", Confirm: true, ExpectedProviderInvocations: 1, AcceptResultUnknownRisk: true}
	rpc := &participantRetryRPC{t: t, reply: &pb.Receipt{SessionId: sessionID, RunId: "00000000-0000-4000-8000-000000000003", Version: 5, Status: "queued"}}
	client := &ParticipantClient{RPC: rpc}
	scope := app.DraftScope{OrganizationID: 12, OperatorUserID: 34}
	value, err := client.RetryParticipant(context.Background(), scope, sessionID, command)
	if err != nil || value.Version != 5 || rpc.calls != 1 || rpc.request.Scope.OrganizationId != 12 || rpc.request.Scope.OperatorUserId != 34 || rpc.request.CommandId != command.CommandID || !rpc.request.AcceptResultUnknownRisk {
		t.Fatal(value, err, rpc)
	}
	rpc.err = context.DeadlineExceeded
	if _, err = client.RetryParticipant(context.Background(), scope, sessionID, command); !errors.Is(err, context.DeadlineExceeded) || rpc.calls != 2 {
		t.Fatal(err, rpc.calls)
	}
	rpc.err = nil
	rpc.reply.Version = 7
	if _, err = client.RetryParticipant(context.Background(), scope, sessionID, command); err == nil || errors.Is(err, app.ErrConflict) {
		t.Fatal("malformed post-commit receipt must not look like a rejected command", err)
	}
}
