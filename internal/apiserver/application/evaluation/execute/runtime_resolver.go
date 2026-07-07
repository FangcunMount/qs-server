package execute

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// ResolvedExecution captures runtime descriptor routing and the legacy execution identity.
type ResolvedExecution struct {
	DescriptorKey     evalpipeline.RuntimeDescriptorKey
	Descriptor        evalpipeline.RuntimeDescriptor
	ExecutionIdentity evaluation.ExecutionIdentity
	UsedDescriptor    bool
}

// RuntimeResolver routes evaluation execution through runtime descriptors with EvaluatorKey fallback.
type RuntimeResolver struct {
	descriptors      *evalpipeline.RuntimeDescriptorRegistry
	evaluators       EvaluatorRegistry
	familyEvaluators map[modelcatalog.AlgorithmFamily]Evaluator
}

// NewRuntimeResolver creates a resolver backed by descriptor and evaluator registries.
func NewRuntimeResolver(
	descriptors *evalpipeline.RuntimeDescriptorRegistry,
	evaluators EvaluatorRegistry,
	familyEvaluators map[modelcatalog.AlgorithmFamily]Evaluator,
) *RuntimeResolver {
	return &RuntimeResolver{
		descriptors:      descriptors,
		evaluators:       evaluators,
		familyEvaluators: familyEvaluators,
	}
}

// ResolveExecution selects the runtime descriptor and evaluator key for one evaluation.
func (r *RuntimeResolver) ResolveExecution(a *assessment.Assessment, input *evaluationinput.InputSnapshot) (ResolvedExecution, error) {
	if r == nil || r.evaluators == nil {
		return ResolvedExecution{}, fmt.Errorf("evaluation runtime resolver is not configured")
	}
	executionIdentity := resolveExecutionIdentity(a, input)
	resolved := ResolvedExecution{ExecutionIdentity: executionIdentity}

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
			if canonicalIdentity, ok := canonicalExecutionIdentityForFamily(desc.AlgorithmFamily); ok {
				resolved.ExecutionIdentity = canonicalIdentity
			}
			if _, err := r.resolveEvaluator(resolved); err != nil {
				return ResolvedExecution{}, err
			}
			return resolved, nil
		}
	}

	if _, err := r.evaluators.Resolve(executionIdentity); err != nil {
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
	evaluator, err := r.resolveEvaluator(resolved)
	if err != nil {
		return nil, resolved, err
	}
	outcome, err := evaluator.Execute(ctx, ExecutionInput{Assessment: a, Input: input})
	return outcome, resolved, err
}

func (r *RuntimeResolver) resolveEvaluator(resolved ResolvedExecution) (Evaluator, error) {
	if resolved.UsedDescriptor {
		if r.familyEvaluators != nil {
			if evaluator, ok := r.familyEvaluators[resolved.Descriptor.AlgorithmFamily]; ok {
				return evaluator, nil
			}
			return nil, fmt.Errorf("unsupported evaluation algorithm family: %s", resolved.Descriptor.AlgorithmFamily)
		}
	}
	return r.evaluators.Resolve(resolved.ExecutionIdentity)
}
