package evaluation

import evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"

type (
	ModelKind         = evalpipeline.ModelKind
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
	ExecutionIdentityPersonalityTypology     = evalpipeline.ExecutionIdentityPersonalityTypology
	ExecutionIdentityBehavioralRatingDefault = evalpipeline.ExecutionIdentityBehavioralRatingDefault
	ExecutionIdentityCognitiveDefault        = evalpipeline.ExecutionIdentityCognitiveDefault
)

var (
	PersonalityTypologyIdentity     = evalpipeline.PersonalityTypologyIdentity
	ExecutionIdentityFromLegacyKind = evalpipeline.ExecutionIdentityFromLegacyKind
)
