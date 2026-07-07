package pipeline

import (
	"context"
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type descriptorPipeline struct {
	registry *RuntimeDescriptorRegistry
}

// NewEvaluationPipeline 创建pipeline 基于 运行时描述符注册表。
func NewEvaluationPipeline(registry *RuntimeDescriptorRegistry) EvaluationPipeline {
	return &descriptorPipeline{registry: registry}
}

func (p *descriptorPipeline) Supports(snapshot modelcatalog.PublishedModelSnapshot) bool {
	if p == nil || p.registry == nil {
		return false
	}
	_, err := p.registry.Resolve(snapshot)
	return err == nil
}

func (p *descriptorPipeline) Execute(ctx context.Context, snapshot modelcatalog.PublishedModelSnapshot) (any, error) {
	if p == nil || p.registry == nil {
		return nil, fmt.Errorf("evaluation pipeline is not configured")
	}
	desc, err := p.registry.Resolve(snapshot)
	if err != nil {
		return nil, err
	}
	if desc.Calculator == nil {
		return nil, fmt.Errorf("calculator is not configured for %s", desc.Key)
	}
	input := CalculationInput{Snapshot: snapshot}
	if desc.InputAssembler != nil {
		assembled, err := desc.InputAssembler.Assemble(snapshot)
		if err != nil {
			return nil, err
		}
		input = assembled
	}
	result, err := desc.Calculator.Calculate(ctx, input)
	if err != nil {
		return nil, err
	}
	if desc.OutcomeAssembler == nil || result == nil {
		return result, nil
	}
	return desc.OutcomeAssembler.Assemble(result)
}
