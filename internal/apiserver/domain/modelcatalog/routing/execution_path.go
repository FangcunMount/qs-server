package routing

// ExecutionPath 命名如何 模型家族 是 materialized 为 评估执行time 注册表。
type ExecutionPath string

const (
	ExecutionPathNone                       ExecutionPath = "none"
	ExecutionPathScaleDescriptor            ExecutionPath = "scale_descriptor"
	ExecutionPathTypologyDescriptor         ExecutionPath = "typology_descriptor"
	ExecutionPathBehavioralRatingDescriptor ExecutionPath = "behavioral_rating_descriptor"
	ExecutionPathCognitiveDescriptor        ExecutionPath = "cognitive_descriptor"
)
