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

// DecisionKindFromSnapshot 解析判定策略 用于 运行时路由。
func DecisionKindFromSnapshot(snapshot modelcatalog.PublishedModelSnapshot) (modelcatalog.DecisionKind, bool) {
	if snapshot.Decision.Kind != "" {
		return snapshot.Decision.Kind, true
	}
	return modelcatalog.DecisionKindForIdentity(snapshot.Model.Kind, snapshot.Model.SubKind, snapshot.Model.Algorithm)
}

// ExecutionRoutingFromSnapshot 是单一 来源 用于 运行时 和 report 机制 路由。
// Legacy 建模类型 路由 按 执行路径家族; 判定类型For身份 保持 用于 publish matrices。
func ExecutionRoutingFromSnapshot(snapshot modelcatalog.PublishedModelSnapshot) (RuntimeDescriptorKey, error) {
	family, ok := ExecutionFamilyFromSnapshot(snapshot)
	if !ok {
		return RuntimeDescriptorKey{}, fmt.Errorf("unsupported snapshot identity for runtime descriptor: %s/%s", snapshot.Model.Kind, snapshot.Model.Algorithm)
	}
	return RuntimeDescriptorKey{
		AlgorithmFamily: family,
		DecisionKind:    ExecutionDecisionFromSnapshot(snapshot, family),
		PayloadFormat:   snapshot.PayloadFormat,
	}, nil
}

// RuntimeDescriptorKeyFromSnapshot 推导机制 路由 键 从 已发布快照。
func RuntimeDescriptorKeyFromSnapshot(snapshot modelcatalog.PublishedModelSnapshot) (RuntimeDescriptorKey, error) {
	return ExecutionRoutingFromSnapshot(snapshot)
}

// ExecutionFamilyFromSnapshot 解析执行家族 using 类型-主 路由。
func ExecutionFamilyFromSnapshot(snapshot modelcatalog.PublishedModelSnapshot) (modelcatalog.AlgorithmFamily, bool) {
	if family, ok := executionFamilyFromModelKind(snapshot.Model); ok {
		return family, true
	}
	if snapshot.Decision.Kind != "" {
		return modelcatalog.AlgorithmFamilyFromDecisionKind(snapshot.Decision.Kind)
	}
	return modelcatalog.AlgorithmFamilyFromIdentity(snapshot.Model.Kind, snapshot.Model.SubKind, snapshot.Model.Algorithm)
}

// ExecutionDecisionFromSnapshot 解析判定类型 aligned 使用 执行家族。
func ExecutionDecisionFromSnapshot(snapshot modelcatalog.PublishedModelSnapshot, family modelcatalog.AlgorithmFamily) modelcatalog.DecisionKind {
	if snapshot.Decision.Kind != "" {
		if decisionFamily, ok := modelcatalog.AlgorithmFamilyFromDecisionKind(snapshot.Decision.Kind); ok && decisionFamily == family {
			return snapshot.Decision.Kind
		}
	}
	if family == modelcatalog.AlgorithmFamilyFactorClassification {
		if decision, ok := modelcatalog.DecisionKindForIdentity(snapshot.Model.Kind, snapshot.Model.SubKind, snapshot.Model.Algorithm); ok {
			return decision
		}
	}
	return defaultDecisionKindForFamily(family)
}

func executionFamilyFromModelKind(model modelcatalog.ModelDefinition) (modelcatalog.AlgorithmFamily, bool) {
	switch model.Kind {
	case modelcatalog.KindScale:
		return modelcatalog.AlgorithmFamilyFactorScoring, true
	case modelcatalog.KindPersonality:
		if model.SubKind == modelcatalog.SubKindTypology || model.SubKind == "" {
			return modelcatalog.AlgorithmFamilyFactorClassification, true
		}
	case modelcatalog.KindBehavioralRating:
		return modelcatalog.AlgorithmFamilyFactorNorm, true
	case modelcatalog.KindCognitive:
		return modelcatalog.AlgorithmFamilyTaskPerformance, true
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

// AlgorithmFamilyFromSnapshot 解析执行家族 用于 已发布快照。
func AlgorithmFamilyFromSnapshot(snapshot modelcatalog.PublishedModelSnapshot) (modelcatalog.AlgorithmFamily, bool) {
	return ExecutionFamilyFromSnapshot(snapshot)
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
