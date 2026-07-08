package reporting_test

import (
	"context"
	"testing"

	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type stubMechanismBuilder struct {
	reporting.FactorScoringReportBuilder
	key reporting.MechanismReportBuilderKey
}

func (b stubMechanismBuilder) MechanismKey() reporting.MechanismReportBuilderKey {
	return b.key
}

type namedMechanismBuilder struct {
	key reporting.MechanismReportBuilderKey
}

func (b namedMechanismBuilder) ExecutionIdentity() evaluation.ExecutionIdentity {
	return evaluation.ExecutionIdentity{}
}

func (b namedMechanismBuilder) Key() evaluation.ExecutionIdentity {
	return b.ExecutionIdentity()
}

func (b namedMechanismBuilder) ReportType() domainReport.ReportType {
	if b.key.ReportType == "" {
		return domainReport.ReportTypeStandard
	}
	return b.key.ReportType
}

func (b namedMechanismBuilder) MechanismKey() reporting.MechanismReportBuilderKey {
	return b.key
}

func (b namedMechanismBuilder) Build(context.Context, evaloutcome.Outcome) (*domainReport.InterpretReport, error) {
	return nil, nil
}

func TestResolveByMechanismFallsBackFromAlgorithmToFamily(t *testing.T) {
	familyKey := reporting.MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		ReportType:      domainReport.ReportTypeStandard,
	}
	registry, err := reporting.NewReportBuilderRegistry(
		stubMechanismBuilder{key: familyKey},
	)
	if err != nil {
		t.Fatalf("NewReportBuilderRegistry: %v", err)
	}
	specific := reporting.MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		ReportType:      domainReport.ReportTypeStandard,
		Algorithm:       modelcatalog.AlgorithmScaleDefault,
		ProductChannel:  modelcatalog.ProductChannelMedicalScale,
	}
	builder, err := registry.ResolveByMechanism(specific)
	if err != nil {
		t.Fatalf("ResolveByMechanism: %v", err)
	}
	if builder == nil {
		t.Fatal("builder is nil")
	}
}

func TestResolveByMechanismPrefersSpecificBuildersBeforeBroadFallback(t *testing.T) {
	broadKey := reporting.MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		ReportType:      domainReport.ReportTypeStandard,
	}
	algorithmKey := reporting.MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		ReportType:      domainReport.ReportTypeStandard,
		Algorithm:       modelcatalog.AlgorithmScaleDefault,
	}
	channelKey := reporting.MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		ReportType:      domainReport.ReportTypeStandard,
		ProductChannel:  modelcatalog.ProductChannelMedicalScale,
	}
	broadBuilder := namedMechanismBuilder{key: broadKey}
	algorithmBuilder := namedMechanismBuilder{key: algorithmKey}
	channelBuilder := namedMechanismBuilder{key: channelKey}
	registry, err := reporting.NewReportBuilderRegistry(
		broadBuilder,
		algorithmBuilder,
		channelBuilder,
	)
	if err != nil {
		t.Fatalf("NewReportBuilderRegistry: %v", err)
	}

	fullKey := reporting.MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		ReportType:      domainReport.ReportTypeStandard,
		Algorithm:       modelcatalog.AlgorithmScaleDefault,
		ProductChannel:  modelcatalog.ProductChannelMedicalScale,
	}
	builder, err := registry.ResolveByMechanism(fullKey)
	if err != nil {
		t.Fatalf("ResolveByMechanism(full): %v", err)
	}
	keyed, ok := builder.(reporting.MechanismKeyedReportBuilder)
	if !ok {
		t.Fatal("builder does not implement MechanismKeyedReportBuilder")
	}
	if keyed.MechanismKey() != algorithmKey {
		t.Fatalf("full key builder = %#v, want algorithm-specific %#v", keyed.MechanismKey(), algorithmKey)
	}

	channelOnlyKey := reporting.MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		ReportType:      domainReport.ReportTypeStandard,
		Algorithm:       modelcatalog.Algorithm("unknown"),
		ProductChannel:  modelcatalog.ProductChannelMedicalScale,
	}
	builder, err = registry.ResolveByMechanism(channelOnlyKey)
	if err != nil {
		t.Fatalf("ResolveByMechanism(channel): %v", err)
	}
	keyed, ok = builder.(reporting.MechanismKeyedReportBuilder)
	if !ok {
		t.Fatal("builder does not implement MechanismKeyedReportBuilder")
	}
	if keyed.MechanismKey() != channelKey {
		t.Fatalf("channel key builder = %#v, want product-channel-specific %#v", keyed.MechanismKey(), channelKey)
	}

	unknownKey := reporting.MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		ReportType:      domainReport.ReportTypeStandard,
		Algorithm:       modelcatalog.Algorithm("unknown"),
		ProductChannel:  modelcatalog.ProductChannel("unknown"),
	}
	builder, err = registry.ResolveByMechanism(unknownKey)
	if err != nil {
		t.Fatalf("ResolveByMechanism(broad): %v", err)
	}
	keyed, ok = builder.(reporting.MechanismKeyedReportBuilder)
	if !ok {
		t.Fatal("builder does not implement MechanismKeyedReportBuilder")
	}
	if keyed.MechanismKey() != broadKey {
		t.Fatalf("fallback builder = %#v, want broad %#v", keyed.MechanismKey(), broadKey)
	}
}

func TestResolveByMechanismFallsBackToBroadWhenAudienceAndProfileSpecified(t *testing.T) {
	broadKey := reporting.MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		ReportType:      domainReport.ReportTypeStandard,
	}
	registry, err := reporting.NewReportBuilderRegistry(namedMechanismBuilder{key: broadKey})
	if err != nil {
		t.Fatalf("NewReportBuilderRegistry: %v", err)
	}

	specific := reporting.MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		ReportType:      domainReport.ReportTypeStandard,
		Algorithm:       modelcatalog.AlgorithmScaleDefault,
		ProductChannel:  modelcatalog.ProductChannelMedicalScale,
		Audience:        policy.AudienceParticipant,
		ReportProfile:   policy.ReportProfile("trait_profile"),
	}
	builder, err := registry.ResolveByMechanism(specific)
	if err != nil {
		t.Fatalf("ResolveByMechanism: %v", err)
	}
	keyed, ok := builder.(reporting.MechanismKeyedReportBuilder)
	if !ok {
		t.Fatal("builder does not implement MechanismKeyedReportBuilder")
	}
	if keyed.MechanismKey() != broadKey {
		t.Fatalf("builder = %#v, want broad %#v", keyed.MechanismKey(), broadKey)
	}
}

func TestResolveByMechanismPrefersAudienceAndProfileBuildersBeforeBroadFallback(t *testing.T) {
	broadKey := reporting.MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		ReportType:      domainReport.ReportTypeStandard,
	}
	audienceKey := reporting.MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		ReportType:      domainReport.ReportTypeStandard,
		Audience:        policy.AudienceClinician,
	}
	profileKey := reporting.MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		ReportType:      domainReport.ReportTypeStandard,
		ReportProfile:   policy.ReportProfile("personality_type"),
	}
	registry, err := reporting.NewReportBuilderRegistry(
		namedMechanismBuilder{key: broadKey},
		namedMechanismBuilder{key: audienceKey},
		namedMechanismBuilder{key: profileKey},
	)
	if err != nil {
		t.Fatalf("NewReportBuilderRegistry: %v", err)
	}

	audienceBuilder, err := registry.ResolveByMechanism(reporting.MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		ReportType:      domainReport.ReportTypeStandard,
		Audience:        policy.AudienceClinician,
		ReportProfile:   policy.ReportProfile("unknown"),
	})
	if err != nil {
		t.Fatalf("ResolveByMechanism(audience): %v", err)
	}
	keyed, ok := audienceBuilder.(reporting.MechanismKeyedReportBuilder)
	if !ok {
		t.Fatal("builder does not implement MechanismKeyedReportBuilder")
	}
	if keyed.MechanismKey() != audienceKey {
		t.Fatalf("audience builder = %#v, want %#v", keyed.MechanismKey(), audienceKey)
	}

	profileBuilder, err := registry.ResolveByMechanism(reporting.MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		ReportType:      domainReport.ReportTypeStandard,
		Audience:        policy.Audience("unknown"),
		ReportProfile:   policy.ReportProfile("personality_type"),
	})
	if err != nil {
		t.Fatalf("ResolveByMechanism(profile): %v", err)
	}
	keyed, ok = profileBuilder.(reporting.MechanismKeyedReportBuilder)
	if !ok {
		t.Fatal("builder does not implement MechanismKeyedReportBuilder")
	}
	if keyed.MechanismKey() != profileKey {
		t.Fatalf("profile builder = %#v, want %#v", keyed.MechanismKey(), profileKey)
	}
}
