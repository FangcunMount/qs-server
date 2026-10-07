package evaluation_test

import (
	"testing"

	evalruntime "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	evaluationinputInfra "github.com/FangcunMount/qs-server/internal/apiserver/infra/evaluationinput"
)

func TestEvaluationModuleMaterializesProvidersForEveryDescriptorPath(t *testing.T) {
	t.Parallel()
	registry, err := evalruntime.DefaultRuntimeDescriptorRegistry()
	if err != nil {
		t.Fatal(err)
	}
	paths, err := evalruntime.ExecutionPathsFromRegistry(registry)
	if err != nil {
		t.Fatal(err)
	}
	// Only provider materialization is exercised; no catalog or Survey reads occur.
	providers, err := evaluationinputInfra.MaterializeInputProviders(paths, evaluationinputInfra.InputProviderDeps{
		ScaleCatalog:            evaluationinputInfra.NewPublishedScaleCatalog(nil),
		TypologyCatalog:         evaluationinputInfra.NewPublishedTypologyCatalog(nil),
		BehavioralRatingCatalog: evaluationinputInfra.NewPublishedBehavioralRatingCatalog(nil),
		CognitiveCatalog:        evaluationinputInfra.NewPublishedCognitiveCatalog(nil),
		AnswerSheets:            evaluationinputInfra.NewRepositoryAnswerSheetSnapshotReader(nil),
		Questionnaires:          evaluationinputInfra.NewRepositoryQuestionnaireSnapshotReader(nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	descriptorPaths := make(map[modelcatalog.ExecutionPath]bool, len(paths))
	for _, path := range paths {
		descriptorPaths[path] = true
	}
	expectedPaths := make([]modelcatalog.ExecutionPath, 0, len(providers))
	covered := make(map[modelcatalog.ExecutionPath]bool, len(paths))
	for _, provider := range providers {
		capability, ok := modelcatalog.FamilyCapabilityByKind(provider.ExecutionIdentity().Kind)
		if !ok || !descriptorPaths[capability.ExecutionPath] {
			t.Fatalf("provider identity %#v has no descriptor path", provider.ExecutionIdentity())
		}
		expectedPaths = append(expectedPaths, capability.ExecutionPath)
		covered[capability.ExecutionPath] = true
	}
	if err := assertExecutionPathParity(expectedPaths, providers); err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		if !covered[path] {
			t.Fatalf("descriptor path %s has no materialized input provider", path)
		}
	}
}
