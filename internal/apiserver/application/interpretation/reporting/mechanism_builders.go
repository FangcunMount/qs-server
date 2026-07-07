package reporting

import (
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// MechanismReportBuilderKey 路由报告构建器 按 执行机制, 不 测评编码。
type MechanismReportBuilderKey struct {
	AlgorithmFamily modelcatalog.AlgorithmFamily
	DecisionKind    modelcatalog.DecisionKind
	ReportType      domainReport.ReportType
}

func (k MechanismReportBuilderKey) String() string {
	return k.AlgorithmFamily.String() + "/" + string(k.DecisionKind) + "/" + string(k.ReportType)
}

// MechanismKeyedReportBuilder 暴露机制 路由 元数据 用于 报告构建器。
// MechanismKey 是主 路由 键; 键 保持 用于 旧版 表征。
type MechanismKeyedReportBuilder interface {
	ReportBuilder
	MechanismKey() MechanismReportBuilderKey
}

// MultiMechanismKeyedReportBuilder registers 额外 decision-granularity 机制键。
type MultiMechanismKeyedReportBuilder interface {
	MechanismKeyedReportBuilder
	MechanismKeys() []MechanismReportBuilderKey
}

func (b FactorScoringReportBuilder) MechanismKey() MechanismReportBuilderKey {
	return MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		ReportType:      b.ReportType(),
	}
}

func (b NormProfileReportBuilder) MechanismKey() MechanismReportBuilderKey {
	return MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorNorm,
		DecisionKind:    modelcatalog.DecisionKindNormLookup,
		ReportType:      b.ReportType(),
	}
}

func (b TaskPerformanceReportBuilder) MechanismKey() MechanismReportBuilderKey {
	return MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyTaskPerformance,
		DecisionKind:    modelcatalog.DecisionKindAbilityLevel,
		ReportType:      b.ReportType(),
	}
}

func (FactorScoringScoreProjector) MechanismKey() MechanismReportBuilderKey {
	return MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		ReportType:      domainReport.ReportTypeStandard,
	}
}

func (NormProfileScoreProjector) MechanismKey() MechanismReportBuilderKey {
	return MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorNorm,
		DecisionKind:    modelcatalog.DecisionKindNormLookup,
		ReportType:      domainReport.ReportTypeStandard,
	}
}

func (TaskPerformanceScoreProjector) MechanismKey() MechanismReportBuilderKey {
	return MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyTaskPerformance,
		DecisionKind:    modelcatalog.DecisionKindAbilityLevel,
		ReportType:      domainReport.ReportTypeStandard,
	}
}

// MechanismKeyedScoreProjector 暴露机制 路由 元数据 用于 score 投影器。
type MechanismKeyedScoreProjector interface {
	ScoreProjector
	MechanismKey() MechanismReportBuilderKey
}

// MultiMechanismKeyedScoreProjector registers 额外 decision-granularity 机制键。
type MultiMechanismKeyedScoreProjector interface {
	MechanismKeyedScoreProjector
	MechanismKeys() []MechanismReportBuilderKey
}

// MechanismKeyedEventAssembler 暴露机制 路由 元数据 用于 事件组装器。
type MechanismKeyedEventAssembler interface {
	EventAssembler
	MechanismKey() MechanismReportBuilderKey
}

// MultiMechanismKeyedEventAssembler registers 额外 decision-granularity 机制键。
type MultiMechanismKeyedEventAssembler interface {
	MechanismKeyedEventAssembler
	MechanismKeys() []MechanismReportBuilderKey
}
