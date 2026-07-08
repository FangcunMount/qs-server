package reporting

import (
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// ReportRoutingContext 携带报告路由的执行机制和产品分类上下文。
type ReportRoutingContext struct {
	AlgorithmFamily modelcatalog.AlgorithmFamily
	DecisionKind    modelcatalog.DecisionKind
	ReportType      domainReport.ReportType
	Algorithm       modelcatalog.Algorithm
	ProductChannel  modelcatalog.ProductChannel
}

// ReportRoutingContextFromOutcome 从评估结果推导报告路由上下文。
func ReportRoutingContextFromOutcome(outcome evaloutcome.Outcome) (ReportRoutingContext, bool) {
	ctx := ReportRoutingContext{ReportType: resolveReportType(outcome)}
	if snapshot, ok := evaloutcome.PublishedSnapshotFromInput(outcome.Input); ok {
		ctx.Algorithm = snapshot.Model.Algorithm
		ctx.ProductChannel = snapshot.Model.ProductChannel
		if ctx.ProductChannel == "" {
			ctx.ProductChannel = modelcatalog.DefaultProductChannelFor(snapshot.Model.Kind)
		}
		if routingKey, err := evalpipeline.ExecutionRoutingFromSnapshot(snapshot); err == nil {
			ctx.AlgorithmFamily = routingKey.AlgorithmFamily
			ctx.DecisionKind = routingKey.DecisionKind
		}
	}
	if !outcome.RuntimeDescriptorKey.IsZero() {
		ctx.AlgorithmFamily = outcome.RuntimeDescriptorKey.AlgorithmFamily
		ctx.DecisionKind = outcome.RuntimeDescriptorKey.DecisionKind
		if ctx.DecisionKind == "" {
			ctx.DecisionKind = defaultDecisionKindForFamily(ctx.AlgorithmFamily)
		}
	}
	if ctx.Algorithm == "" || ctx.ProductChannel == "" {
		kind, algorithm := reportRoutingIdentity(outcome)
		if ctx.Algorithm == "" {
			ctx.Algorithm = algorithm
		}
		if ctx.ProductChannel == "" && kind != "" {
			ctx.ProductChannel = modelcatalog.DefaultProductChannelFor(kind)
		}
	}
	if ctx.AlgorithmFamily == "" || ctx.DecisionKind == "" {
		return ReportRoutingContext{}, false
	}
	return ctx, true
}

func (c ReportRoutingContext) MechanismKey() (MechanismReportBuilderKey, bool) {
	if c.AlgorithmFamily == "" || c.DecisionKind == "" {
		return MechanismReportBuilderKey{}, false
	}
	reportType := c.ReportType
	if reportType == "" {
		reportType = domainReport.ReportTypeStandard
	}
	return MechanismReportBuilderKey{
		AlgorithmFamily: c.AlgorithmFamily,
		DecisionKind:    c.DecisionKind,
		ReportType:      reportType,
	}, true
}

func reportRoutingIdentity(outcome evaloutcome.Outcome) (modelcatalog.Kind, modelcatalog.Algorithm) {
	if outcome.Execution != nil && !outcome.Execution.ModelRef.IsEmpty() {
		modelRef := outcome.Execution.ModelRef
		return modelRef.Kind(), modelRef.Algorithm()
	}
	if outcome.Assessment != nil && outcome.Assessment.EvaluationModelRef() != nil {
		modelRef := outcome.Assessment.EvaluationModelRef()
		return modelRef.Kind(), modelRef.Algorithm()
	}
	if outcome.Input != nil && outcome.Input.Model != nil {
		model := outcome.Input.Model
		return modelcatalog.Kind(model.Kind), modelcatalog.Algorithm(model.Algorithm)
	}
	return "", ""
}
