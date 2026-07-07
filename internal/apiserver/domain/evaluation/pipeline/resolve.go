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

// RuntimeDescriptorKeyFromSnapshot derives mechanism routing keys from a published snapshot.
func RuntimeDescriptorKeyFromSnapshot(snapshot modelcatalog.PublishedModelSnapshot) (RuntimeDescriptorKey, error) {
	family, ok := AlgorithmFamilyFromSnapshot(snapshot)
	if !ok {
		return RuntimeDescriptorKey{}, fmt.Errorf("unsupported snapshot identity for runtime descriptor: %s/%s", snapshot.Model.Kind, snapshot.Model.Algorithm)
	}
	return RuntimeDescriptorKey{
		AlgorithmFamily: family,
		PayloadFormat:   snapshot.PayloadFormat,
	}, nil
}

// AlgorithmFamilyFromSnapshot resolves the execution family for a published snapshot.
func AlgorithmFamilyFromSnapshot(snapshot modelcatalog.PublishedModelSnapshot) (modelcatalog.AlgorithmFamily, bool) {
	if snapshot.Decision.Kind != "" {
		return modelcatalog.AlgorithmFamilyFromDecisionKind(snapshot.Decision.Kind)
	}
	return modelcatalog.AlgorithmFamilyFromIdentity(snapshot.Model.Kind, snapshot.Model.SubKind, snapshot.Model.Algorithm)
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
