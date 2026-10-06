package aibridge

import (
	"context"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
)

func (c *EvaluationClient) CorrectEvaluationReview(ctx context.Context, scope app.EvaluationScope, command app.EvaluationReviewCorrection) (app.EvaluationState, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	response, err := c.RPC.CorrectReview(ctx, &pb.EvaluationReviewCorrectionCommand{
		Scope: query(scope), CommandId: command.CommandID, ExpectedVersion: command.ExpectedVersion,
		Role: command.Role, CandidateId: command.CandidateID, PreviousReviewFingerprint: command.PreviousReviewFingerprint,
		CandidateOutputFingerprint: command.CandidateOutputFingerprint, Decision: command.Decision, Reason: command.Reason, Confirm: command.Confirm,
	})
	if err != nil {
		return app.EvaluationState{}, err
	}
	return state(response, scope)
}
