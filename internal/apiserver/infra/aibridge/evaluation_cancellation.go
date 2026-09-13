package aibridge

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
)

// Check transport bindings; AI owns cancellation eligibility and original ledger validation.
func evaluationCancellation(response *pb.EvaluationState, creation *app.EvaluationCreationReceipt) (*app.EvaluationCancellationReceipt, error) {
	raw := response.CancellationJson
	if raw == "" { // Existing unknown-result cancellations and older AI servers have no receipt.
		return nil, nil
	}
	var receipt app.EvaluationCancellationReceipt
	if len(raw) > 8192 || !utf8.ValidString(raw) || json.Unmarshal([]byte(raw), &receipt) != nil ||
		receipt.SchemaVersion != "qs-ai-evaluation-cancellation/v1" || receipt.RunID != response.RunId ||
		receipt.SourceVersion < 1 || receipt.SourceVersion != response.Version-1 || receipt.Version != response.Version ||
		receipt.Status != "canceled" || response.Status != "canceled" || receipt.Discard == nil ||
		response.UnresolvedResultUnknownCount != 0 || response.FinalizationJson != "" ||
		creation == nil || receipt.ReleaseFingerprint != creation.ReleaseFingerprint ||
		strings.TrimSpace(receipt.Reason) == "" || receipt.Reason != strings.TrimSpace(receipt.Reason) || len(receipt.Reason) > 1000 || strings.ContainsAny(receipt.Reason, "<>") {
		return nil, app.ErrConflict
	}
	switch receipt.SourceStatus {
	case "requested", "collecting", "blocked", "awaiting_review":
	default:
		return nil, app.ErrConflict
	}
	actor, err := strconv.ParseInt(strings.TrimPrefix(receipt.Actor, "user:"), 10, 64)
	if err != nil || actor < 1 || receipt.Actor != fmt.Sprintf("user:%d", actor) || *receipt.Discard != (receipt.SourceStatus == "awaiting_review") {
		return nil, app.ErrConflict
	}
	if (receipt.ExecutionID == "") != (receipt.InvocationID == "") ||
		(receipt.ExecutionID != "" && (receipt.SourceStatus != "collecting" || !creationActor.MatchString(receipt.ExecutionID) || !creationActor.MatchString(receipt.InvocationID))) {
		return nil, app.ErrConflict
	}
	at, err := time.Parse(time.RFC3339Nano, receipt.CanceledAt)
	created, createErr := time.Parse(time.RFC3339Nano, creation.CreatedAt)
	if err != nil || createErr != nil || at.Before(created) {
		return nil, app.ErrConflict
	}
	return &receipt, nil
}

func (c *EvaluationClient) CancelEvaluation(ctx context.Context, scope app.EvaluationScope, command app.EvaluationCancel) (app.EvaluationState, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	response, err := c.RPC.Cancel(ctx, &pb.EvaluationCancelCommand{
		Scope: query(scope), ExpectedVersion: command.ExpectedVersion, Reason: command.Reason,
		Confirm: command.Confirm, Discard: command.Discard,
	}, grpc.MaxCallRecvMsgSize(4*1024*1024))
	if err != nil {
		return app.EvaluationState{}, err
	}
	result, err := state(response, scope)
	if err != nil {
		return app.EvaluationState{}, err
	}
	receipt := result.Cancellation
	if receipt == nil || command.Discard == nil || receipt.SourceVersion != command.ExpectedVersion ||
		receipt.Actor != fmt.Sprintf("user:%d", scope.OperatorUserID) || receipt.Reason != command.Reason || *receipt.Discard != *command.Discard {
		return app.EvaluationState{}, app.ErrConflict
	}
	return result, nil
}
