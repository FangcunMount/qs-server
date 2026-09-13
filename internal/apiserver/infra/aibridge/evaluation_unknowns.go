package aibridge

import (
	"context"
	"regexp"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

var unknownFailureCode = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,127}$`)

func (c *EvaluationClient) ListEvaluationUnknowns(ctx context.Context, scope app.EvaluationScope, expectedVersion int64) (app.EvaluationUnknownIndex, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	response, err := c.RPC.ListUnknownExecutions(ctx, &pb.EvaluationUnknownQuery{Scope: query(scope), ExpectedVersion: expectedVersion}, grpc.MaxCallRecvMsgSize(256*1024))
	if err != nil {
		return app.EvaluationUnknownIndex{}, err
	}
	if response == nil || response.RunId != scope.RunID || expectedVersion < 1 || response.Version != expectedVersion ||
		!gateReleaseFingerprint.MatchString(response.ReleaseFingerprint) || len(response.Executions) > 140 || proto.Size(response) > 256*1024 ||
		response.UnresolvedResultUnknownCount != int32(len(response.Executions)) {
		return app.EvaluationUnknownIndex{}, app.ErrConflict
	}
	switch response.Status {
	case "requested", "collecting", "blocked", "awaiting_review", "approved", "rejected", "canceled":
	default:
		return app.EvaluationUnknownIndex{}, app.ErrConflict
	}
	if (len(response.Executions) > 0 && response.Status != "blocked" && response.Status != "canceled") ||
		response.CanResolve != (response.Status == "blocked" && len(response.Executions) > 0) {
		return app.EvaluationUnknownIndex{}, app.ErrConflict
	}
	result := app.EvaluationUnknownIndex{RunID: response.RunId, Version: response.Version, ReleaseFingerprint: response.ReleaseFingerprint,
		Status: response.Status, UnresolvedResultUnknownCount: response.UnresolvedResultUnknownCount, CanResolve: response.CanResolve,
		Executions: make([]app.EvaluationUnknownExecution, 0, len(response.Executions))}
	executions, invocations := map[string]bool{}, map[string]bool{}
	for _, item := range response.Executions {
		if !validUnknownExecution(item, response.CanResolve) || executions[item.ExecutionId] || invocations[item.InvocationId] {
			return app.EvaluationUnknownIndex{}, app.ErrConflict
		}
		executions[item.ExecutionId], invocations[item.InvocationId] = true, true
		result.Executions = append(result.Executions, app.EvaluationUnknownExecution{
			ExecutionID: item.ExecutionId, InvocationID: item.InvocationId, Kind: item.Kind, CaseID: item.CaseId,
			SlotOrdinal: item.SlotOrdinal, CandidateID: item.CandidateId, ExecutionOrdinal: item.ExecutionOrdinal,
			StartedAt: item.StartedAt, FinishedAt: item.FinishedAt, ProviderCallCount: item.ProviderCallCount,
			FailureStage: item.FailureStage, FailureCode: item.FailureCode, TargetExecutionCount: item.TargetExecutionCount,
			TargetExecutionLimit: item.TargetExecutionLimit, StageExecutionCount: item.StageExecutionCount,
			StageExecutionLimit: item.StageExecutionLimit, ReplacementAllowed: item.ReplacementAllowed,
		})
	}
	return result, nil
}

func validUnknownExecution(item *pb.EvaluationUnknownExecution, canResolve bool) bool {
	if item == nil || !candidateIdentity.MatchString(item.ExecutionId) || !candidateIdentity.MatchString(item.InvocationId) ||
		!candidateIdentity.MatchString(item.CaseId) || item.SlotOrdinal < 1 || item.SlotOrdinal > 5 ||
		!unknownFailureCode.MatchString(item.FailureCode) || item.ProviderCallCount < 0 || item.ProviderCallCount > 1 {
		return false
	}
	switch item.Kind {
	case "generation":
		if item.CandidateId != "" || item.FailureStage != "generation_execution" {
			return false
		}
	case "semantic":
		if !candidateIdentity.MatchString(item.CandidateId) || item.FailureStage != "semantic_evaluation" {
			return false
		}
	default:
		return false
	}
	started, startErr := time.Parse(time.RFC3339Nano, item.StartedAt)
	finished, finishErr := time.Parse(time.RFC3339Nano, item.FinishedAt)
	if startErr != nil || finishErr != nil || finished.Before(started) {
		return false
	}
	if item.TargetExecutionLimit < 1 || item.TargetExecutionLimit > 2 || item.StageExecutionLimit < 1 || item.StageExecutionLimit > 70 ||
		item.ExecutionOrdinal < 1 || item.ExecutionOrdinal > item.TargetExecutionCount ||
		item.TargetExecutionCount > item.TargetExecutionLimit || item.StageExecutionCount < item.TargetExecutionCount ||
		item.StageExecutionCount > item.StageExecutionLimit {
		return false
	}
	// Eligibility is only a snapshot. AI rechecks frozen policy and current state on resolution.
	return item.ReplacementAllowed == (canResolve && item.TargetExecutionCount < item.TargetExecutionLimit && item.StageExecutionCount < item.StageExecutionLimit)
}
