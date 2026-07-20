package typology

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

// AlgorithmBackfillEligibility describes whether a typology identity rewrite is safe.
type AlgorithmBackfillEligibility struct {
	Eligible bool
	Reason   string
	From     binding.Algorithm
	To       binding.Algorithm
}

// EvaluateAlgorithmBackfill reports whether rewriting retained typology algorithm
// to personality_typology preserves configured runtime semantics (MC-R018 batch 3).
//
// Safe when RuntimeSpec can be derived from DefinitionV2 (algorithm-independent)
// or when the wire payload already carries an explicit Runtime section.
func EvaluateAlgorithmBackfill(algorithm binding.Algorithm, def *definition.Definition, payload *Payload) AlgorithmBackfillEligibility {
	target, ok := identity.TypologyAlgorithmBackfillTarget(algorithm)
	if !ok {
		return AlgorithmBackfillEligibility{
			Eligible: false, Reason: "not_retained_read_alias", From: algorithm,
		}
	}
	out := AlgorithmBackfillEligibility{From: algorithm, To: target}
	if def != nil {
		spec, defErr := RuntimeSpecFromDefinition(def)
		if defErr == nil && spec != nil {
			if validErr := validateRuntimeSpec(spec); validErr == nil {
				out.Eligible = true
				return out
			} else {
				out.Reason = "definition_runtime_invalid:" + validErr.Error()
				return out
			}
		}
		if defErr != nil {
			out.Reason = "definition_runtime_unavailable:" + defErr.Error()
		}
	}
	if payload != nil && payload.HasExplicitRuntime() {
		spec, err := payload.ToRuntimeSpec()
		if err != nil {
			out.Reason = "explicit_runtime_invalid:" + err.Error()
			return out
		}
		if err := validateRuntimeSpec(spec); err != nil {
			out.Reason = "explicit_runtime_invalid:" + err.Error()
			return out
		}
		// Ensure rewriting algorithm would not change derived legacy sections:
		// with explicit Runtime, ToRuntimeSpec ignores algorithm for required fields.
		rewritten := clonePayloadForAlgorithmCheck(payload, target)
		after, err := rewritten.ToRuntimeSpec()
		if err != nil {
			out.Reason = "rewritten_runtime_invalid:" + err.Error()
			return out
		}
		if !runtimeSpecCoreEqual(spec, after) {
			out.Reason = "runtime_drift_after_algorithm_rewrite"
			return out
		}
		out.Eligible = true
		out.Reason = ""
		return out
	}
	if out.Reason == "" {
		out.Reason = "requires_definition_v2_or_explicit_runtime"
	}
	return out
}

func clonePayloadForAlgorithmCheck(payload *Payload, algorithm binding.Algorithm) *Payload {
	if payload == nil {
		return nil
	}
	cloned := *payload
	cloned.Algorithm = algorithm
	return &cloned
}

func runtimeSpecCoreEqual(left, right *RuntimeSpec) bool {
	if left == nil || right == nil {
		return left == right
	}
	if left.Decision.Kind != right.Decision.Kind {
		return false
	}
	if left.OutcomeMapping.DetailKind != right.OutcomeMapping.DetailKind {
		return false
	}
	if left.OutcomeMapping.ResolvedDetailAdapterKey(left.Decision.Kind) != right.OutcomeMapping.ResolvedDetailAdapterKey(right.Decision.Kind) {
		return false
	}
	if left.Report.Kind != right.Report.Kind {
		return false
	}
	return true
}

// ErrAlgorithmBackfillIneligible is returned by callers that require eligibility.
var ErrAlgorithmBackfillIneligible = fmt.Errorf("typology algorithm backfill is not eligible")
