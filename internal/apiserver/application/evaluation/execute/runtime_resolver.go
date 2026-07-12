package execute

import (
	"context"
	"fmt"

	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// ResolvedExecution records the one descriptor route selected for an execution.
type ResolvedExecution struct {
	DescriptorKey     evalpipeline.DescriptorKey
	Descriptor        evalpipeline.RuntimeDescriptor
	ExecutionIdentity evaluation.ExecutionIdentity
}

// RuntimeResolver routes Evaluation exclusively through RuntimeDescriptor.
type RuntimeResolver struct {
	descriptors *evalpipeline.RuntimeDescriptorRegistry
	executor    evalpipeline.DescriptorExecutor
}

// NewRuntimeResolver creates the descriptor-only runtime resolver.
func NewRuntimeResolver(
	descriptors *evalpipeline.RuntimeDescriptorRegistry,
	executor evalpipeline.DescriptorExecutor,
) *RuntimeResolver {
	return &RuntimeResolver{
		descriptors: descriptors,
		executor:    executor,
	}
}

// ResolveExecution selects a runtime descriptor for one evaluated snapshot.
func (r *RuntimeResolver) ResolveExecution(a *assessment.Assessment, input *evaluationinput.InputSnapshot) (ResolvedExecution, error) {
	if r == nil || r.descriptors == nil || r.executor == nil {
		return ResolvedExecution{}, fmt.Errorf("evaluation runtime resolver is not configured")
	}
	executionIdentity := resolveExecutionIdentity(a, input)
	resolved := ResolvedExecution{ExecutionIdentity: executionIdentity}

	route, ok := modelRouteFromInput(input)
	if ok {
		_, ok = evalpipeline.ExecutionFamilyFromRoute(route)
	}
	if !ok {
		route, ok = modelRouteFromAssessment(a)
	}
	if !ok {
		return resolved, fmt.Errorf("evaluation runtime requires model route")
	}
	desc, err := r.descriptors.Resolve(route)
	if err != nil {
		return resolved, err
	}
	key, err := evalpipeline.DescriptorKeyFromRoute(route)
	if err != nil {
		return ResolvedExecution{}, err
	}
	resolved.DescriptorKey = key
	resolved.Descriptor = desc
	if canonicalIdentity, ok := canonicalExecutionIdentityForFamily(desc.AlgorithmFamily); ok {
		resolved.ExecutionIdentity = canonicalIdentity
	}
	return resolved, nil
}

// Execute runs Evaluation through the resolved descriptor pipeline.
func (r *RuntimeResolver) Execute(
	ctx context.Context,
	a *assessment.Assessment,
	input *evaluationinput.InputSnapshot,
) (*domainoutcome.Execution, ResolvedExecution, error) {
	resolved, err := r.ResolveExecution(a, input)
	if err != nil {
		return nil, ResolvedExecution{}, err
	}
	outcome, err := r.ExecuteResolved(ctx, resolved, a, input)
	return outcome, resolved, err
}

// ExecuteResolved executes exactly the descriptor selected by ResolveExecution.
func (r *RuntimeResolver) ExecuteResolved(
	ctx context.Context,
	resolved ResolvedExecution,
	a *assessment.Assessment,
	input *evaluationinput.InputSnapshot,
) (*domainoutcome.Execution, error) {
	if r == nil || r.executor == nil {
		return nil, fmt.Errorf("evaluation runtime resolver is not configured")
	}
	outcome, err := r.executor.Execute(ctx, resolved.Descriptor, evalpipeline.ExecutionInput{Assessment: a, Input: input})
	return outcome, err
}
