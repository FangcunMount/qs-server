package reporting

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// MechanismReportBuilderKey routes report builders by execution mechanism, not assessment code.
type MechanismReportBuilderKey struct {
	AlgorithmFamily modelcatalog.AlgorithmFamily
	DecisionKind    modelcatalog.DecisionKind
	ReportType      domainReport.ReportType
}

func (k MechanismReportBuilderKey) String() string {
	return k.AlgorithmFamily.String() + "/" + string(k.DecisionKind) + "/" + string(k.ReportType)
}

// MechanismKeyedReportBuilder exposes mechanism routing metadata for a report builder.
// MechanismKey is the primary routing key after Round 5; Key remains for legacy characterization.
type MechanismKeyedReportBuilder interface {
	ReportBuilder
	MechanismKey() MechanismReportBuilderKey
}

// FactorScoringReportBuilder builds factor-scoring reports.
type FactorScoringReportBuilder = ScaleReportBuilder

// NewFactorScoringReportBuilder creates a factor-scoring report builder.
func NewFactorScoringReportBuilder(composer domainReport.ReportBuilder) FactorScoringReportBuilder {
	return NewScaleReportBuilder(composer)
}

func (b FactorScoringReportBuilder) MechanismKey() MechanismReportBuilderKey {
	return MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorScoring,
		DecisionKind:    modelcatalog.DecisionKindScoreRange,
		ReportType:      b.ReportType(),
	}
}

// NormProfileReportBuilder builds norm-profile reports.
type NormProfileReportBuilder = BehavioralRatingReportBuilder

// NewNormProfileReportBuilder creates a norm-profile report builder.
func NewNormProfileReportBuilder(composer domainReport.ReportBuilder) NormProfileReportBuilder {
	return NewBehavioralRatingReportBuilder(composer)
}

func (b NormProfileReportBuilder) MechanismKey() MechanismReportBuilderKey {
	return MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyFactorNorm,
		DecisionKind:    modelcatalog.DecisionKindNormLookup,
		ReportType:      b.ReportType(),
	}
}

// TaskPerformanceReportBuilder builds task-performance reports.
type TaskPerformanceReportBuilder = CognitiveReportBuilder

// NewTaskPerformanceReportBuilder creates a task-performance report builder.
func NewTaskPerformanceReportBuilder(composer domainReport.ReportBuilder) TaskPerformanceReportBuilder {
	return NewCognitiveReportBuilder(composer)
}

func (b TaskPerformanceReportBuilder) MechanismKey() MechanismReportBuilderKey {
	return MechanismReportBuilderKey{
		AlgorithmFamily: modelcatalog.AlgorithmFamilyTaskPerformance,
		DecisionKind:    modelcatalog.DecisionKindAbilityLevel,
		ReportType:      b.ReportType(),
	}
}

// FactorScoringScoreProjector projects factor-scoring scores after interpretation.
type FactorScoringScoreProjector = ScaleScoreProjector

// NewFactorScoringScoreProjector creates a factor-scoring score projector.
func NewFactorScoringScoreProjector(scoreRepo assessment.ScoreRepository) FactorScoringScoreProjector {
	return NewScaleScoreProjector(scoreRepo)
}

// NormProfileScoreProjector projects norm-profile scores after interpretation.
type NormProfileScoreProjector = BehavioralRatingScoreProjector

// NewNormProfileScoreProjector creates a norm-profile score projector.
func NewNormProfileScoreProjector(scoreRepo assessment.ScoreRepository) NormProfileScoreProjector {
	return NewBehavioralRatingScoreProjector(scoreRepo)
}

// TaskPerformanceScoreProjector projects task-performance scores after interpretation.
type TaskPerformanceScoreProjector = CognitiveScoreProjector

// NewTaskPerformanceScoreProjector creates a task-performance score projector.
func NewTaskPerformanceScoreProjector(scoreRepo assessment.ScoreRepository) TaskPerformanceScoreProjector {
	return NewCognitiveScoreProjector(scoreRepo)
}
