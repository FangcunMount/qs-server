package factor

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/capability"
)

// ValidateScoringStrategyCapability checks Scoring.Strategy against the
// Calculation capability catalog for an execution path.
// Empty strategy is skipped (draft may omit it; publish completeness is separate).
func ValidateScoringStrategyCapability(path capability.Path, scoring []Scoring) []HierarchyIssue {
	if path == "" || len(scoring) == 0 {
		return nil
	}
	issues := make([]HierarchyIssue, 0)
	for _, rule := range scoring {
		if rule.Strategy == "" {
			continue
		}
		usage, ok := scoringUsage(path, rule)
		if !ok {
			continue
		}
		if capability.Supports(path, usage, string(rule.Strategy)) {
			continue
		}
		supported := capability.SupportedCodes(path, usage)
		issues = append(issues, HierarchyIssue{
			Field:   fmt.Sprintf("scoring[%s].strategy", rule.FactorCode),
			Code:    "strategy.unsupported_for_path",
			Message: fmt.Sprintf("strategy %q is not supported for %s/%s (supported: %v)", rule.Strategy, path, usage, supported),
		})
	}
	return issues
}

func scoringUsage(path capability.Path, rule Scoring) (capability.Usage, bool) {
	hasQuestion := scoringHasSourceKind(rule, ScoringSourceQuestion)
	hasFactor := scoringHasSourceKind(rule, ScoringSourceFactor)
	switch path {
	case capability.PathTypologyDescriptor:
		switch {
		case hasQuestion && !hasFactor:
			return capability.UsageTypologyLeaf, true
		case hasFactor && !hasQuestion:
			return capability.UsageTypologyComposite, true
		default:
			return "", false
		}
	default:
		// scale / behavioral_rating / cognitive share question vs composite usages
		switch {
		case hasQuestion && !hasFactor:
			return capability.UsageQuestionAggregation, true
		case hasFactor && !hasQuestion:
			return capability.UsageCompositeProjection, true
		default:
			return "", false
		}
	}
}
