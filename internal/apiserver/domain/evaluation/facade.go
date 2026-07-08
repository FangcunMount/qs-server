package evaluation

import evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"

type (
	ModelKind         = evalpipeline.ModelKind
	ModelDescriptor   = evalpipeline.ModelDescriptor
	ExecutionIdentity = evalpipeline.ExecutionIdentity
)

const (
	ModelKindScale            = evalpipeline.ModelKindScale
	ModelKindTypology         = evalpipeline.ModelKindTypology
	ModelKindBehavioralRating = evalpipeline.ModelKindBehavioralRating
	ModelKindCognitive        = evalpipeline.ModelKindCognitive
)

var (
	ExecutionIdentityScaleDefault            = evalpipeline.ExecutionIdentityScaleDefault
	ExecutionIdentityMBTI                    = evalpipeline.ExecutionIdentityMBTI
	ExecutionIdentitySBTI                    = evalpipeline.ExecutionIdentitySBTI
	ExecutionIdentityBigFive                 = evalpipeline.ExecutionIdentityBigFive
	ExecutionIdentityPersonalityTypology     = evalpipeline.ExecutionIdentityPersonalityTypology
	ExecutionIdentityBehavioralRatingDefault = evalpipeline.ExecutionIdentityBehavioralRatingDefault
	ExecutionIdentityCognitiveDefault        = evalpipeline.ExecutionIdentityCognitiveDefault
)

var (
	CognitiveModelDescriptor                   = evalpipeline.CognitiveModelDescriptor
	BehavioralRatingModelDescriptor            = evalpipeline.BehavioralRatingModelDescriptor
	ScaleModelDescriptor                       = evalpipeline.ScaleModelDescriptor
	DefaultModelDescriptors                    = evalpipeline.DefaultModelDescriptors
	TypologyAlgorithms                         = evalpipeline.TypologyAlgorithms
	PersonalityTypologyIdentity                = evalpipeline.PersonalityTypologyIdentity
	PersonalityTypologyLegacyIdentities        = evalpipeline.PersonalityTypologyLegacyIdentities
	ResolvePersonalityTypologyExecutorIdentity = evalpipeline.ResolvePersonalityTypologyExecutorIdentity
	ResolveBehavioralRatingExecutorIdentity    = evalpipeline.ResolveBehavioralRatingExecutorIdentity
	ExecutionIdentityFromLegacyKind            = evalpipeline.ExecutionIdentityFromLegacyKind
	ModelDescriptorFromIdentity                = evalpipeline.ModelDescriptorFromIdentity
	ExecutionPathForDescriptor                 = evalpipeline.ExecutionPathForDescriptor
)
