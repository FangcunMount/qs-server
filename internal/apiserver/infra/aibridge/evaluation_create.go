package aibridge

import (
	"context"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"time"
)

func frozenRef(r app.FrozenEvaluationRef) *pb.FrozenEvaluationRef {
	return &pb.FrozenEvaluationRef{Id: r.ID, Version: r.Version, Fingerprint: r.Fingerprint}
}
func (c *EvaluationClient) CreateEvaluation(ctx context.Context, scope app.EvaluationScope, command app.EvaluationCreate) (app.EvaluationState, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	release := command.Release
	response, err := c.RPC.Create(ctx, &pb.EvaluationCreateCommand{
		Scope: query(scope), Reason: command.Reason, Confirm: command.Confirm,
		Release: &pb.EvaluationRelease{
			Suite:                frozenRef(release.Suite),
			Prompt:               frozenRef(release.Prompt),
			Profile:              frozenRef(release.Profile),
			InputSchema:          frozenRef(release.InputSchema),
			OutputSchema:         frozenRef(release.OutputSchema),
			GenerationRoute:      frozenRef(release.GenerationRoute),
			SemanticPrompt:       frozenRef(release.SemanticPrompt),
			SemanticOutputSchema: frozenRef(release.SemanticOutputSchema),
			SemanticRoute:        frozenRef(release.SemanticRoute),
			ExecutionPolicy:      frozenRef(release.ExecutionPolicy),
			GatePolicy:           frozenRef(release.GatePolicy),
		}})
	if err != nil {
		return app.EvaluationState{}, err
	}
	return state(response, scope)
}
