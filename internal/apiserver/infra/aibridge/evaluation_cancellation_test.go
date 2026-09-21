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

func (s *evaluationRPCStub) Cancel(ctx context.Context, command *pb.EvaluationCancelCommand, _ ...grpc.CallOption) (*pb.EvaluationState, error) {
	s.calls++
	s.cancelCommand = command
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 5*time.Second {
		s.t.Fatal("bounded cancellation deadline required")
	}
	return s.cancelState, s.fail
}

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

func TestCancelForwardsOnceAndMatchesOriginalDecision(t *testing.T) {
	for _, discard := range []bool{false, true} {
		rpc := &evaluationRPCStub{t: t, cancelState: canceledState(discard)}
		client := &EvaluationClient{RPC: rpc}
		scope := app.EvaluationScope{RunID: rpc.cancelState.RunId, OrganizationID: 7, OperatorUserID: 42}
		command := app.EvaluationCancel{ExpectedVersion: rpc.cancelState.Version - 1, Reason: "停止后续工作", Confirm: true, Discard: &discard}
		result, err := client.CancelEvaluation(context.Background(), scope, command)
		if err != nil || result.Cancellation == nil || *result.Cancellation.Discard != discard || rpc.calls != 1 {
			t.Fatal("valid cancellation receipt rejected", result, err)
		}
		if rpc.cancelCommand.Scope.OperatorUserId != 42 || rpc.cancelCommand.Scope.OrganizationId != 7 || rpc.cancelCommand.Discard == nil || *rpc.cancelCommand.Discard != discard {
			t.Fatal("scope or explicit decision lost")
		}
		if discard && string(result.ReviewReopenings) != rpc.cancelState.ReopeningsJson {
			t.Fatal("discard erased historical reviews")
		}
		rpc.fail = context.DeadlineExceeded
		if _, err := client.CancelEvaluation(context.Background(), scope, command); !errors.Is(err, context.DeadlineExceeded) || rpc.calls != 2 {
			t.Fatal("uncertain cancellation retried", err)
		}
		rpc.fail = nil
		for _, mutate := range []func(*app.EvaluationScope, *app.EvaluationCancel){
			func(s *app.EvaluationScope, _ *app.EvaluationCancel) { s.OperatorUserID++ },
			func(_ *app.EvaluationScope, c *app.EvaluationCancel) { c.ExpectedVersion-- },
			func(_ *app.EvaluationScope, c *app.EvaluationCancel) { c.Reason = "另一次操作" },
			func(_ *app.EvaluationScope, c *app.EvaluationCancel) { opposite := !discard; c.Discard = &opposite },
		} {
			s, c := scope, command
			mutate(&s, &c)
			if _, err := client.CancelEvaluation(context.Background(), s, c); !errors.Is(err, app.ErrConflict) {
				t.Fatal("mismatched cancellation accepted", err)
			}
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
	rpc := &evaluationRPCStub{t: t, cancelState: response}
	client := &EvaluationClient{RPC: rpc}
	discard := false
	result, err := client.CancelEvaluation(context.Background(), app.EvaluationScope{RunID: response.RunId, OrganizationID: 7, OperatorUserID: 42},
		app.EvaluationCancel{ExpectedVersion: 7, Reason: request.Reason, Confirm: true, Discard: &discard})
	if err != nil || result.CancelRequest == nil || result.Cancellation != nil || !result.CancelDraining || result.ActiveCallCount != 2 {
		t.Fatal(result, err)
	}
	response.CancelDraining = false
	if _, err := state(response, app.EvaluationScope{RunID: response.RunId}); !errors.Is(err, app.ErrConflict) {
		t.Fatal("inconsistent drain accepted", err)
	}
}
