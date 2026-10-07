package aibridge

import (
	"context"
	"errors"
	"testing"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
)

type correctionRPC struct {
	pb.EvaluationManagementClient
	calls   int
	command *pb.EvaluationReviewCorrectionCommand
	failure error
}

func (s *correctionRPC) CorrectReview(_ context.Context, command *pb.EvaluationReviewCorrectionCommand, _ ...grpc.CallOption) (*pb.EvaluationState, error) {
	s.calls++
	s.command = command
	if s.failure != nil {
		return nil, s.failure
	}
	return &pb.EvaluationState{RunId: command.Scope.RunId, Version: command.ExpectedVersion + 1, Status: "awaiting_review", ResolutionsJson: "[]", ReviewsJson: "[]", ReopeningsJson: "[]", OriginalReviewsJson: "[]", ReviewCorrectionsJson: "[]", ReviewFingerprintsJson: "[]"}, nil
}
func TestCorrectionForwardsTrustedScopeOnceWithoutImplicitRetry(t *testing.T) {
	rpc := &correctionRPC{}
	client := &EvaluationClient{RPC: rpc}
	scope := app.EvaluationScope{RunID: "00000000-0000-4000-8000-000000000001", OrganizationID: 7, OperatorUserID: 42}
	command := app.EvaluationReviewCorrection{CommandID: "00000000-0000-4000-8000-000000000002", ExpectedVersion: 7, CandidateID: "candidate:1", Role: "safety_product", PreviousReviewFingerprint: "sha256:a", CandidateOutputFingerprint: "sha256:b", Decision: "approve", Reason: "复核", Confirm: true}
	result, err := client.CorrectEvaluationReview(context.Background(), scope, command)
	if err != nil || result.Version != 8 || rpc.calls != 1 || rpc.command.Scope.OperatorUserId != 42 || rpc.command.CommandId != command.CommandID {
		t.Fatalf("%+v %v", result, err)
	}
	rpc.failure = context.DeadlineExceeded
	if _, err := client.CorrectEvaluationReview(context.Background(), scope, command); !errors.Is(err, context.DeadlineExceeded) || rpc.calls != 2 {
		t.Fatal(err)
	}
	for _, raw := range []string{"null", "{}", "[broken"} {
		if _, err := reviewAuditArray(raw, 210); !errors.Is(err, app.ErrConflict) {
			t.Fatal(err)
		}
	}
}
