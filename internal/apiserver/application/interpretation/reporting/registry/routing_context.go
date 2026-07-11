package registry

import (
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
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
