package pipeline

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
	ExecutionIdentityBehavioralRatingDefault = ExecutionIdentity{
		Kind:      modelcatalog.KindBehavioralRating,
		SubKind:   modelcatalog.SubKindEmpty,
		Algorithm: modelcatalog.AlgorithmBehavioralRatingDefault,
	}
	ExecutionIdentityCognitiveDefault = ExecutionIdentity{
		Kind:      modelcatalog.KindCognitive,
		SubKind:   modelcatalog.SubKindEmpty,
		Algorithm: modelcatalog.AlgorithmSPM,
	}
)

// PersonalityTypologyIdentity 构建执行路由身份 用于 类型学算法。
func PersonalityTypologyIdentity(algorithm modelcatalog.Algorithm) ExecutionIdentity {
	return ExecutionIdentity{
		Kind:      modelcatalog.KindTypology,
		SubKind:   modelcatalog.SubKindTypology,
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

func ExecutionIdentityFromLegacyKind(kind modelcatalog.Kind) (ExecutionIdentity, bool) {
	mappedKind, subKind, algorithm, ok := modelcatalog.LegacyKindMapping(kind)
	if !ok {
		return ExecutionIdentity{}, false
	}
	return ExecutionIdentity{Kind: mappedKind, SubKind: subKind, Algorithm: algorithm}, true
}

// ModelDescriptorFromIdentity 映射路由身份 到 its 运行时描述符。
func ModelDescriptorFromIdentity(id ExecutionIdentity) ModelDescriptor {
	switch {
	case id == ExecutionIdentityScaleDefault:
		return ScaleModelDescriptor()
	case id == ExecutionIdentityBehavioralRatingDefault:
		return BehavioralRatingModelDescriptor()
	case id == ExecutionIdentityCognitiveDefault:
		return CognitiveModelDescriptor()
	case id == ExecutionIdentityPersonalityTypology:
		return ModelDescriptor{
			Kind:      ModelKindTypology,
			Algorithm: modelcatalog.AlgorithmPersonalityTypology,
		}
	case id.Kind == modelcatalog.KindTypology && id.SubKind == modelcatalog.SubKindTypology && id.Algorithm != "":
		return ModelDescriptor{Kind: ModelKindTypology, Algorithm: id.Algorithm}
	default:
		return ModelDescriptor{}
	}
}
