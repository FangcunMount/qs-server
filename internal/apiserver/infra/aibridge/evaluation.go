package aibridge

import (
	"context"
	"encoding/json"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
)

// The supplied connection must use the existing QS workload mTLS configuration.
type EvaluationClient struct{ RPC pb.EvaluationManagementClient }

func NewEvaluationClient(conn grpc.ClientConnInterface) *EvaluationClient {
	return &EvaluationClient{RPC: pb.NewEvaluationManagementClient(conn)}
}
func query(scope app.EvaluationScope) *pb.EvaluationQuery {
	return &pb.EvaluationQuery{RunId: scope.RunID, OrganizationId: scope.OrganizationID, OperatorUserId: scope.OperatorUserID}
}
func state(response *pb.EvaluationState, scope app.EvaluationScope) (app.EvaluationState, error) {
	if response == nil || response.RunId != scope.RunID || response.Version < 1 || response.UnresolvedResultUnknownCount < 0 || !json.Valid([]byte(response.ResolutionsJson)) {
		return app.EvaluationState{}, app.ErrConflict
	}
	reviews := response.ReviewsJson
	if reviews == "" { // Older AI versions do not expose review history yet.
		reviews = "[]"
	}
	var history []json.RawMessage
	if json.Unmarshal([]byte(reviews), &history) != nil || history == nil || len(history) > 70 {
		return app.EvaluationState{}, app.ErrConflict
	}
	final, err := finalization(response)
	if err != nil || (len(final) > 0 && len(history) != 70) {
		return app.EvaluationState{}, app.ErrConflict
	}
	reopenings, err := reviewReopenings(response)
	if err != nil {
		return app.EvaluationState{}, err
	}
	return app.EvaluationState{RunID: response.RunId, Version: response.Version, Status: response.Status,
		UnresolvedResultUnknownCount: response.UnresolvedResultUnknownCount, Resolutions: json.RawMessage(response.ResolutionsJson), Reviews: json.RawMessage(reviews), Finalization: final, ReviewReopenings: reopenings}, nil
}
func (c *EvaluationClient) GetEvaluation(ctx context.Context, scope app.EvaluationScope) (app.EvaluationState, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	response, err := c.RPC.Get(ctx, query(scope))
	if err != nil {
		return app.EvaluationState{}, err
	}
	return state(response, scope)
}
func (c *EvaluationClient) ResolveUnknown(ctx context.Context, scope app.EvaluationScope, command app.UnknownResolution) (app.EvaluationState, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	response, err := c.RPC.ResolveUnknown(ctx, &pb.UnknownResolutionCommand{
		Scope: query(scope), ExpectedVersion: command.ExpectedVersion, ExecutionId: command.ExecutionID,
		Decision: command.Decision, Reason: command.Reason, Confirm: command.Confirm,
		AcknowledgedDuplicateCallAndCostRisk: command.AcknowledgedDuplicateCallAndCostRisk,
	})
	if err != nil {
		return app.EvaluationState{}, err
	}
	return state(response, scope)
}

func (c *EvaluationClient) StartEvaluation(ctx context.Context, scope app.EvaluationScope, command app.EvaluationStart) (app.EvaluationState, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	response, err := c.RPC.Start(ctx, &pb.EvaluationStartCommand{Scope: query(scope), ExpectedVersion: command.ExpectedVersion, Reason: command.Reason, Confirm: command.Confirm})
	if err != nil {
		return app.EvaluationState{}, err
	}
	return state(response, scope)
}
