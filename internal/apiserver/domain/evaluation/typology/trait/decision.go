package trait

import calcclassification "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/classification"

type DecisionKind = calcclassification.DecisionKind
type PoleSpec = calcclassification.PoleSpec
type PatternCandidate = calcclassification.PatternCandidate
type DecisionSpec = calcclassification.DecisionSpec
type LevelRule = calcclassification.LevelRule
type OutcomeCandidate = calcclassification.OutcomeCandidate

const (
	DecisionKindPoleComposition = calcclassification.DecisionKindPoleComposition
	DecisionKindNearestPattern  = calcclassification.DecisionKindNearestPattern
	DecisionKindTraitProfile    = calcclassification.DecisionKindTraitProfile
)
