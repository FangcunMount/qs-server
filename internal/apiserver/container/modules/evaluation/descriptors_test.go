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
	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func TestAssertRegistryKeyParityRejectsMismatchedEvaluatorKey(t *testing.T) {
	descs := []evaldomain.ModelDescriptor{
		{Key: evaldomain.EvaluatorKeyScaleDefault, Kind: evaldomain.ModelKindScale},
	}
	evaluators := []execute.Evaluator{
		parityStubEvaluator{key: evaldomain.EvaluatorKeyPersonalityTypology},
	}
	builders := []interpretationreporting.ReportBuilder{
		parityStubReportBuilder{key: evaldomain.EvaluatorKeyScaleDefault},
	}
	providers := []evaluationinputInfra.ModelInputProvider{
		parityStubInputProvider{key: evaldomain.EvaluatorKeyScaleDefault},
	}

	err := evalmodule.AssertRegistryKeyParity(descs, evaluators, builders, providers)
	if err == nil {
		t.Fatal("expected parity error for mismatched evaluator key")
	}
}

func TestAssertRegistryKeyParityRejectsCountMismatch(t *testing.T) {
	descs := []evaldomain.ModelDescriptor{
		{Key: evaldomain.EvaluatorKeyScaleDefault, Kind: evaldomain.ModelKindScale},
	}
	err := evalmodule.AssertRegistryKeyParity(descs, nil, nil, nil)
	if err == nil {
		t.Fatal("expected parity error for descriptor count mismatch")
	}
}

type parityStubEvaluator struct {
	key evaldomain.EvaluatorKey
}

func (s parityStubEvaluator) Key() evaldomain.EvaluatorKey { return s.key }

func (parityStubEvaluator) Execute(context.Context, execute.ExecutionInput) (*assessment.AssessmentOutcome, error) {
	return nil, nil
}

type parityStubReportBuilder struct {
	key evaldomain.EvaluatorKey
}

func (s parityStubReportBuilder) Key() evaldomain.EvaluatorKey { return s.key }

func (parityStubReportBuilder) ReportType() domainreport.ReportType {
	return domainreport.ReportTypeStandard
}

func (parityStubReportBuilder) Build(context.Context, evaloutcome.Outcome) (*domainreport.InterpretReport, error) {
	return nil, nil
}

type parityStubInputProvider struct {
	key evaldomain.EvaluatorKey
}

func (s parityStubInputProvider) EvaluatorKey() evaldomain.EvaluatorKey { return s.key }

func (parityStubInputProvider) ResolveInput(context.Context, evaluationinput.InputRef) (*evaluationinput.InputSnapshot, error) {
	return nil, nil
}
