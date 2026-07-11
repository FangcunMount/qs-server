package typology

import (
	"context"
	"fmt"

	interpretationreporting "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reporting"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	interpinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	reporttypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/typology/patterns"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

type ReportBuilder struct {
	registry ReportAdapterRegistry
	key      evaluation.ExecutionIdentity
}

var (
	_ interpretationreporting.ReportBuilder                    = ReportBuilder{}
	_ interpretationreporting.MechanismKeyedReportBuilder      = ReportBuilder{}
	_ interpretationreporting.MultiMechanismKeyedReportBuilder = ReportBuilder{}
)

func NewReportBuilder(algorithm modelcatalog.Algorithm) (ReportBuilder, error) {
	return ReportBuilder{
		registry: DefaultReportAdapterRegistry(),
		key:      evaluation.PersonalityTypologyIdentity(algorithm),
	}, nil
}

func NewConfiguredReportBuilderWithRegistry(registry ReportAdapterRegistry) (ReportBuilder, error) {
	if registry.Len() == 0 {
		return ReportBuilder{}, fmt.Errorf("typology report adapter registry is required")
	}
	return ReportBuilder{
		registry: registry,
		key:      evaluation.ExecutionIdentityPersonalityTypology,
	}, nil
}

func NewConfiguredReportBuilder() (ReportBuilder, error) {
	return NewConfiguredReportBuilderWithRegistry(DefaultReportAdapterRegistry())
}

func NewReportBuilderWithRegistry(registry ReportAdapterRegistry, algorithm modelcatalog.Algorithm) (ReportBuilder, error) {
	if registry.Len() == 0 {
		return ReportBuilder{}, fmt.Errorf("typology report adapter registry is required")
	}
	return ReportBuilder{
		registry: registry,
		key:      evaluation.PersonalityTypologyIdentity(algorithm),
	}, nil
}

func NewLegacyTypologyAliasReportBuilder(configured ReportBuilder, algorithm modelcatalog.Algorithm) (ReportBuilder, error) {
	if configured.registry.Len() == 0 {
		return ReportBuilder{}, fmt.Errorf("configured typology report builder is required")
	}
	return ReportBuilder{
		registry: configured.registry,
		key:      evaluation.PersonalityTypologyIdentity(algorithm),
	}, nil
}

func (b ReportBuilder) ExecutionIdentity() evaluation.ExecutionIdentity {
	return b.key
}

func (b ReportBuilder) Key() evaluation.ExecutionIdentity {
	return b.ExecutionIdentity()
}

func (ReportBuilder) ReportType() domainReport.ReportType {
	return domainReport.ReportTypeStandard
}

func (ReportBuilder) MechanismKey() interpretationreporting.MechanismReportBuilderKey {
	return typologyMechanismKeys()[0]
}

func (ReportBuilder) MechanismKeys() []interpretationreporting.MechanismReportBuilderKey {
	return typologyMechanismKeys()
}

func typologyMechanismKeys() []interpretationreporting.MechanismReportBuilderKey {
	reportType := domainReport.ReportTypeStandard
	return []interpretationreporting.MechanismReportBuilderKey{
		{
			AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification,
			DecisionKind:    modelcatalog.DecisionKindPoleComposition,
			ReportType:      reportType,
		},
		{
			AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification,
			DecisionKind:    modelcatalog.DecisionKindTraitProfile,
			ReportType:      reportType,
		},
		{
			AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorClassification,
			DecisionKind:    modelcatalog.DecisionKindNearestPattern,
			ReportType:      reportType,
		},
	}
}

func (b ReportBuilder) Build(_ context.Context, input interpinput.InterpretationInput) (*report.Draft, error) {
	if b.registry.Len() == 0 {
		return nil, fmt.Errorf("personality typology report builder is not configured")
	}
	modelCode := input.Model.Code
	if input.PersonalityType != nil {
		adapter := personalityAdapter(input)
		rpt, err := reporttypology.BuildPersonalityTypeReport(reporttypology.PersonalityTypeReportInput{
			AssessmentID: report.ID(input.Association.AssessmentID), ModelCode: modelCode,
			TotalScore: primaryValue(input), RiskLevel: riskLevel(input), Detail: input.PersonalityType.Detail,
		}, reporttypology.PersonalityTypeTemplateForSpec(reporttypology.ReportSpec{AdapterKey: adapter, TemplateID: input.Report.TemplateID}))
		if err != nil {
			return nil, err
		}
		return interpretationreporting.DraftFromLegacyReport(input, rpt), nil
	}
	if input.TraitProfile != nil {
		adapter := traitProfileAdapter(input)
		rpt, err := reporttypology.BuildTraitProfileReport(reporttypology.TraitProfileReportInput{
			AssessmentID: report.ID(input.Association.AssessmentID), ModelCode: modelCode,
			TotalScore: primaryValue(input), RiskLevel: riskLevel(input), Detail: input.TraitProfile.Detail,
		}, reporttypology.TraitProfileTemplateForSpec(reporttypology.ReportSpec{AdapterKey: adapter, TemplateID: input.Report.TemplateID}))
		if err != nil {
			return nil, err
		}
		return interpretationreporting.DraftFromLegacyReport(input, rpt), nil
	}
	return nil, fmt.Errorf("typology interpretation facts are required")
}

func personalityAdapter(input interpinput.InterpretationInput) reporttypology.ReportAdapterKey {
	if input.Report.AdapterKey != "" {
		return reporttypology.ReportAdapterKey(input.Report.AdapterKey)
	}
	switch input.Model.Algorithm {
	case "mbti":
		return reporttypology.ReportAdapterMBTI
	case "sbti":
		return reporttypology.ReportAdapterSBTI
	default:
		return reporttypology.ReportAdapterPersonalityType
	}
}

func traitProfileAdapter(input interpinput.InterpretationInput) reporttypology.ReportAdapterKey {
	if input.Report.AdapterKey != "" {
		return reporttypology.ReportAdapterKey(input.Report.AdapterKey)
	}
	if input.Model.Algorithm == "bigfive" {
		return reporttypology.ReportAdapterBigFive
	}
	return reporttypology.ReportAdapterTraitProfile
}

func primaryValue(input interpinput.InterpretationInput) float64 {
	if input.Result.Primary == nil {
		return 0
	}
	return input.Result.Primary.Value
}

func riskLevel(input interpinput.InterpretationInput) report.RiskLevel {
	if input.Result.Level == nil || !domainReport.IsRiskLevelCode(input.Result.Level.Code) {
		return report.RiskLevelNone
	}
	return report.RiskLevel(input.Result.Level.Code)
}
