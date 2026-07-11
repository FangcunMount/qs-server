package calculationadapter

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
)

func CalculationKindFromOutcome(kind domainoutcome.DimensionKind) calculation.DimensionKind {
	switch kind {
	case domainoutcome.DimensionKindFactor:
		return calculation.DimensionKindFactor
	case domainoutcome.DimensionKindPole:
		return calculation.DimensionKindPole
	case domainoutcome.DimensionKindTrait:
		return calculation.DimensionKindTrait
	case domainoutcome.DimensionKindIndex:
		return calculation.DimensionKindIndex
	case domainoutcome.DimensionKindAbility:
		return calculation.DimensionKindAbility
	default:
		return calculation.DimensionKind(kind)
	}
}

func OutcomeKindFromCalculation(kind calculation.DimensionKind) domainoutcome.DimensionKind {
	switch kind {
	case calculation.DimensionKindFactor:
		return domainoutcome.DimensionKindFactor
	case calculation.DimensionKindPole:
		return domainoutcome.DimensionKindPole
	case calculation.DimensionKindTrait:
		return domainoutcome.DimensionKindTrait
	case calculation.DimensionKindIndex:
		return domainoutcome.DimensionKindIndex
	case calculation.DimensionKindAbility:
		return domainoutcome.DimensionKindAbility
	default:
		return domainoutcome.DimensionKind(kind)
	}
}
