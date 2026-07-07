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
	ExecutionIdentityMBTI                = PersonalityTypologyIdentity(modelcatalog.AlgorithmMBTI)
	ExecutionIdentitySBTI                = PersonalityTypologyIdentity(modelcatalog.AlgorithmSBTI)
	ExecutionIdentityBigFive             = PersonalityTypologyIdentity(modelcatalog.AlgorithmBigFive)
	ExecutionIdentityPersonalityTypology = ExecutionIdentity{
		Kind:      modelcatalog.KindPersonality,
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
		Kind:      modelcatalog.KindPersonality,
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

// IsPersonalityTypologyLegacyIdentity 报告是否 身份 是 内置 类型学算法 别名。
func (id ExecutionIdentity) IsPersonalityTypologyLegacyIdentity() bool {
	if id.Kind != modelcatalog.KindPersonality || id.SubKind != modelcatalog.SubKindTypology {
		return false
	}
	switch id.Algorithm {
	case modelcatalog.AlgorithmMBTI, modelcatalog.AlgorithmSBTI, modelcatalog.AlgorithmBigFive:
		return true
	default:
		return false
	}
}

// PersonalityTypologyLegacyIdentities 返回内置 类型学算法 路由身份。
func PersonalityTypologyLegacyIdentities() []ExecutionIdentity {
	return []ExecutionIdentity{
		ExecutionIdentityMBTI,
		ExecutionIdentitySBTI,
		ExecutionIdentityBigFive,
	}
}

// ResolvePersonalityTypologyExecutorIdentity 映射旧版 类型学 身份 到 配置化运行时 身份。
func ResolvePersonalityTypologyExecutorIdentity(id ExecutionIdentity) ExecutionIdentity {
	if id == ExecutionIdentityPersonalityTypology || id.IsPersonalityTypologyLegacyIdentity() {
		return ExecutionIdentityPersonalityTypology
	}
	return id
}

// ResolveBehavioralRatingExecutorIdentity 映射算法-特定 身份 到 配置化运行时 executor。
func ResolveBehavioralRatingExecutorIdentity(id ExecutionIdentity) ExecutionIdentity {
	switch id.Kind {
	case modelcatalog.KindBehavioralRating:
		return ExecutionIdentityBehavioralRatingDefault
	default:
		return id
	}
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
	case id.Kind == modelcatalog.KindPersonality && id.SubKind == modelcatalog.SubKindTypology && id.Algorithm != "":
		return ModelDescriptor{Kind: ModelKindTypology, Algorithm: id.Algorithm}
	default:
		return ModelDescriptor{}
	}
}
