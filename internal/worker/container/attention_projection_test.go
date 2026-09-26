package container

import (
	"context"
	"errors"
	"testing"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/internalapi"
)

type attentionRPCStub struct {
	request  *pb.SyncAssessmentAttentionRequest
	response *pb.SyncAssessmentAttentionResponse
	err      error
}

func (s *attentionRPCStub) SyncAssessmentAttention(_ context.Context, request *pb.SyncAssessmentAttentionRequest) (*pb.SyncAssessmentAttentionResponse, error) {
	s.request = request
	return s.response, s.err
}

func TestAttentionProjectionRequiresBusinessSuccessResponse(t *testing.T) {
	for _, test := range []struct {
		name     string
		response *pb.SyncAssessmentAttentionResponse
		err      error
		wantErr  bool
	}{
		{name: "accepted", response: &pb.SyncAssessmentAttentionResponse{Success: true}},
		{name: "rejected", response: &pb.SyncAssessmentAttentionResponse{Success: false}, wantErr: true},
		{name: "missing response", wantErr: true},
		{name: "transport error", err: errors.New("rpc unavailable"), wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			rpc := &attentionRPCStub{response: test.response, err: test.err}
			err := (&internalAttentionSyncClient{client: rpc}).SyncAssessmentAttention(t.Context(), 42, "high", true)
			if (err != nil) != test.wantErr {
				t.Fatalf("sync error = %v, wantErr %t", err, test.wantErr)
			}
			if rpc.request == nil || rpc.request.TesteeId != 42 || rpc.request.RiskLevel != "high" || !rpc.request.MarkKeyFocus {
				t.Fatalf("forwarded attention request = %+v", rpc.request)
			}
		})
	}
}
