package execute

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// ResolvedExecution captures runtime descriptor routing and the legacy evaluator key.
type ResolvedExecution struct {
	DescriptorKey  evalpipeline.RuntimeDescriptorKey
	Descriptor     evalpipeline.RuntimeDescriptor
	EvaluatorKey   evaluation.EvaluatorKey
	UsedDescriptor bool
}

// RuntimeResolver routes evaluation execution through runtime descriptors with EvaluatorKey fallback.
type RuntimeResolver struct {
	descriptors *evalpipeline.RuntimeDescriptorRegistry
	evaluators  EvaluatorRegistry
}

// NewRuntimeResolver creates a resolver backed by descriptor and evaluator registries.
func NewRuntimeResolver(
	descriptors *evalpipeline.RuntimeDescriptorRegistry,
	evaluators EvaluatorRegistry,
) *RuntimeResolver {
	return &RuntimeResolver{descriptors: descriptors, evaluators: evaluators}
}

// ResolveExecution selects the runtime descriptor and evaluator key for one evaluation.
func (r *RuntimeResolver) ResolveExecution(a *assessment.Assessment, input *evaluationinput.InputSnapshot) (ResolvedExecution, error) {
	if r == nil || r.evaluators == nil {
		return ResolvedExecution{}, fmt.Errorf("evaluation runtime resolver is not configured")
	}
	evaluatorKey := resolveEvaluatorKey(a, input)
	resolved := ResolvedExecution{EvaluatorKey: evaluatorKey}

	snapshot, ok := publishedSnapshotFromInput(input)
	if ok && r.descriptors != nil {
		desc, err := r.descriptors.Resolve(snapshot)
		if err == nil {
			key, keyErr := evalpipeline.RuntimeDescriptorKeyFromSnapshot(snapshot)
			if keyErr != nil {
				return ResolvedExecution{}, keyErr
			}
			resolved.DescriptorKey = key
			resolved.Descriptor = desc
			resolved.UsedDescriptor = true
			if _, err := r.evaluators.Resolve(evaluatorKey); err != nil {
				return ResolvedExecution{}, err
			}
			return resolved, nil
		}
	}

	if _, err := r.evaluators.Resolve(evaluatorKey); err != nil {
		return ResolvedExecution{}, err
	}
	return resolved, nil
}

// Execute runs evaluation using the descriptor-primary path with EvaluatorKey dispatch.
func (r *RuntimeResolver) Execute(
	ctx context.Context,
	a *assessment.Assessment,
	input *evaluationinput.InputSnapshot,
) (*assessment.AssessmentOutcome, ResolvedExecution, error) {
	resolved, err := r.ResolveExecution(a, input)
	if err != nil {
		return nil, ResolvedExecution{}, err
	}
	evaluator, err := r.evaluators.Resolve(resolved.EvaluatorKey)
	if err != nil {
		return nil, resolved, err
	}
	outcome, err := evaluator.Execute(ctx, ExecutionInput{Assessment: a, Input: input})
	return outcome, resolved, err
}
