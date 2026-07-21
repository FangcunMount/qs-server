package evaluation

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"

// ExecutionIdentity 路由execution 到 concrete evaluator 实现。
type ExecutionIdentity struct {
	Kind      modelcatalog.Kind
	SubKind   modelcatalog.SubKind
	Algorithm modelcatalog.Algorithm
}

var (
	ExecutionIdentityScaleDefault = ExecutionIdentity{
		Kind:      modelcatalog.KindScale,
		SubKind:   modelcatalog.SubKindEmpty,
		Algorithm: modelcatalog.AlgorithmScaleDefault,
	}
	ExecutionIdentityPersonalityTypology = ExecutionIdentity{
		Kind:      modelcatalog.KindTypology,
		SubKind:   modelcatalog.SubKindTypology,
		Algorithm: modelcatalog.AlgorithmPersonalityTypology,
	}
	// ExecutionIdentityCognitiveDefault is the SPM cognitive route identity.
	ExecutionIdentityCognitiveDefault = CognitiveIdentity(modelcatalog.AlgorithmSPM)
)

// PersonalityTypologyIdentity 构建执行路由身份 用于 类型学算法。
func PersonalityTypologyIdentity(algorithm modelcatalog.Algorithm) ExecutionIdentity {
	return ExecutionIdentity{
		Kind:      modelcatalog.KindTypology,
		SubKind:   modelcatalog.SubKindTypology,
		Algorithm: algorithm,
	}
}

// BehavioralRatingIdentity builds the exact execution route key for a behavioral algorithm.
func BehavioralRatingIdentity(algorithm modelcatalog.Algorithm) ExecutionIdentity {
	return ExecutionIdentity{
		Kind:      modelcatalog.KindBehavioralRating,
		SubKind:   modelcatalog.SubKindEmpty,
		Algorithm: algorithm,
	}
}

// CognitiveIdentity builds the exact execution route key for a cognitive algorithm.
func CognitiveIdentity(algorithm modelcatalog.Algorithm) ExecutionIdentity {
	return ExecutionIdentity{
		Kind:      modelcatalog.KindCognitive,
		SubKind:   modelcatalog.SubKindEmpty,
		Algorithm: algorithm,
	}
}

func (id ExecutionIdentity) String() string {
	if id.SubKind == "" && id.Algorithm == "" {
		return id.Kind.String()
	}
	return id.Kind.String() + "/" + id.SubKind.String() + "/" + id.Algorithm.String()
}

func (id ExecutionIdentity) IsZero() bool {
	return id.Kind == "" && id.SubKind == "" && id.Algorithm == ""
}
