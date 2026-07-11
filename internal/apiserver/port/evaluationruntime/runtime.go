// Package evaluationruntime exposes model-routing contracts shared with
// consumers of Evaluation facts without exposing Evaluation implementation paths.
package evaluationruntime

import (
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
)

type (
	ExecutionIdentity = evaldomain.ExecutionIdentity
	ModelDescriptor   = evaldomain.ModelDescriptor
)

var (
	ExecutionIdentityScaleDefault            = evaldomain.ExecutionIdentityScaleDefault
	ExecutionIdentityPersonalityTypology     = evaldomain.ExecutionIdentityPersonalityTypology
	ExecutionIdentityBehavioralRatingDefault = evaldomain.ExecutionIdentityBehavioralRatingDefault
	ExecutionIdentityCognitiveDefault        = evaldomain.ExecutionIdentityCognitiveDefault
	PersonalityTypologyIdentity              = evaldomain.PersonalityTypologyIdentity
	ExecutionPathForDescriptor               = evaldomain.ExecutionPathForDescriptor
)
