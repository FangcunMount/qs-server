package pipeline

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// ModelKind 区分面向机制 运行时描述符 在 迁移。
type ModelKind string

const (
	ModelKindScale            ModelKind = "scale"
	ModelKindTypology         ModelKind = "typology"
	ModelKindBehavioralRating ModelKind = "behavioral_rating"
	ModelKindCognitive        ModelKind = "cognitive"
)

// DecisionKindFromRoute 解析判定策略用于运行时路由；仅使用 route 显式 decision.kind。
func DecisionKindFromRoute(route ModelRoute) (modelcatalog.DecisionKind, bool) {
	if route.DecisionKind != "" {
		return route.DecisionKind, true
	}
	return "", false
}

// ExecutionRoutingFromRoute 是单一 来源 用于 运行时 和 report 机制 路由。
// Legacy 建模类型 路由 按 执行路径家族; 判定类型For身份 保持 用于 publish matrices。
func ExecutionRoutingFromRoute(route ModelRoute) (RuntimeDescriptorKey, error) {
	family, ok := ExecutionFamilyFromRoute(route)
	if !ok {
		return RuntimeDescriptorKey{}, fmt.Errorf("unsupported model route for runtime descriptor: %s/%s", route.Kind, route.Algorithm)
	}
	return RuntimeDescriptorKey{
		AlgorithmFamily: family,
		DecisionKind:    ExecutionDecisionFromRoute(route, family),
		PayloadFormat:   route.PayloadFormat,
	}, nil
}

// RuntimeDescriptorKeyFromRoute 推导机制 路由 键 从 模型路由。
func RuntimeDescriptorKeyFromRoute(route ModelRoute) (RuntimeDescriptorKey, error) {
	return ExecutionRoutingFromRoute(route)
}

// ExecutionFamilyFromRoute 解析执行家族 using modelcatalog identity as the primary route.
func ExecutionFamilyFromRoute(route ModelRoute) (modelcatalog.AlgorithmFamily, bool) {
	if family, ok := modelcatalog.AlgorithmFamilyFromIdentity(route.Kind, route.SubKind, route.Algorithm); ok {
		return family, true
	}
	if family, ok := legacyTypologyFamilyFromRoute(route); ok {
		return family, true
	}
	if route.DecisionKind != "" {
		return modelcatalog.AlgorithmFamilyFromDecisionKind(route.DecisionKind)
	}
	return modelcatalog.AlgorithmFamilyFromIdentity(route.Kind, route.SubKind, route.Algorithm)
}

// ExecutionDecisionFromRoute 解析判定类型 aligned 使用 执行家族。
func ExecutionDecisionFromRoute(route ModelRoute, family modelcatalog.AlgorithmFamily) modelcatalog.DecisionKind {
	if route.DecisionKind != "" {
		if decisionFamily, ok := modelcatalog.AlgorithmFamilyFromDecisionKind(route.DecisionKind); ok && decisionFamily == family {
			return route.DecisionKind
		}
	}
	return defaultDecisionKindForFamily(family)
}

func legacyTypologyFamilyFromRoute(route ModelRoute) (modelcatalog.AlgorithmFamily, bool) {
	switch route.Kind {
	case modelcatalog.KindTypology, modelcatalog.KindPersonality:
		if route.SubKind == "" {
			return modelcatalog.AlgorithmFamilyFactorClassification, true
		}
	}
	return "", false
}

func defaultDecisionKindForFamily(family modelcatalog.AlgorithmFamily) modelcatalog.DecisionKind {
	switch family {
	case modelcatalog.AlgorithmFamilyFactorScoring:
		return modelcatalog.DecisionKindScoreRange
	case modelcatalog.AlgorithmFamilyFactorClassification:
		return modelcatalog.DecisionKindPoleComposition
	case modelcatalog.AlgorithmFamilyFactorNorm:
		return modelcatalog.DecisionKindNormLookup
	case modelcatalog.AlgorithmFamilyTaskPerformance:
		return modelcatalog.DecisionKindAbilityLevel
	default:
		return ""
	}
}

// AlgorithmFamilyFromRoute 解析执行家族 用于 模型路由。
func AlgorithmFamilyFromRoute(route ModelRoute) (modelcatalog.AlgorithmFamily, bool) {
	return ExecutionFamilyFromRoute(route)
}

// AlgorithmFamilyFromModelKind 映射旧版 模型类型描述符 到 机制家族。
func AlgorithmFamilyFromModelKind(kind ModelKind) (modelcatalog.AlgorithmFamily, bool) {
	switch kind {
	case ModelKindScale:
		return modelcatalog.AlgorithmFamilyFactorScoring, true
	case ModelKindTypology:
		return modelcatalog.AlgorithmFamilyFactorClassification, true
	case ModelKindBehavioralRating:
		return modelcatalog.AlgorithmFamilyFactorNorm, true
	case ModelKindCognitive:
		return modelcatalog.AlgorithmFamilyTaskPerformance, true
	default:
		return "", false
	}
}

// ExecutionPathForFamily 映射算法家族 到 its 物化路径。
func ExecutionPathForFamily(family modelcatalog.AlgorithmFamily) (modelcatalog.ExecutionPath, error) {
	switch family {
	case modelcatalog.AlgorithmFamilyFactorScoring:
		return modelcatalog.ExecutionPathScaleDescriptor, nil
	case modelcatalog.AlgorithmFamilyFactorClassification:
		return modelcatalog.ExecutionPathTypologyDescriptor, nil
	case modelcatalog.AlgorithmFamilyFactorNorm:
		return modelcatalog.ExecutionPathBehavioralRatingDescriptor, nil
	case modelcatalog.AlgorithmFamilyTaskPerformance:
		return modelcatalog.ExecutionPathCognitiveDescriptor, nil
	default:
		return "", fmt.Errorf("unsupported algorithm family: %s", family)
	}
}

// ExecutionPathForModelKind 映射旧模型类型 到 its 物化路径。
func ExecutionPathForModelKind(kind ModelKind) (modelcatalog.ExecutionPath, error) {
	family, ok := AlgorithmFamilyFromModelKind(kind)
	if !ok {
		return "", fmt.Errorf("unsupported evaluation model kind: %s", kind)
	}
	return ExecutionPathForFamily(family)
}
