package behavioral

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

// AlgorithmBackfillEligibility describes whether a behavioral identity rewrite is safe.
type AlgorithmBackfillEligibility struct {
	Eligible bool
	Reason   string
	From     binding.Algorithm
	To       binding.Algorithm
}

// EvaluateAlgorithmBackfill reports whether rewriting behavioral_rating_default
// is safe (MC-R018 batch 4).
//
// preferredTarget is required when Definition has NormRefs but no Brief2Spec
// (brief2 vs spm_sensory cannot be inferred). Pass empty to only auto-accept
// Brief2Spec-backed snapshots.
func EvaluateAlgorithmBackfill(algorithm binding.Algorithm, def *definition.Definition, preferredTarget binding.Algorithm) AlgorithmBackfillEligibility {
	hasBrief2 := def != nil && def.Execution.Brief2 != nil
	hasNorms := def != nil && len(def.Calibration.NormRefs) > 0
	target, reason, ok := identity.BehavioralAlgorithmBackfillTarget(algorithm, hasBrief2, hasNorms, preferredTarget)
	out := AlgorithmBackfillEligibility{From: algorithm, To: target, Reason: reason, Eligible: ok}
	if !ok {
		return out
	}
	if def == nil {
		return AlgorithmBackfillEligibility{
			Eligible: false, Reason: "definition_required", From: algorithm,
		}
	}
	out.Eligible = true
	out.Reason = ""
	return out
}
