package trait

import calcclassification "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/classification"

type FactorID = calcclassification.FactorID
type FactorKind = calcclassification.FactorKind
type AggregationMethod = calcclassification.AggregationMethod
type OptionScoringPolicy = calcclassification.OptionScoringPolicy
type AnswerContribution = calcclassification.AnswerContribution
type LeafScoringSpec = calcclassification.LeafScoringSpec
type PersonalityFactor = calcclassification.PersonalityFactor

const (
	FactorKindLeaf      = calcclassification.FactorKindLeaf
	FactorKindComposite = calcclassification.FactorKindComposite

	AggregationSum         = calcclassification.AggregationSum
	AggregationAvg         = calcclassification.AggregationAvg
	AggregationWeightedAvg = calcclassification.AggregationWeightedAvg

	OptionScoringStrict = calcclassification.OptionScoringStrict
	OptionScoringCompat = calcclassification.OptionScoringCompat
)
