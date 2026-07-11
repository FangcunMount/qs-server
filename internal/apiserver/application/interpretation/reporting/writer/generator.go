package writer

import (
	"context"
	"fmt"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/projection"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/registry"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

type reportGenerator struct {
	builders   registry.ReportBuilderRegistry
	assemblers projection.EventAssemblerRegistry
}

func NewGenerator(builders registry.ReportBuilderRegistry) (Generator, error) {
	assemblers, err := projection.NewEventAssemblerRegistry(projection.DefaultMechanismEventAssemblers()...)
	if err != nil {
		return nil, err
	}
	return &reportGenerator{builders: builders, assemblers: assemblers}, nil
}

func (g *reportGenerator) Generate(ctx context.Context, outcome evaloutcome.Outcome) (Generation, error) {
	if outcome.Assessment == nil || outcome.Execution == nil {
		return Generation{}, fmt.Errorf("persisted evaluation outcome context is incomplete")
	}
	if !outcome.Assessment.Status().CanGenerateReport() {
		return Generation{}, assessment.NewInvalidStatusError("generate report", outcome.Assessment.Status())
	}
	mechanismKey, ok := registry.MechanismReportBuilderKeyFromOutcome(outcome)
	if !ok {
		return Generation{}, fmt.Errorf("unsupported mechanism report builder key for outcome")
	}
	if g == nil || g.builders == nil {
		return Generation{}, fmt.Errorf("interpretation report builder registry is not configured")
	}
	builder, err := g.builders.ResolveByMechanism(mechanismKey)
	if err != nil {
		return Generation{}, err
	}
	rpt, err := builder.Build(ctx, outcome)
	if err != nil {
		return Generation{}, err
	}
	assembler := g.assemblers.ResolveByMechanism(mechanismKey)
	events := assembler.BuildSuccessEvents(outcome, rpt)
	filtered := events[:0]
	for _, evt := range events {
		if evt == nil || evt.EventType() == assessment.EventTypeInterpretedOutcome {
			continue
		}
		filtered = append(filtered, evt)
	}
	return Generation{Report: rpt, Events: filtered}, nil
}

// ResolveOutcomeKey 解析评估器键 从 结果。
func ResolveOutcomeKey(outcome evaloutcome.Outcome) evaluation.ExecutionIdentity {
	if outcome.Execution != nil && !outcome.Execution.ModelRef.IsEmpty() {
		return outcome.Execution.ModelRef.ExecutionIdentity()
	}
	if outcome.Assessment != nil && outcome.Assessment.EvaluationModelRef() != nil {
		return outcome.Assessment.EvaluationModelRef().ExecutionIdentity()
	}
	if outcome.Input != nil && outcome.Input.Model != nil {
		return outcome.Input.Model.ModelRef().ExecutionIdentity()
	}
	return evaluation.ExecutionIdentity{}
}
