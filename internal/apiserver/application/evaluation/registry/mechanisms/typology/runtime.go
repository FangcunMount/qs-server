package typology

import (
	"fmt"

	configuredadapter "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology/runtime/adapter/configured"
	personalityconfigured "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry/mechanisms/typology/runtime/configured"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// PersonalityRuntime is the single configured factor-classification runtime.
// Model algorithms are data, not executable aliases registered in Evaluation.
type PersonalityRuntime struct {
	assembler  OutcomeAssembler
	configured configuredadapter.Adapter
}

func DefaultPersonalityRuntime() PersonalityRuntime {
	return NewPersonalityRuntime(PersonalityRuntimeOptions{})
}

func NewPersonalityRuntime(opts PersonalityRuntimeOptions) PersonalityRuntime {
	opts = resolvePersonalityRuntimeOptions(opts)
	evaluator := personalityconfigured.NewEvaluatorWithDetails(opts.DetailRegistry)
	return PersonalityRuntime{
		assembler:  NewOutcomeAssemblerWithRegistry(opts.OutcomeRegistry),
		configured: configuredadapter.NewRuntimeAdapterWithEvaluator(evaluator),
	}
}

func (r PersonalityRuntime) runnerForIdentity(key evaluation.ExecutionIdentity) (algorithmRunner, error) {
	if key != evaluation.ExecutionIdentityPersonalityTypology &&
		(key.Kind != modelcatalog.KindTypology || key.SubKind != modelcatalog.SubKindTypology) {
		return algorithmRunner{}, fmt.Errorf("unsupported typology evaluator key: %s", key)
	}
	return algorithmRunner{adapter: r.configured, outcomeAssembler: r.assembler}, nil
}
