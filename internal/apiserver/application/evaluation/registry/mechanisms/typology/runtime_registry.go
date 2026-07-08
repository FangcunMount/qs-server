package typology

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	configuredadapter "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/typology/adapter/configured"
	personalityconfigured "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/typology/configured"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// PersonalityRuntimeRegistry 解析类型学 execution 能力 按 评估器键 和 算法别名。
type PersonalityRuntimeRegistry struct {
	assembler      OutcomeAssembler
	reportRegistry ReportAdapterRegistry
	configured     configuredadapter.Adapter
	aliases        map[modelcatalog.Algorithm]configuredadapter.Adapter
}

// 默认PersonalityRuntimeRegistry 构建默认 配置化 类型学 运行时。
func DefaultPersonalityRuntimeRegistry() PersonalityRuntimeRegistry {
	return NewPersonalityRuntimeRegistry(
		DefaultAlgorithmAliases()...,
	)
}

// NewPersonalityRuntimeRegistry registers 算法别名 over 配置化运行时。
func NewPersonalityRuntimeRegistry(algorithms ...modelcatalog.Algorithm) PersonalityRuntimeRegistry {
	return NewPersonalityRuntimeRegistryWith(PersonalityRuntimeOptions{}, algorithms...)
}

// NewPersonalityRuntimeRegistryWith registers 算法别名 使用 injectable adapter 注册表。
func NewPersonalityRuntimeRegistryWith(opts PersonalityRuntimeOptions, algorithms ...modelcatalog.Algorithm) PersonalityRuntimeRegistry {
	opts = resolvePersonalityRuntimeOptions(opts)
	evaluator := personalityconfigured.NewEvaluatorWithDetails(opts.DetailRegistry)
	aliases := make(map[modelcatalog.Algorithm]configuredadapter.Adapter, len(algorithms))
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

func (r PersonalityRuntimeRegistry) runnerForIdentity(key evaluation.ExecutionIdentity) (algorithmRunner, error) {
	switch evaluation.ResolvePersonalityTypologyExecutorIdentity(key) {
	case evaluation.ExecutionIdentityPersonalityTypology:
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

func (r PersonalityRuntimeRegistry) runnerFor(algorithm modelcatalog.Algorithm) (algorithmRunner, error) {
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

func (r PersonalityRuntimeRegistry) Algorithms() []modelcatalog.Algorithm {
	out := make([]modelcatalog.Algorithm, 0, len(r.aliases))
	for algorithm := range r.aliases {
		out = append(out, algorithm)
	}
	return out
}

func (r PersonalityRuntimeRegistry) Len() int {
	return len(r.aliases)
}

// AsModuleRegistry 适配运行时 注册表 到 旧版 module 注册表 API。
func (r PersonalityRuntimeRegistry) AsModuleRegistry() ModuleRegistry {
	return ModuleRegistry{runtime: r}
}
