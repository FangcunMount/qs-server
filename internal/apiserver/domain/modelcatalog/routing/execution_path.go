package routing

// ExecutionPath names how a model family is materialized into evaluation runtime registries.
type ExecutionPath string

const (
	ExecutionPathNone                       ExecutionPath = "none"
	ExecutionPathScaleDescriptor            ExecutionPath = "scale_descriptor"
	ExecutionPathTypologyDescriptor         ExecutionPath = "typology_descriptor"
	ExecutionPathBehavioralRatingDescriptor ExecutionPath = "behavioral_rating_descriptor"
	ExecutionPathCognitiveDescriptor        ExecutionPath = "cognitive_descriptor"
)
