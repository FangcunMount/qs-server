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

// ResolvedExecution 记录运行时描述符 路由 和 旧版 执行身份。
type ResolvedExecution struct {
	DescriptorKey     evalpipeline.RuntimeDescriptorKey
	Descriptor        evalpipeline.RuntimeDescriptor
	ExecutionIdentity evaluation.ExecutionIdentity
	UsedDescriptor    bool
}

// RuntimeResolver 路由评估执行 通过 运行时描述符 使用 Evaluator键 fallback。
type RuntimeResolver struct {
	descriptors         *evalpipeline.RuntimeDescriptorRegistry
	evaluators          EvaluatorRegistry
	descriptorExecutors map[modelcatalog.AlgorithmFamily]DescriptorExecutor
}

// NewRuntimeResolver 创建resolver 基于 描述符 和 evaluator 注册表。
func NewRuntimeResolver(
	descriptors *evalpipeline.RuntimeDescriptorRegistry,
	evaluators EvaluatorRegistry,
	familyEvaluators map[modelcatalog.AlgorithmFamily]Evaluator,
) *RuntimeResolver {
	return &RuntimeResolver{
		descriptors:         descriptors,
		evaluators:          evaluators,
		descriptorExecutors: descriptorExecutorsFromFamilyEvaluators(familyEvaluators),
	}
}

// ResolveExecution 选择运行时描述符 和 评估器键 用于 一个评估。
func (r *RuntimeResolver) ResolveExecution(a *assessment.Assessment, input *evaluationinput.InputSnapshot) (ResolvedExecution, error) {
	if r == nil || r.evaluators == nil {
		return ResolvedExecution{}, fmt.Errorf("evaluation runtime resolver is not configured")
	}
	executionIdentity := resolveExecutionIdentity(a, input)
	resolved := ResolvedExecution{ExecutionIdentity: executionIdentity}

	snapshot, ok := publishedSnapshotFromInput(input)
	if ok && r.descriptors != nil {
		desc, err := r.descriptors.Resolve(snapshot)
		if err != nil {
			return resolved, err
		}
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
		if _, err := r.resolveDescriptorExecutor(resolved); err != nil {
			return ResolvedExecution{}, err
		}
		return resolved, nil
	}

	if _, err := r.evaluators.Resolve(executionIdentity); err != nil {
		return ResolvedExecution{}, err
	}
	return resolved, nil
}

// Execute 运行评估 using 描述符-主 path 使用 Evaluator键 分发。
func (r *RuntimeResolver) Execute(
	ctx context.Context,
	a *assessment.Assessment,
	input *evaluationinput.InputSnapshot,
) (*assessment.AssessmentOutcome, ResolvedExecution, error) {
	resolved, err := r.ResolveExecution(a, input)
	if err != nil {
		return nil, ResolvedExecution{}, err
	}
	if resolved.UsedDescriptor {
		executor, err := r.resolveDescriptorExecutor(resolved)
		if err != nil {
			return nil, resolved, err
		}
		outcome, err := executor.Execute(ctx, resolved.Descriptor, ExecutionInput{Assessment: a, Input: input})
		return outcome, resolved, err
	}
	evaluator, err := r.resolveEvaluator(resolved.ExecutionIdentity)
	if err != nil {
		return nil, resolved, err
	}
	outcome, err := evaluator.Execute(ctx, ExecutionInput{Assessment: a, Input: input})
	return outcome, resolved, err
}

func (r *RuntimeResolver) resolveDescriptorExecutor(resolved ResolvedExecution) (DescriptorExecutor, error) {
	if r.descriptorExecutors != nil {
		if executor, ok := r.descriptorExecutors[resolved.Descriptor.AlgorithmFamily]; ok {
			return executor, nil
		}
		return nil, fmt.Errorf("unsupported evaluation algorithm family: %s", resolved.Descriptor.AlgorithmFamily)
	}
	evaluator, err := r.resolveEvaluator(resolved.ExecutionIdentity)
	if err != nil {
		return nil, err
	}
	return evaluatorBackedDescriptorExecutor{evaluator: evaluator}, nil
}

func (r *RuntimeResolver) resolveEvaluator(key evaluation.ExecutionIdentity) (Evaluator, error) {
	return r.evaluators.Resolve(key)
}

type evaluatorBackedDescriptorExecutor struct {
	evaluator Evaluator
}

func (e evaluatorBackedDescriptorExecutor) Execute(
	ctx context.Context,
	_ evalpipeline.RuntimeDescriptor,
	input ExecutionInput,
) (*assessment.AssessmentOutcome, error) {
	if e.evaluator == nil {
		return nil, fmt.Errorf("evaluation descriptor executor is not configured")
	}
	return e.evaluator.Execute(ctx, input)
}

func descriptorExecutorsFromFamilyEvaluators(
	familyEvaluators map[modelcatalog.AlgorithmFamily]Evaluator,
) map[modelcatalog.AlgorithmFamily]DescriptorExecutor {
	if familyEvaluators == nil {
		return nil
	}
	out := make(map[modelcatalog.AlgorithmFamily]DescriptorExecutor, len(familyEvaluators))
	for family, evaluator := range familyEvaluators {
		out[family] = evaluatorBackedDescriptorExecutor{evaluator: evaluator}
	}
	return out
}
