package reporting_test

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting/registry"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type stubBroadBuilder struct{}

func (stubBroadBuilder) ReportType() domainreport.ReportType {
	return domainreport.ReportTypeStandard
}
func (stubBroadBuilder) TemplateVersion() policy.TemplateVersion { return policy.TemplateVersionV1 }
func (stubBroadBuilder) BuilderIdentity() string                 { return "audience-test" }
func (stubBroadBuilder) ContentSchemaVersion() string            { return "report-content/v1" }
func (stubBroadBuilder) MechanismKey() registry.MechanismReportBuilderKey {
	return registry.MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		ReportType:      domainreport.ReportTypeStandard,
	}
}
func (stubBroadBuilder) Build(context.Context, interpinput.InterpretationInput) (*report.Draft, error) {
	return report.NewDraft(report.Content{Model: report.ModelIdentity{Title: "scale", Code: "phq9"}, PrimaryScore: report.NewRawTotalScore(10, nil), Level: domainreport.LevelFromRisk(domainreport.RiskLevelNone), Conclusion: "ok", ModelExtra: &domainreport.ModelExtra{}}), nil
}

func TestExpandAudienceProfileBuildersRegistersAudienceAndProfileKeys(t *testing.T) {
	expanded := reporting.ExpandAudienceProfileBuilders(stubBroadBuilder{})
	reg, err := registry.NewReportBuilderRegistry(expanded...)
	if err != nil {
		t.Fatalf("NewReportBuilderRegistry: %v", err)
	}

	broad := registry.MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		ReportType:      domainreport.ReportTypeStandard,
	}
	if _, err := reg.ResolveByMechanism(broad); err != nil {
		t.Fatalf("broad resolve: %v", err)
	}

	clinician := broad
	clinician.Audience = policy.AudienceClinician
	builder, err := reg.ResolveByMechanism(clinician)
	if err != nil {
		t.Fatalf("clinician resolve: %v", err)
	}
	draft, err := builder.Build(context.Background(), interpinput.InterpretationInput{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if draft.Content().ModelExtra != nil {
		t.Fatal("clinician builder should hide model_extra")
	}

	profile := broad
	profile.ReportProfile = policy.ReportProfileScale
	if _, err := reg.ResolveByMechanism(profile); err != nil {
		t.Fatalf("profile resolve: %v", err)
	}
}

type typologyStub struct {
	stubBroadBuilder
}

func (typologyStub) MechanismKeys() []registry.MechanismReportBuilderKey {
	return []registry.MechanismReportBuilderKey{
		{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification, DecisionKind: modelcatalog.DecisionKindPoleComposition, ReportType: domainreport.ReportTypeStandard},
		{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification, DecisionKind: modelcatalog.DecisionKindTraitProfile, ReportType: domainreport.ReportTypeStandard},
		{AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification, DecisionKind: modelcatalog.DecisionKindNearestPattern, ReportType: domainreport.ReportTypeStandard},
	}
}

func TestExpandAudienceProfileBuildersTypologyExpandsAllDecisionKeys(t *testing.T) {
	expanded := reporting.ExpandAudienceProfileBuilders(typologyStub{})
	reg, err := registry.NewReportBuilderRegistry(expanded...)
	if err != nil {
		t.Fatalf("NewReportBuilderRegistry: %v", err)
	}
	stub := typologyStub{}
	for _, key := range stub.MechanismKeys() {
		key.Audience = policy.AudienceAdmin
		if _, err := reg.ResolveByMechanism(key); err != nil {
			t.Fatalf("admin resolve for %s: %v", key.DecisionKind, err)
		}
	}
}
