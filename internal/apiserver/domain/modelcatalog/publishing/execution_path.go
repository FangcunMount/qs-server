package publishing

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"

type ExecutionPath = binding.ExecutionPath

const (
	ExecutionPathNone                       = binding.ExecutionPathNone
	ExecutionPathScaleDescriptor            = binding.ExecutionPathScaleDescriptor
	ExecutionPathTypologyDescriptor         = binding.ExecutionPathTypologyDescriptor
	ExecutionPathBehavioralRatingDescriptor = binding.ExecutionPathBehavioralRatingDescriptor
	ExecutionPathCognitiveDescriptor        = binding.ExecutionPathCognitiveDescriptor
)
