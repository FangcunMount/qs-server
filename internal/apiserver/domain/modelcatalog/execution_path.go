package modelcatalog

// ExecutionPath names how a model family is materialized into evaluation runtime registries.
type ExecutionPath string

const (
	ExecutionPathNone                        ExecutionPath = "none"
	ExecutionPathScaleDescriptor             ExecutionPath = "scale_descriptor"
	ExecutionPathTypologyDescriptor          ExecutionPath = "typology_descriptor"
	ExecutionPathBehaviorAbilityScaleAdapter ExecutionPath = "behavior_ability_scale_adapter"
	ExecutionPathBehavioralRatingDescriptor  ExecutionPath = "behavioral_rating_descriptor"
	ExecutionPathCognitiveDescriptor         ExecutionPath = "cognitive_descriptor"
)
