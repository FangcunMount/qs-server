package execute

import (
	"context"
	"fmt"

	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	evaluation "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
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
	compat      evaluation.CompatibilityResolver
}

// NewRuntimeResolver creates the descriptor-only runtime resolver.
func NewRuntimeResolver(
	descriptors *evalpipeline.RuntimeDescriptorRegistry,
	executor evalpipeline.DescriptorExecutor,
) *RuntimeResolver {
	return &RuntimeResolver{
		descriptors: descriptors,
		executor:    executor,
		compat:      evaluation.NewCompatibilityResolver(),
	}
}

// ResolveExecution selects a runtime descriptor for one evaluated snapshot.
func (r *RuntimeResolver) ResolveExecution(a *assessment.Assessment, input *evaluationinput.InputSnapshot) (ResolvedExecution, error) {
	if r == nil || r.descriptors == nil || r.executor == nil {
		return ResolvedExecution{}, fmt.Errorf("evaluation runtime resolver is not configured")
	}
	executionIdentity := resolveExecutionIdentity(a, input)
	resolved := ResolvedExecution{ExecutionIdentity: executionIdentity}

	route, ok, frozen := resolveModelRoute(a, input, r.compat)
	if !ok {
		if frozen {
			return resolved, fmt.Errorf("evaluation runtime cannot resolve frozen model route")
		}
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
	return resolved, nil
}

// resolveModelRoute prefers InputSnapshot. Frozen RuntimeIdentity never falls back to
// Assessment ModelRef. Legacy routes may use Assessment ModelRef only as an explicit
// CompatibilityResolver source (EV-R008).
func resolveModelRoute(
	a *assessment.Assessment,
	input *evaluationinput.InputSnapshot,
	compat evaluation.CompatibilityResolver,
) (route evalpipeline.ModelRoute, ok bool, frozen bool) {
	frozen = input != nil && input.Model != nil && input.Model.HasFrozenRuntime()
	route, fromInput := modelRouteFromInput(input)
	if fromInput {
		if _, familyOK := evalpipeline.ExecutionFamilyFromRoute(route); familyOK {
			if frozen {
				return route, true, true
			}
			return compat.EnrichLegacyRoute(route), true, false
		}
		if frozen {
			return route, false, true
		}
	} else if frozen {
		return evalpipeline.ModelRoute{}, false, true
	}

	route, ok = modelRouteFromAssessment(a)
	if !ok {
		return evalpipeline.ModelRoute{}, false, false
	}
	evaluation.ObserveRuntimeCompat(evaluation.CompatibilityHit{
		Used:   true,
		Source: evaluation.CompatibilitySourceAssessmentModelRef,
	}, "route")
	return compat.EnrichLegacyRoute(route), true, false
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
