package calculationadapter

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
)

// CalculationKindFromAssessment maps assessment dimension kinds to calculation kinds.
func CalculationKindFromAssessment(kind assessment.DimensionKind) calculation.DimensionKind {
	switch kind {
	case assessment.DimensionKindFactor:
		return calculation.DimensionKindFactor
	case assessment.DimensionKindPole:
		return calculation.DimensionKindPole
	case assessment.DimensionKindTrait:
		return calculation.DimensionKindTrait
	case assessment.DimensionKindIndex:
		return calculation.DimensionKindIndex
	case assessment.DimensionKindAbility:
		return calculation.DimensionKindAbility
	default:
		return calculation.DimensionKind(kind)
	}
}

// AssessmentKindFromCalculation maps calculation dimension kinds to assessment kinds.
func AssessmentKindFromCalculation(kind calculation.DimensionKind) assessment.DimensionKind {
	switch kind {
	case calculation.DimensionKindFactor:
		return assessment.DimensionKindFactor
	case calculation.DimensionKindPole:
		return assessment.DimensionKindPole
	case calculation.DimensionKindTrait:
		return assessment.DimensionKindTrait
	case calculation.DimensionKindIndex:
		return assessment.DimensionKindIndex
	case calculation.DimensionKindAbility:
		return assessment.DimensionKindAbility
	default:
		return assessment.DimensionKind(kind)
	}
}
