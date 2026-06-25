package evaluationinput

import (
	rulesetmbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/mbti"
	rulesetsbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/sbti"
	rulesetscale "github.com/FangcunMount/qs-server/internal/apiserver/domain/ruleset/scale"
)

type ScaleSnapshot = rulesetscale.ScaleSnapshot
type FactorSnapshot = rulesetscale.FactorSnapshot
type ScoringParamsSnapshot = rulesetscale.ScoringParamsSnapshot
type InterpretRuleSnapshot = rulesetscale.InterpretRuleSnapshot

type MBTIModelSnapshot = rulesetmbti.ModelSnapshot
type MBTISourceSnapshot = rulesetmbti.SourceSnapshot
type MBTIDimensionSnapshot = rulesetmbti.DimensionSnapshot
type MBTIQuestionMappingSnapshot = rulesetmbti.QuestionMappingSnapshot
type MBTITypeProfileSnapshot = rulesetmbti.TypeProfileSnapshot

type SBTIModelSnapshot = rulesetsbti.ModelSnapshot
type SBTISourceSnapshot = rulesetsbti.SourceSnapshot
type SBTIDimensionSnapshot = rulesetsbti.DimensionSnapshot
type SBTIQuestionMappingSnapshot = rulesetsbti.QuestionMappingSnapshot
type SBTIOutcomeSnapshot = rulesetsbti.OutcomeSnapshot
type SBTIRaritySnapshot = rulesetsbti.RaritySnapshot
type SBTIDrinkTriggerSnapshot = rulesetsbti.DrinkTriggerSnapshot
