package reporting

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/registry"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	evaluation "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationruntime"
)

const (
	reportSectionSuggestions = "suggestions"
	reportSectionModelExtra  = "model_extra"
)

// ExpandAudienceProfileBuilders registers audience/profile keyed variants for each mechanism builder.
func ExpandAudienceProfileBuilders(builders ...registry.ReportBuilder) []registry.ReportBuilder {
	out := make([]registry.ReportBuilder, 0, len(builders)*6)
	out = append(out, builders...)
	for _, builder := range builders {
		for _, baseKey := range mechanismBaseKeys(builder) {
			out = append(out, audienceProfileVariants(builder, baseKey)...)
		}
	}
	return out
}

func mechanismBaseKeys(builder registry.ReportBuilder) []registry.MechanismReportBuilderKey {
	if multi, ok := builder.(registry.MultiMechanismKeyedReportBuilder); ok {
		return multi.MechanismKeys()
	}
	if keyed, ok := builder.(registry.MechanismKeyedReportBuilder); ok {
		return []registry.MechanismReportBuilderKey{keyed.MechanismKey()}
	}
	return nil
}

func audienceProfileVariants(
	delegate registry.ReportBuilder,
	base registry.MechanismReportBuilderKey,
) []registry.ReportBuilder {
	profile := policy.ReportProfileForDecisionKind(base.DecisionKind)
	variants := []registry.ReportBuilder{
		newKeyedReportBuilder(delegate, withAudience(base, policy.AudienceParticipant)),
		newVisibilityPolicyReportBuilder(delegate, withAudience(base, policy.AudienceClinician), clinicianVisibilityPolicy()),
		newKeyedReportBuilder(delegate, withAudience(base, policy.AudienceAdmin)),
	}
	if profile != policy.ReportProfileDefault {
		variants = append(variants,
			newKeyedReportBuilder(delegate, withReportProfile(base, profile)),
			newVisibilityPolicyReportBuilder(delegate, withAudienceAndProfile(base, policy.AudienceClinician, profile), clinicianVisibilityPolicy()),
		)
	}
	return variants
}

func withAudience(base registry.MechanismReportBuilderKey, audience policy.Audience) registry.MechanismReportBuilderKey {
	base.Audience = audience
	base.ReportProfile = ""
	return base
}

func withReportProfile(base registry.MechanismReportBuilderKey, profile policy.ReportProfile) registry.MechanismReportBuilderKey {
	base.Audience = ""
	base.ReportProfile = profile
	return base
}

func withAudienceAndProfile(
	base registry.MechanismReportBuilderKey,
	audience policy.Audience,
	profile policy.ReportProfile,
) registry.MechanismReportBuilderKey {
	base.Audience = audience
	base.ReportProfile = profile
	return base
}

func clinicianVisibilityPolicy() policy.VisibilityPolicy {
	return policy.VisibilityPolicy{
		Audience: policy.AudienceClinician,
		Hidden:   []string{reportSectionModelExtra},
	}
}

type keyedReportBuilder struct {
	delegate registry.ReportBuilder
	key      registry.MechanismReportBuilderKey
}

func newKeyedReportBuilder(delegate registry.ReportBuilder, key registry.MechanismReportBuilderKey) keyedReportBuilder {
	return keyedReportBuilder{delegate: delegate, key: key}
}

func (b keyedReportBuilder) ExecutionIdentity() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentity{}
}
func (b keyedReportBuilder) Key() evaluation.ExecutionIdentity   { return evaluation.ExecutionIdentity{} }
func (b keyedReportBuilder) ReportType() domainReport.ReportType { return b.delegate.ReportType() }
func (b keyedReportBuilder) TemplateVersion() policy.TemplateVersion {
	return b.delegate.TemplateVersion()
}
func (b keyedReportBuilder) BuilderIdentity() string      { return b.delegate.BuilderIdentity() }
func (b keyedReportBuilder) ContentSchemaVersion() string { return b.delegate.ContentSchemaVersion() }
func (b keyedReportBuilder) MechanismKey() registry.MechanismReportBuilderKey {
	return b.key
}
func (b keyedReportBuilder) Build(ctx context.Context, input interpinput.InterpretationInput) (*report.Draft, error) {
	return b.delegate.Build(ctx, input)
}

type visibilityPolicyReportBuilder struct {
	keyedReportBuilder
	visibility policy.VisibilityPolicy
}

func newVisibilityPolicyReportBuilder(
	delegate registry.ReportBuilder,
	key registry.MechanismReportBuilderKey,
	visibility policy.VisibilityPolicy,
) visibilityPolicyReportBuilder {
	return visibilityPolicyReportBuilder{
		keyedReportBuilder: newKeyedReportBuilder(delegate, key),
		visibility:         visibility,
	}
}

func (b visibilityPolicyReportBuilder) Build(ctx context.Context, input interpinput.InterpretationInput) (*report.Draft, error) {
	draft, err := b.delegate.Build(ctx, input)
	if err != nil {
		return nil, err
	}
	return filterDraftByVisibility(draft, b.visibility), nil
}

func filterDraftByVisibility(draft *report.Draft, visibility policy.VisibilityPolicy) *report.Draft {
	if draft == nil {
		return nil
	}
	content := draft.Content()
	if !visibility.IsVisible(reportSectionSuggestions) {
		content.Suggestions = nil
	}
	if !visibility.IsVisible(reportSectionModelExtra) {
		content.ModelExtra = nil
	}
	return report.NewDraft(content)
}
