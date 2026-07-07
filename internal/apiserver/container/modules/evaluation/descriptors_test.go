package evaluation_test

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	evalmodule "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/evaluation"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func TestAssertExecutionPathParityRejectsMismatchedEvaluatorPath(t *testing.T) {
	descs := []evaldomain.ModelDescriptor{
		{Kind: evaldomain.ModelKindScale},
	}
	evaluators := []execute.Evaluator{
		parityStubEvaluator{path: modelcatalog.ExecutionPathTypologyDescriptor},
	}
	builders := []interpretationreporting.ReportBuilder{
		parityStubReportBuilder{path: modelcatalog.ExecutionPathScaleDescriptor},
	}
	providers := []evaluationinputInfra.ModelInputProvider{
		parityStubInputProvider{path: modelcatalog.ExecutionPathScaleDescriptor},
	}

	err := evalmodule.AssertExecutionPathParity(descs, evaluators, builders, providers)
	if err == nil {
		t.Fatal("expected parity error for mismatched evaluator execution path")
	}
}

func TestAssertExecutionPathParityRejectsCountMismatch(t *testing.T) {
	descs := []evaldomain.ModelDescriptor{
		{Kind: evaldomain.ModelKindScale},
	}
	err := evalmodule.AssertExecutionPathParity(descs, nil, nil, nil)
	if err == nil {
		t.Fatal("expected parity error for descriptor count mismatch")
	}
}

type parityStubEvaluator struct {
	path modelcatalog.ExecutionPath
}

func (s parityStubEvaluator) ExecutionIdentity() evaldomain.ExecutionIdentity {
	return evaldomain.ExecutionIdentityScaleDefault
}

func (s parityStubEvaluator) ExecutionPath() modelcatalog.ExecutionPath { return s.path }

func (parityStubEvaluator) Execute(context.Context, execute.ExecutionInput) (*assessment.AssessmentOutcome, error) {
	return nil, nil
}

func (s parityStubEvaluator) Key() evaldomain.ExecutionIdentity {
	return s.ExecutionIdentity()
}

type parityStubReportBuilder struct {
	path modelcatalog.ExecutionPath
}

func (s parityStubReportBuilder) ExecutionIdentity() evaldomain.ExecutionIdentity {
	return evaldomain.ExecutionIdentityScaleDefault
}

func (s parityStubReportBuilder) ReportType() domainreport.ReportType {
	return domainreport.ReportTypeStandard
}

func (s parityStubReportBuilder) Key() evaldomain.ExecutionIdentity {
	return s.ExecutionIdentity()
}

func (parityStubReportBuilder) Build(context.Context, evaloutcome.Outcome) (*domainreport.InterpretReport, error) {
	return nil, nil
}

func (s parityStubReportBuilder) MechanismKey() interpretationreporting.MechanismReportBuilderKey {
	return interpretationreporting.MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		ReportType:      domainreport.ReportTypeStandard,
	}
}

type parityStubInputProvider struct {
	path modelcatalog.ExecutionPath
}

func (s parityStubInputProvider) ExecutionIdentity() evaldomain.ExecutionIdentity {
	return evaldomain.ExecutionIdentityScaleDefault
}

func (s parityStubInputProvider) EvaluatorKey() evaldomain.ExecutionIdentity {
	return s.ExecutionIdentity()
}

func (s parityStubInputProvider) ExecutionPath() modelcatalog.ExecutionPath { return s.path }

func (parityStubInputProvider) ResolveInput(context.Context, evaluationinput.InputRef) (*evaluationinput.InputSnapshot, error) {
	return nil, nil
}
