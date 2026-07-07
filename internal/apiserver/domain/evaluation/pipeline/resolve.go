package pipeline

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// ModelKind distinguishes mechanism-oriented runtime descriptors during migration.
type ModelKind string

const (
	ModelKindScale            ModelKind = "scale"
	ModelKindTypology         ModelKind = "typology"
	ModelKindBehavioralRating ModelKind = "behavioral_rating"
	ModelKindCognitive        ModelKind = "cognitive"
)

// DecisionKindFromSnapshot resolves the decision strategy for runtime routing.
func DecisionKindFromSnapshot(snapshot modelcatalog.PublishedModelSnapshot) (modelcatalog.DecisionKind, bool) {
	if snapshot.Decision.Kind != "" {
		return snapshot.Decision.Kind, true
	}
	return modelcatalog.DecisionKindForIdentity(snapshot.Model.Kind, snapshot.Model.SubKind, snapshot.Model.Algorithm)
}

// ExecutionRoutingFromSnapshot is the single source for runtime and report mechanism routing.
// Legacy model kinds route by execution path family; DecisionKindForIdentity remains for publish matrices.
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

// RuntimeDescriptorKeyFromSnapshot derives mechanism routing keys from a published snapshot.
func RuntimeDescriptorKeyFromSnapshot(snapshot modelcatalog.PublishedModelSnapshot) (RuntimeDescriptorKey, error) {
	return ExecutionRoutingFromSnapshot(snapshot)
}

// ExecutionFamilyFromSnapshot resolves the execution family using kind-primary routing.
func ExecutionFamilyFromSnapshot(snapshot modelcatalog.PublishedModelSnapshot) (modelcatalog.AlgorithmFamily, bool) {
	if family, ok := executionFamilyFromModelKind(snapshot.Model); ok {
		return family, true
	}
	if snapshot.Decision.Kind != "" {
		return modelcatalog.AlgorithmFamilyFromDecisionKind(snapshot.Decision.Kind)
	}
	return modelcatalog.AlgorithmFamilyFromIdentity(snapshot.Model.Kind, snapshot.Model.SubKind, snapshot.Model.Algorithm)
}

// ExecutionDecisionFromSnapshot resolves decision kind aligned with the execution family.
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

// AlgorithmFamilyFromSnapshot resolves the execution family for a published snapshot.
func AlgorithmFamilyFromSnapshot(snapshot modelcatalog.PublishedModelSnapshot) (modelcatalog.AlgorithmFamily, bool) {
	return ExecutionFamilyFromSnapshot(snapshot)
}

// AlgorithmFamilyFromModelKind maps legacy model-kind descriptors to mechanism families.
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

// ExecutionPathForFamily maps an algorithm family to its materialization path.
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

// ExecutionPathForModelKind maps a legacy model kind to its materialization path.
func ExecutionPathForModelKind(kind ModelKind) (modelcatalog.ExecutionPath, error) {
	family, ok := AlgorithmFamilyFromModelKind(kind)
	if !ok {
		return "", fmt.Errorf("unsupported evaluation model kind: %s", kind)
	}
	return ExecutionPathForFamily(family)
}
