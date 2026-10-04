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

func canceledState(discard bool) *pb.EvaluationState {
	creation := creationFixture()
	response := creationState(creation)
	receipt := app.EvaluationCancellationReceipt{
		SchemaVersion: "qs-ai-evaluation-cancellation/v1", RunID: response.RunId,
		SourceVersion: 7, Version: 8, SourceStatus: "collecting", Status: "canceled",
		ReleaseFingerprint: creation.ReleaseFingerprint, Actor: "user:42", Reason: "停止后续工作",
		Discard: &discard, CanceledAt: "2026-09-13T02:00:00Z", ExecutionID: "execution:1", InvocationID: "invocation:1",
	}
	if discard {
		receipt.SourceStatus, receipt.SourceVersion, receipt.Version = "awaiting_review", 9, 10
		receipt.ExecutionID, receipt.InvocationID = "", ""
		rounds := reopenedState().ReopeningsJson
		rounds = strings.ReplaceAll(rounds, "run:1", response.RunId)
		response.ReopeningsJson = strings.ReplaceAll(rounds, "sha256:"+strings.Repeat("a", 64), creation.ReleaseFingerprint)
	}
	response.Status, response.Version = "canceled", receipt.Version
	raw, _ := json.Marshal(receipt)
	response.CancellationJson = string(raw)
	return response
}

type cancellationReadRPC struct {
	pb.EvaluationManagementClient
	t        *testing.T
	calls    int
	query    *pb.EvaluationQuery
	response *pb.EvaluationState
	fail     error
}

func (s *cancellationReadRPC) Get(ctx context.Context, query *pb.EvaluationQuery, _ ...grpc.CallOption) (*pb.EvaluationState, error) {
	s.calls++
	s.query = query
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 5*time.Second {
		s.t.Fatal("bounded query deadline required")
	}
	return s.response, s.fail
}
func TestCancellationReadPreservesDecisionWithoutWritingOrRetrying(t *testing.T) {
	for _, discard := range []bool{false, true} {
		rpc := &cancellationReadRPC{t: t, response: canceledState(discard)}
		client := &EvaluationClient{RPC: rpc}
		scope := app.EvaluationScope{RunID: rpc.response.RunId, OrganizationID: 7, OperatorUserID: 42}
		result, err := client.GetEvaluation(context.Background(), scope)
		if err != nil || result.Cancellation == nil || *result.Cancellation.Discard != discard || rpc.calls != 1 {
			t.Fatal("valid historical cancellation rejected", result, err)
		}
		if rpc.query.OperatorUserId != 42 || rpc.query.OrganizationId != 7 || rpc.query.RunId != scope.RunID {
			t.Fatal("trusted read scope lost")
		}
		if discard && string(result.ReviewReopenings) != rpc.response.ReopeningsJson {
			t.Fatal("discard erased historical reviews")
		}
		rpc.fail = context.DeadlineExceeded
		if _, err := client.GetEvaluation(context.Background(), scope); !errors.Is(err, context.DeadlineExceeded) || rpc.calls != 2 {
			t.Fatal("query uncertainty retried", err)
		}
	}
}

func TestCancellationReadPreservesOriginalActorAndChecksSourceBindings(t *testing.T) {
	for name, mutate := range map[string]func(*app.EvaluationCancellationReceipt){
		"schema":      func(r *app.EvaluationCancellationReceipt) { r.SchemaVersion = "unknown" },
		"run":         func(r *app.EvaluationCancellationReceipt) { r.RunID = "another" },
		"version":     func(r *app.EvaluationCancellationReceipt) { r.Version++ },
		"source":      func(r *app.EvaluationCancellationReceipt) { r.SourceVersion-- },
		"terminal":    func(r *app.EvaluationCancellationReceipt) { r.SourceStatus = "approved" },
		"discard":     func(r *app.EvaluationCancellationReceipt) { r.Discard = nil },
		"actor":       func(r *app.EvaluationCancellationReceipt) { r.Actor = "system:worker" },
		"reason":      func(r *app.EvaluationCancellationReceipt) { r.Reason = " " },
		"fingerprint": func(r *app.EvaluationCancellationReceipt) { r.ReleaseFingerprint = "sha256:" + strings.Repeat("f", 64) },
		"time":        func(r *app.EvaluationCancellationReceipt) { r.CanceledAt = "2026-09-13T00:00:00Z" },
		"execution":   func(r *app.EvaluationCancellationReceipt) { r.ExecutionID = "execution:other" },
	} {
		t.Run(name, func(t *testing.T) {
			response := canceledState(true)
			var receipt app.EvaluationCancellationReceipt
			_ = json.Unmarshal([]byte(response.CancellationJson), &receipt)
			mutate(&receipt)
			raw, _ := json.Marshal(receipt)
			response.CancellationJson = string(raw)
			if _, err := state(response, app.EvaluationScope{RunID: response.RunId}); !errors.Is(err, app.ErrConflict) {
				t.Fatal("corrupt receipt accepted", err)
			}
		})
	}
	response := canceledState(true)
	view, err := state(response, app.EvaluationScope{RunID: response.RunId, OperatorUserID: 99})
	if err != nil || view.Cancellation.Actor != "user:42" {
		t.Fatal("auditor replaced original actor", err)
	}
	response.CancellationJson = ""
	if _, err := state(response, app.EvaluationScope{RunID: response.RunId}); !errors.Is(err, app.ErrConflict) {
		t.Fatal("canceled review history requires original cancellation evidence")
	}
	response = &pb.EvaluationState{ReviewsJson: "[]", ReopeningsJson: "[]", RunId: "run:1", Version: 7, Status: "canceled", ResolutionsJson: "[]"}
	if view, err := state(response, app.EvaluationScope{RunID: "run:1"}); err != nil || view.Cancellation != nil {
		t.Fatal("older cancellation compatibility lost", err)
	}
}

func TestCancellationDrainRequestSurvivesLaterProjectionVersions(t *testing.T) {
	response := creationState(creationFixture())
	response.Version, response.Status, response.CancelDraining = 12, "collecting", true
	response.ExecutionMode, response.ActiveCallCount, response.ParallelCallLimit = "candidate_v2", 2, 3
	request := app.EvaluationCancelRequest{SchemaVersion: "qs-ai-evaluation-cancel-request/v1", RunID: response.RunId,
		SourceVersion: 7, Version: 8, Status: "cancel_requested", Actor: "user:42", Reason: "停止后续工作", RequestedAt: "2026-09-13T02:00:00Z"}
	raw, _ := json.Marshal(request)
	response.CancelRequestJson = string(raw)
	rpc := &cancellationReadRPC{t: t, response: response}
	client := &EvaluationClient{RPC: rpc}
	result, err := client.GetEvaluation(context.Background(), app.EvaluationScope{RunID: response.RunId, OrganizationID: 7, OperatorUserID: 42})
	if err != nil || result.CancelRequest == nil || result.Cancellation != nil || !result.CancelDraining || result.ActiveCallCount != 2 {
		t.Fatal(result, err)
	}
	response.CancelDraining = false
	if _, err := state(response, app.EvaluationScope{RunID: response.RunId}); !errors.Is(err, app.ErrConflict) {
		t.Fatal("inconsistent drain accepted", err)
	}
}
