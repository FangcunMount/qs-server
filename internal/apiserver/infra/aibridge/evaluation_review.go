package aibridge

import (
	"context"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"time"
)

func (c *EvaluationClient) ReviewEvaluation(ctx context.Context, scope app.EvaluationScope, command app.EvaluationReview) (app.EvaluationState, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	request := &pb.EvaluationReviewCommand{Scope: query(scope), ExpectedVersion: command.ExpectedVersion, Role: command.Role}
	for _, item := range command.Reviews {
		value := &pb.CandidateReviewItem{CandidateId: item.CandidateID, Decision: item.Decision, Reason: item.Reason}
		if r := item.SemanticReview; r != nil {
			value.SemanticReview = &pb.SemanticContradictionReview{PolicyVersion: r.PolicyVersion, ExecutionId: r.ExecutionID,
				OutputFingerprint: r.OutputFingerprint, AssertionOrdinal: r.AssertionOrdinal,
				OriginalDetail: r.OriginalDetail, CandidateExcerpt: r.CandidateExcerpt, Reason: r.Reason}
		}
		request.Reviews = append(request.Reviews, value)
	}
	response, err := c.RPC.Review(ctx, request)
	if err != nil {
		return app.EvaluationState{}, err
	}
	return state(response, scope)
}
