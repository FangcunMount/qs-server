package runtime

import (
	"fmt"

	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// FamilyManifestEntry is one AlgorithmFamily that must be fully wired for
// Evaluation runtime (EV-R014). Modules keep their own factories; this list is
// the completeness contract.
type FamilyManifestEntry struct {
	Family modelcatalog.AlgorithmFamily
	Path   modelcatalog.ExecutionPath
}

// RequiredFamilyManifest is the single source of truth for path↔family pairs
// that DefaultRuntimeDescriptorRegistry, InputProvider materialization, and
// AttachNativePipelines must cover.
func RequiredFamilyManifest() []FamilyManifestEntry {
	return []FamilyManifestEntry{
		{Family: modelcatalog.AlgorithmFamilyFactorScoring, Path: modelcatalog.ExecutionPathScaleDescriptor},
		{Family: modelcatalog.AlgorithmFamilyFactorClassification, Path: modelcatalog.ExecutionPathTypologyDescriptor},
		{Family: modelcatalog.AlgorithmFamilyFactorNorm, Path: modelcatalog.ExecutionPathBehavioralRatingDescriptor},
		{Family: modelcatalog.AlgorithmFamilyTaskPerformance, Path: modelcatalog.ExecutionPathCognitiveDescriptor},
	}
}

// ValidateFamilyManifestCompleteness fails when a required family is missing
// from the registry, has the wrong ExecutionPath, or lacks a native pipeline
// triple after AttachNativePipelines.
func ValidateFamilyManifestCompleteness(registry *evalpipeline.RuntimeDescriptorRegistry) error {
	if registry == nil {
		return fmt.Errorf("runtime descriptor registry is nil")
	}
	for _, entry := range RequiredFamilyManifest() {
		desc, ok := registry.DescriptorForFamily(entry.Family)
		if !ok {
			return fmt.Errorf("EV-R014: missing runtime descriptor for algorithm family %s", entry.Family)
		}
		if desc.ExecutionPath != entry.Path {
			return fmt.Errorf("EV-R014: family %s execution path = %s, want %s", entry.Family, desc.ExecutionPath, entry.Path)
		}
		if desc.InputAssembler == nil || desc.Calculator == nil || desc.OutcomeAssembler == nil {
			return fmt.Errorf("EV-R014: incomplete native pipeline for algorithm family %s", entry.Family)
		}
	}
	return nil
}
