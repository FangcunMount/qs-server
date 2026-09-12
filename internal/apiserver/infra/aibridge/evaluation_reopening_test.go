package aibridge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
)

func (s *evaluationRPCStub) ReopenReview(ctx context.Context, command *pb.EvaluationReopenCommand, _ ...grpc.CallOption) (*pb.EvaluationState, error) {
	s.calls++
	s.reopenCommand = command
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 5*time.Second {
		s.t.Fatal("bounded reopening timeout required")
	}
	return s.reopenState, s.fail
}

func reopenedState() *pb.EvaluationState {
	old := finalState()
	var reviews []json.RawMessage
	_ = json.Unmarshal([]byte(old.ReviewsJson), &reviews)
	entry := reviewReopeningReceipt{SourceVersion: 8, Version: 9, TransitionCount: 3, PreviousFinalization: json.RawMessage(old.FinalizationJson), PreviousReviews: reviews, CandidateIDs: []string{"candidate:1"}, Actor: "user:42", Reason: "复核", ReopenedAt: "2026-09-13T01:00:01Z"}
	raw, _ := json.Marshal([]reviewReopeningReceipt{entry})
	return &pb.EvaluationState{RunId: old.RunId, Version: 9, Status: "awaiting_review", ReviewsJson: "[]", ResolutionsJson: "[]", ReopeningsJson: string(raw)}
}

func TestReopenForwardsOnceAndBindsAcceptedRound(t *testing.T) {
	rpc := &evaluationRPCStub{t: t, reopenState: reopenedState()}
	client := &EvaluationClient{RPC: rpc}
	scope := app.EvaluationScope{RunID: "run:1", OrganizationID: 7, OperatorUserID: 42}
	command := app.EvaluationReopen{ExpectedVersion: 8, Reason: "复核", Confirm: true}
	result, err := client.ReopenEvaluationReview(context.Background(), scope, command)
	if err != nil || result.Version != 9 || len(result.ReviewReopenings) == 0 || rpc.calls != 1 {
		t.Fatalf("%+v %v", result, err)
	}
	if rpc.reopenCommand.Scope.OperatorUserId != 42 || rpc.reopenCommand.ExpectedVersion != 8 || !rpc.reopenCommand.Confirm {
		t.Fatal("trusted command drift")
	}
	rpc.fail = context.DeadlineExceeded
	if _, err := client.ReopenEvaluationReview(context.Background(), scope, command); !errors.Is(err, context.DeadlineExceeded) || rpc.calls != 2 {
		t.Fatal("uncertain write retried", err)
	}
	rpc.fail = nil
	for _, mutate := range []func(){
		func() { scope.OperatorUserID = 43 },
		func() { command.Reason = "另一个理由" },
		func() { command.ExpectedVersion-- },
		func() { rpc.reopenState.ReopeningsJson = "[]" },
	} {
		scope.OperatorUserID = 42
		command = app.EvaluationReopen{ExpectedVersion: 8, Reason: "复核", Confirm: true}
		mutate()
		if _, err := client.ReopenEvaluationReview(context.Background(), scope, command); !errors.Is(err, app.ErrConflict) {
			t.Fatal("mismatched reopening accepted", err)
		}
	}
}

func TestEveryStateReadChecksBoundedHistoricalReceipts(t *testing.T) {
	for _, mutate := range []func(*pb.EvaluationState){
		func(r *pb.EvaluationState) { r.ReopeningsJson = "null" },
		func(r *pb.EvaluationState) { r.ReopeningsJson = strings.Repeat("x", 2*1024*1024+1) },
		func(r *pb.EvaluationState) { r.Version = 8 },
		func(r *pb.EvaluationState) { r.Status = "collecting" },
		func(r *pb.EvaluationState) { r.UnresolvedResultUnknownCount = 1 },
		func(r *pb.EvaluationState) {
			r.ReopeningsJson = strings.ReplaceAll(r.ReopeningsJson, `"source_version":8`, `"source_version":7`)
		},
		func(r *pb.EvaluationState) {
			r.ReopeningsJson = strings.ReplaceAll(r.ReopeningsJson, `"reopened_at":"2026-09-13T01:00:01Z"`, `"reopened_at":"2026-09-13T00:00:01Z"`)
		},
		func(r *pb.EvaluationState) {
			r.ReopeningsJson = strings.ReplaceAll(r.ReopeningsJson, `"candidate:1"`, `"candidate:1","candidate:1"`)
		},
		func(r *pb.EvaluationState) {
			var h []reviewReopeningReceipt
			_ = json.Unmarshal([]byte(r.ReopeningsJson), &h)
			h[0].PreviousReviews = h[0].PreviousReviews[:69]
			b, _ := json.Marshal(h)
			r.ReopeningsJson = string(b)
		},
	} {
		response := reopenedState()
		mutate(response)
		if _, err := state(response, app.EvaluationScope{RunID: "run:1"}); !errors.Is(err, app.ErrConflict) {
			t.Fatal("invalid history accepted", err)
		}
	}
	old := &pb.EvaluationState{RunId: "run:1", Version: 1, Status: "requested", ResolutionsJson: "[]"}
	if result, err := state(old, app.EvaluationScope{RunID: "run:1"}); err != nil || string(result.ReviewReopenings) != "[]" {
		t.Fatal("older AI compatibility lost", err)
	}
}
