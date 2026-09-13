package aibridge

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

func (s *evaluationRPCStub) ListUnknownExecutions(ctx context.Context, q *pb.EvaluationUnknownQuery, options ...grpc.CallOption) (*pb.EvaluationUnknownIndex, error) {
	s.calls++
	s.unknownQuery = q
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 5*time.Second {
		s.t.Fatal("missing bounded deadline")
	}
	bounded := false
	for _, option := range options {
		if value, ok := option.(grpc.MaxRecvMsgSizeCallOption); ok && value.MaxRecvMsgSize == 256*1024 {
			bounded = true
		}
	}
	if !bounded {
		s.t.Fatal("missing receive bound")
	}
	return s.unknownIndex, s.fail
}
func unknownIndex() *pb.EvaluationUnknownIndex {
	return &pb.EvaluationUnknownIndex{RunId: "run:1", Version: 7, Status: "blocked", ReleaseFingerprint: "sha256:" + strings.Repeat("a", 64), UnresolvedResultUnknownCount: 1, CanResolve: true,
		Executions: []*pb.EvaluationUnknownExecution{{ExecutionId: "generation:1", InvocationId: "invocation:1", Kind: "generation", CaseId: "case:1", SlotOrdinal: 1, ExecutionOrdinal: 1,
			StartedAt: "2026-09-13T00:00:00+00:00", FinishedAt: "2026-09-13T00:01:00Z", ProviderCallCount: 1, FailureStage: "generation_execution", FailureCode: "provider_timeout",
			TargetExecutionCount: 1, TargetExecutionLimit: 2, StageExecutionCount: 1, StageExecutionLimit: 70, ReplacementAllowed: true}}}
}
func TestUnknownQueryPreservesScopeEvidenceAndDoesNotRetry(t *testing.T) {
	rpc := &evaluationRPCStub{t: t, unknownIndex: unknownIndex()}
	client := &EvaluationClient{RPC: rpc}
	scope := app.EvaluationScope{RunID: "run:1", OrganizationID: 7, OperatorUserID: 42}
	got, err := client.ListEvaluationUnknowns(context.Background(), scope, 7)
	if err != nil || got.Version != 7 || len(got.Executions) != 1 || got.Executions[0].ExecutionID != "generation:1" || !got.Executions[0].ReplacementAllowed {
		t.Fatalf("%+v %v", got, err)
	}
	if rpc.calls != 1 || rpc.unknownQuery.Scope.OrganizationId != 7 || rpc.unknownQuery.Scope.OperatorUserId != 42 || rpc.unknownQuery.ExpectedVersion != 7 {
		t.Fatal("scope/version drift")
	}
	rpc.fail = context.DeadlineExceeded
	if _, err = client.ListEvaluationUnknowns(context.Background(), scope, 7); !errors.Is(err, context.DeadlineExceeded) || rpc.calls != 2 {
		t.Fatal("read error changed or automatically retried")
	}
}
func TestUnknownQueryRejectsInconsistentEvidence(t *testing.T) {
	mutations := map[string]func(*pb.EvaluationUnknownIndex){
		"run":             func(v *pb.EvaluationUnknownIndex) { v.RunId = "foreign" },
		"version":         func(v *pb.EvaluationUnknownIndex) { v.Version++ },
		"fingerprint":     func(v *pb.EvaluationUnknownIndex) { v.ReleaseFingerprint = "bad" },
		"count":           func(v *pb.EvaluationUnknownIndex) { v.UnresolvedResultUnknownCount = 0 },
		"status":          func(v *pb.EvaluationUnknownIndex) { v.Status = "collecting" },
		"unknown status":  func(v *pb.EvaluationUnknownIndex) { v.Status = "unknown" },
		"resolution flag": func(v *pb.EvaluationUnknownIndex) { v.CanResolve = false },
		"nil item":        func(v *pb.EvaluationUnknownIndex) { v.Executions[0] = nil },
		"duplicate execution": func(v *pb.EvaluationUnknownIndex) {
			v.Executions = append(v.Executions, proto.Clone(v.Executions[0]).(*pb.EvaluationUnknownExecution))
			v.Executions[1].InvocationId = "invocation:2"
			v.UnresolvedResultUnknownCount = 2
		},
		"duplicate invocation": func(v *pb.EvaluationUnknownIndex) {
			v.Executions = append(v.Executions, proto.Clone(v.Executions[0]).(*pb.EvaluationUnknownExecution))
			v.Executions[1].ExecutionId = "generation:2"
			v.UnresolvedResultUnknownCount = 2
		},
		"identity":             func(v *pb.EvaluationUnknownIndex) { v.Executions[0].ExecutionId = "bad id" },
		"stage":                func(v *pb.EvaluationUnknownIndex) { v.Executions[0].FailureStage = "semantic_evaluation" },
		"kind":                 func(v *pb.EvaluationUnknownIndex) { v.Executions[0].Kind = "other" },
		"generation candidate": func(v *pb.EvaluationUnknownIndex) { v.Executions[0].CandidateId = "candidate:1" },
		"semantic candidate": func(v *pb.EvaluationUnknownIndex) {
			v.Executions[0].Kind = "semantic"
			v.Executions[0].FailureStage = "semantic_evaluation"
		},
		"slot":              func(v *pb.EvaluationUnknownIndex) { v.Executions[0].SlotOrdinal = 6 },
		"failure body":      func(v *pb.EvaluationUnknownIndex) { v.Executions[0].FailureCode = "raw provider message" },
		"time":              func(v *pb.EvaluationUnknownIndex) { v.Executions[0].StartedAt = "invalid" },
		"backward time":     func(v *pb.EvaluationUnknownIndex) { v.Executions[0].FinishedAt = "2026-09-12T23:00:00Z" },
		"calls":             func(v *pb.EvaluationUnknownIndex) { v.Executions[0].ProviderCallCount = 2 },
		"ordinal":           func(v *pb.EvaluationUnknownIndex) { v.Executions[0].ExecutionOrdinal = 2 },
		"target limit":      func(v *pb.EvaluationUnknownIndex) { v.Executions[0].TargetExecutionLimit = 3 },
		"stage limit":       func(v *pb.EvaluationUnknownIndex) { v.Executions[0].StageExecutionLimit = 71 },
		"stage count":       func(v *pb.EvaluationUnknownIndex) { v.Executions[0].StageExecutionCount = 0 },
		"budget exhausted":  func(v *pb.EvaluationUnknownIndex) { v.Executions[0].TargetExecutionCount = 2 },
		"wrong eligibility": func(v *pb.EvaluationUnknownIndex) { v.Executions[0].ReplacementAllowed = false },
		"excessive entries": func(v *pb.EvaluationUnknownIndex) {
			for len(v.Executions) < 141 {
				v.Executions = append(v.Executions, proto.Clone(v.Executions[0]).(*pb.EvaluationUnknownExecution))
			}
			v.UnresolvedResultUnknownCount = 141
		},
		"response size": func(v *pb.EvaluationUnknownIndex) { v.Executions[0].FailureCode = strings.Repeat("x", 256*1024) },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			value := unknownIndex()
			mutate(value)
			rpc := &evaluationRPCStub{t: t, unknownIndex: value}
			_, err := (&EvaluationClient{RPC: rpc}).ListEvaluationUnknowns(context.Background(), app.EvaluationScope{RunID: "run:1"}, 7)
			if !errors.Is(err, app.ErrConflict) || rpc.calls != 1 {
				t.Fatalf("%v calls=%d", err, rpc.calls)
			}
		})
	}
	rpc := &evaluationRPCStub{t: t}
	if _, err := (&EvaluationClient{RPC: rpc}).ListEvaluationUnknowns(context.Background(), app.EvaluationScope{RunID: "run:1"}, 7); !errors.Is(err, app.ErrConflict) {
		t.Fatal(err)
	}
}
func TestUnknownQuerySupportsSemanticExhaustedAndCanceledAudit(t *testing.T) {
	for _, state := range []string{"blocked", "canceled", "collecting"} {
		t.Run(state, func(t *testing.T) {
			value := unknownIndex()
			value.Status = state
			value.CanResolve = state == "blocked"
			item := value.Executions[0]
			item.Kind = "semantic"
			item.CandidateId = "candidate:1"
			item.FailureStage = "semantic_evaluation"
			item.TargetExecutionCount = 2
			item.StageExecutionCount = 2
			item.ReplacementAllowed = false
			if state == "collecting" {
				value.Executions = nil
				value.UnresolvedResultUnknownCount = 0
			}
			got, err := (&EvaluationClient{RPC: &evaluationRPCStub{t: t, unknownIndex: value}}).ListEvaluationUnknowns(context.Background(), app.EvaluationScope{RunID: "run:1"}, 7)
			if err != nil || got.CanResolve != value.CanResolve || got.Executions == nil {
				t.Fatalf("%+v %v", got, err)
			}
		})
	}
}
