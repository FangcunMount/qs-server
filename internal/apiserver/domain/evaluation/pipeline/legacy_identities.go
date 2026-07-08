package pipeline

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"

// Deprecated: use ExecutionIdentityPersonalityTypology for production routing; legacy algorithm aliases remain for compat decode.
var (
	ExecutionIdentityMBTI    = PersonalityTypologyIdentity(modelcatalog.AlgorithmMBTI)
	ExecutionIdentitySBTI    = PersonalityTypologyIdentity(modelcatalog.AlgorithmSBTI)
	ExecutionIdentityBigFive = PersonalityTypologyIdentity(modelcatalog.AlgorithmBigFive)
)

// PersonalityTypologyLegacyIdentities returns built-in typology algorithm routing aliases.
func PersonalityTypologyLegacyIdentities() []ExecutionIdentity {
	return []ExecutionIdentity{
		ExecutionIdentityMBTI,
		ExecutionIdentitySBTI,
		ExecutionIdentityBigFive,
	}
}
