package typology

import (
	"fmt"

	typologylegacy "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/personality/typology/legacy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	configuredadapter "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/adapter/configured"
	personalityconfigured "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/personality/configured"
)

// PersonalityRuntimeRegistry resolves typology execution capabilities by evaluator key and algorithm alias.
type PersonalityRuntimeRegistry struct {
	assembler      OutcomeAssembler
	reportRegistry ReportAdapterRegistry
	configured     configuredadapter.Adapter
	aliases        map[assessmentmodel.Algorithm]configuredadapter.Adapter
}

// DefaultPersonalityRuntimeRegistry builds the default configured typology runtime.
func DefaultPersonalityRuntimeRegistry() PersonalityRuntimeRegistry {
	return NewPersonalityRuntimeRegistry(
		typologylegacy.DefaultAlgorithmAliases()...,
	)
}

// NewPersonalityRuntimeRegistry registers algorithm aliases over the configured runtime.
func NewPersonalityRuntimeRegistry(algorithms ...assessmentmodel.Algorithm) PersonalityRuntimeRegistry {
	return NewPersonalityRuntimeRegistryWith(PersonalityRuntimeOptions{}, algorithms...)
}

// NewPersonalityRuntimeRegistryWith registers algorithm aliases with injectable adapter registries.
func NewPersonalityRuntimeRegistryWith(opts PersonalityRuntimeOptions, algorithms ...assessmentmodel.Algorithm) PersonalityRuntimeRegistry {
	opts = resolvePersonalityRuntimeOptions(opts)
	evaluator := personalityconfigured.NewEvaluatorWithDetails(opts.DetailRegistry)
	aliases := make(map[assessmentmodel.Algorithm]configuredadapter.Adapter, len(algorithms))
	for _, algorithm := range algorithms {
		if algorithm == "" {
			continue
		}
		aliases[algorithm] = configuredadapter.NewAdapterWithEvaluator(algorithm, evaluator)
	}
	return PersonalityRuntimeRegistry{
		assembler:      NewOutcomeAssemblerWithRegistry(opts.OutcomeRegistry),
		reportRegistry: opts.ReportRegistry,
		configured:     configuredadapter.NewRuntimeAdapterWithEvaluator(evaluator),
		aliases:        aliases,
	}
}

func (r PersonalityRuntimeRegistry) runnerForKey(key evaluation.EvaluatorKey) (algorithmRunner, error) {
	switch evaluation.ResolvePersonalityTypologyExecutorKey(key) {
	case evaluation.EvaluatorKeyPersonalityTypology:
		return r.runnerForConfigured(), nil
	default:
		return algorithmRunner{}, fmt.Errorf("unsupported typology evaluator key: %s", key)
	}
}

func (r PersonalityRuntimeRegistry) runnerForConfigured() algorithmRunner {
	return algorithmRunner{
		adapter:          r.configured,
		outcomeAssembler: r.assembler,
		reportRegistry:   r.reportRegistry,
	}
}

func (r PersonalityRuntimeRegistry) runnerFor(algorithm assessmentmodel.Algorithm) (algorithmRunner, error) {
	adapter, ok := r.aliases[algorithm]
	if !ok {
		return algorithmRunner{}, fmt.Errorf("unsupported typology algorithm: %s", algorithm)
	}
	return algorithmRunner{
		adapter:          adapter,
		outcomeAssembler: r.assembler,
		reportRegistry:   r.reportRegistry,
	}, nil
}

func (r PersonalityRuntimeRegistry) Algorithms() []assessmentmodel.Algorithm {
	out := make([]assessmentmodel.Algorithm, 0, len(r.aliases))
	for algorithm := range r.aliases {
		out = append(out, algorithm)
	}
	return out
}

func (r PersonalityRuntimeRegistry) Len() int {
	return len(r.aliases)
}

// AsModuleRegistry adapts the runtime registry to the legacy module registry API.
func (r PersonalityRuntimeRegistry) AsModuleRegistry() ModuleRegistry {
	return ModuleRegistry{runtime: r}
}
