package aibridge

import (
	"context"
	"encoding/json"
	"errors"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"reflect"
	"strings"
	"testing"
)

func (s *evaluationRPCStub) Review(ctx context.Context, r *pb.EvaluationReviewCommand, _ ...grpc.CallOption) (*pb.EvaluationState, error) {
	s.review = r
	response, err := s.ResolveUnknown(ctx, &pb.UnknownResolutionCommand{Scope: r.Scope, ExpectedVersion: r.ExpectedVersion})
	if response != nil {
		response.ReviewsJson = `[{"reviewer":"user:42"}]`
	}
	return response, err
}

func TestReviewPreservesBatchAndNeverRetriesUnknownResult(t *testing.T) {
	rpc := &evaluationRPCStub{t: t}
	client := &EvaluationClient{RPC: rpc}
	scope := app.EvaluationScope{RunID: "run:1", OrganizationID: 7, OperatorUserID: 42}
	command := app.EvaluationReview{ExpectedVersion: 7, Role: "safety_product", Reviews: []app.CandidateReviewItem{{
		CandidateID: "candidate:1", Decision: "approve", Reason: "人工审核",
		SemanticReview: &app.SemanticContradictionReview{PolicyVersion: "semantic-contradiction-dual-review/v1", ExecutionID: "semantic:1", OutputFingerprint: "sha256:" + strings.Repeat("a", 64), AssertionOrdinal: 1, OriginalDetail: "原始判定", CandidateExcerpt: "原文", Reason: "复核"},
	}}}
	reply, err := client.ReviewEvaluation(context.Background(), scope, command)
	if err != nil {
		t.Fatal(err)
	}
	if rpc.calls != 1 || rpc.review.Scope.OrganizationId != 7 || rpc.review.Scope.OperatorUserId != 42 || rpc.review.Role != command.Role || rpc.review.ExpectedVersion != 7 || string(reply.Reviews) != `[{"reviewer":"user:42"}]` {
		t.Fatal("review scope or history changed")
	}
	want, _ := json.Marshal(command.Reviews[0])
	got, _ := (protojson.MarshalOptions{UseProtoNames: true}).Marshal(rpc.review.Reviews[0])
	var wantFields, gotFields any
	if json.Unmarshal(want, &wantFields) != nil || json.Unmarshal(got, &gotFields) != nil || !reflect.DeepEqual(wantFields, gotFields) {
		t.Fatal("review evidence changed in transit")
	}
	rpc.fail = context.DeadlineExceeded
	if _, err := client.ReviewEvaluation(context.Background(), scope, command); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if rpc.calls != 2 {
		t.Fatal("uncertain review retried")
	}
}

func TestReviewHistoryRequiresBoundedArrayAndAcceptsOlderAIResponse(t *testing.T) {
	scope := app.EvaluationScope{RunID: "run:1"}
	for _, raw := range []string{"null", "{}", "bad", "[" + strings.Repeat("{},", 70) + "{}]"} {
		_, err := state(&pb.EvaluationState{RunId: scope.RunID, Version: 1, ResolutionsJson: "[]", ReviewsJson: raw}, scope)
		if !errors.Is(err, app.ErrConflict) {
			t.Fatal("invalid audit history accepted")
		}
	}
	reply, err := state(&pb.EvaluationState{RunId: scope.RunID, Version: 1, ResolutionsJson: "[]"}, scope)
	if err != nil || string(reply.Reviews) != "[]" {
		t.Fatal("old response incompatible")
	}
}
