package writer

import (
	"context"
	"fmt"

	interpretationinput "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/projection"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/registry"
	assessment "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationassessment"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationcompat"
	evaluation "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationruntime"
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
	input, err := interpretationinput.FromLegacyOutcome(outcome)
	if err != nil {
		return Generation{}, err
	}
	mechanismKey, ok := registry.MechanismReportBuilderKeyFromInput(input)
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
	draft, err := builder.Build(ctx, input)
	if err != nil {
		return Generation{}, err
	}
	rpt := legacyReportFromDraft(input, draft)
	assembler := g.assemblers.ResolveByMechanism(mechanismKey)
	events := assembler.BuildSuccessEvents(outcome, rpt)
	return Generation{Report: rpt, Events: events}, nil
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
