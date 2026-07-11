package registry

import (
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationcompat"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationroute"
)

// ReportRoutingContext 携带报告路由的执行机制和产品分类上下文。
type ReportRoutingContext struct {
	AlgorithmFamily modelcatalog.AlgorithmFamily
	DecisionKind    modelcatalog.DecisionKind
	ReportType      domainReport.ReportType
	TemplateVersion policy.TemplateVersion
	Algorithm       modelcatalog.Algorithm
	ProductChannel  modelcatalog.ProductChannel
	Audience        policy.Audience
	ReportProfile   policy.ReportProfile
}

// ReportRoutingContextFromInput derives routing solely from the frozen
// Interpretation input. Unlike the legacy Outcome variant it never inspects
// Assessment compatibility data.
func ReportRoutingContextFromInput(input interpinput.InterpretationInput) (ReportRoutingContext, bool) {
	ctx := ReportRoutingContext{
		AlgorithmFamily: input.Runtime.AlgorithmFamily,
		DecisionKind:    input.Runtime.DecisionKind,
		ReportType:      input.Report.ReportType,
		TemplateVersion: input.Report.TemplateVersion,
		Algorithm:       input.Report.Algorithm,
		ProductChannel:  input.Report.ProductChannel,
		Audience:        input.Report.Audience,
		ReportProfile:   input.Report.ReportProfile,
	}
	if ctx.ReportType == "" {
		ctx.ReportType = domainReport.ReportTypeStandard
	}
	if ctx.TemplateVersion == "" {
		ctx.TemplateVersion = policy.TemplateVersionV1
	}
	if ctx.DecisionKind == "" {
		ctx.DecisionKind = defaultDecisionKindForFamily(ctx.AlgorithmFamily)
	}
	if ctx.ReportProfile == "" {
		ctx.ReportProfile = policy.ReportProfileForDecisionKind(ctx.DecisionKind)
	}
	if ctx.AlgorithmFamily == "" || ctx.DecisionKind == "" {
		return ReportRoutingContext{}, false
	}
	return ctx, true
}

// ReportRoutingContextFromOutcome 从评估结果推导报告路由上下文。
func ReportRoutingContextFromOutcome(outcome evaloutcome.Outcome) (ReportRoutingContext, bool) {
	ctx := ReportRoutingContext{ReportType: resolveReportType(outcome)}
	if route, ok := evaloutcome.ModelRouteFromInput(outcome.Input); ok {
		ctx.Algorithm = route.Algorithm
		if outcome.Input != nil && outcome.Input.Model != nil {
			ctx.ProductChannel = modelcatalog.ProductChannel(outcome.Input.Model.ProductChannel)
		}
		if ctx.ProductChannel == "" {
			ctx.ProductChannel = modelcatalog.DefaultProductChannelFor(route.Kind)
		}
		if routingKey, err := evalpipeline.ExecutionRoutingFromRoute(route); err == nil {
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
	if profile := policy.ReportProfileForDecisionKind(ctx.DecisionKind); profile != policy.ReportProfileDefault {
		ctx.ReportProfile = profile
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
		TemplateVersion: c.TemplateVersion,
		Algorithm:       c.Algorithm,
		ProductChannel:  c.ProductChannel,
		Audience:        c.Audience,
		ReportProfile:   c.ReportProfile,
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
