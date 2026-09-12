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

func (s *evaluationRPCStub) Finalize(ctx context.Context, command *pb.EvaluationFinalizeCommand, _ ...grpc.CallOption) (*pb.EvaluationState, error) {
	s.calls++
	s.finalCommand = command
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 5*time.Second {
		s.t.Fatal("bounded finalization timeout required")
	}
	return s.finalState, s.fail
}

func finalState() *pb.EvaluationState {
	response := &pb.EvaluationState{RunId: "run:1", Version: 8, Status: "rejected", ResolutionsJson: "[]"}
	reviews := make([]map[string]string, 70)
	for i := range reviews {
		reviews[i] = map[string]string{}
	}
	raw, _ := json.Marshal(reviews)
	response.ReviewsJson = string(raw)
	data := map[string]any{
		"schema_version": "qs-ai-evaluation-finalization/v1", "run_id": "run:1", "version": 8, "source_version": 7,
		"status": "rejected", "passed": false, "actor": "user:42", "reason": "核对后拒绝",
		"release_fingerprint": "sha256:" + strings.Repeat("a", 64), "finalized_at": "2026-09-13T01:00:00Z",
		"gate_result": map[string]any{"evaluated_at": "2026-09-13T01:00:00Z", "gate_passes": map[string]bool{"G1": true, "G2": true, "G3": true, "G4": false, "G5": true},
			"metrics": []any{}, "reasons": []any{}, "semantic_adjudications": []any{}},
	}
	raw, _ = json.Marshal(data)
	response.FinalizationJson = string(raw)
	return response
}

func TestFinalizeForwardsOnceAndVerifiesDecisionReceipt(t *testing.T) {
	rpc := &evaluationRPCStub{t: t, finalState: finalState()}
	client := &EvaluationClient{RPC: rpc}
	scope := app.EvaluationScope{RunID: "run:1", OrganizationID: 7, OperatorUserID: 42}
	passed := false
	command := app.EvaluationFinalize{ExpectedVersion: 7, ExpectedPassed: &passed, Reason: "核对后拒绝", Confirm: true}
	result, err := client.FinalizeEvaluation(context.Background(), scope, command)
	if err != nil || result.Status != "rejected" || len(result.Finalization) == 0 || rpc.calls != 1 {
		t.Fatalf("%+v %v", result, err)
	}
	if rpc.finalCommand.ExpectedPassed == nil || *rpc.finalCommand.ExpectedPassed || rpc.finalCommand.Scope.OperatorUserId != 42 || !rpc.finalCommand.Confirm {
		t.Fatal("explicit decision and trusted scope drift")
	}
	rpc.fail = context.DeadlineExceeded
	if _, err := client.FinalizeEvaluation(context.Background(), scope, command); !errors.Is(err, context.DeadlineExceeded) || rpc.calls != 2 {
		t.Fatal("uncertain write must not retry", err)
	}
	rpc.fail = nil
	scope.OperatorUserID = 43
	if _, err := client.FinalizeEvaluation(context.Background(), scope, command); !errors.Is(err, app.ErrConflict) {
		t.Fatal("mismatched receipt actor accepted", err)
	}
}

func TestFinalizedStateRequiresBoundCompleteReceipt(t *testing.T) {
	scope := app.EvaluationScope{RunID: "run:1"}
	for _, mutate := range []func(*pb.EvaluationState){
		func(r *pb.EvaluationState) { r.FinalizationJson = "" },
		func(r *pb.EvaluationState) { r.FinalizationJson = "null" },
		func(r *pb.EvaluationState) { r.Status = "approved" },
		func(r *pb.EvaluationState) { r.Status = "awaiting_review" },
		func(r *pb.EvaluationState) { r.Version++ },
		func(r *pb.EvaluationState) { r.ReviewsJson = "[]" },
		func(r *pb.EvaluationState) { r.UnresolvedResultUnknownCount = 1 },
		func(r *pb.EvaluationState) {
			r.FinalizationJson = strings.ReplaceAll(r.FinalizationJson, `"user:42"`, `"user:"`)
		},
		func(r *pb.EvaluationState) {
			r.FinalizationJson = strings.ReplaceAll(r.FinalizationJson, `"G4":false`, `"G4":null`)
		},
		func(r *pb.EvaluationState) {
			r.FinalizationJson = strings.ReplaceAll(r.FinalizationJson, `"passed":false`, `"passed":true`)
		},
		func(r *pb.EvaluationState) { r.FinalizationJson = strings.Repeat("x", 256*1024+1) },
	} {
		response := finalState()
		mutate(response)
		if _, err := state(response, scope); !errors.Is(err, app.ErrConflict) {
			t.Fatal("invalid finalization receipt accepted", err)
		}
	}
}
