package pipeline

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"

// ModelDescriptor 是规范 registration entry 用于 评估 model。
type ModelDescriptor struct {
	Kind      ModelKind
	Algorithm modelcatalog.Algorithm
}

// ExecutionIdentity 推导路由身份 用于 运行时描述符。
func (d ModelDescriptor) ExecutionIdentity() ExecutionIdentity {
	switch d.Kind {
	case ModelKindScale:
		return ExecutionIdentityScaleDefault
	case ModelKindBehavioralRating:
		return ExecutionIdentityBehavioralRatingDefault
	case ModelKindCognitive:
		return ExecutionIdentityCognitiveDefault
	case ModelKindTypology:
		if d.Algorithm != "" {
			return PersonalityTypologyIdentity(d.Algorithm)
		}
		return ExecutionIdentityPersonalityTypology
	default:
		return ExecutionIdentity{}
	}
}

// CognitiveModelDescriptor 返回内置 cognitive 运行时描述符。
func CognitiveModelDescriptor() ModelDescriptor {
	return ModelDescriptor{
		Kind:      ModelKindCognitive,
		Algorithm: modelcatalog.AlgorithmSPM,
	}
}

// BehavioralRatingModelDescriptor 返回内置 behavioral_rating 运行时描述符。
func BehavioralRatingModelDescriptor() ModelDescriptor {
	return ModelDescriptor{
		Kind:      ModelKindBehavioralRating,
		Algorithm: modelcatalog.AlgorithmBehavioralRatingDefault,
	}
}

// ScaleModelDescriptor 返回内置 scale 评估 描述符。
func ScaleModelDescriptor() ModelDescriptor {
	return ModelDescriptor{Kind: ModelKindScale}
}

// DefaultModelDescriptors 返回内置 scale 描述符 仅。
// Typology 描述符 是 owned 按 application 类型学.默认Modules() at 组合根。
func DefaultModelDescriptors() []ModelDescriptor {
	return []ModelDescriptor{ScaleModelDescriptor()}
}

// TypologyAlgorithms 返回类型学算法 从 描述符。
func TypologyAlgorithms(descs []ModelDescriptor) []modelcatalog.Algorithm {
	out := make([]modelcatalog.Algorithm, 0, len(descs))
	for _, desc := range descs {
		if desc.Kind != ModelKindTypology || desc.Algorithm == "" {
			continue
		}
		out = append(out, desc.Algorithm)
	}
	return out
}

// ExecutionPathForDescriptor 映射运行时描述符 到 its 物化路径。
func ExecutionPathForDescriptor(desc ModelDescriptor) (modelcatalog.ExecutionPath, error) {
	return ExecutionPathForModelKind(desc.Kind)
}
