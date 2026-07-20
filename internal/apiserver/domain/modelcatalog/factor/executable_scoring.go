package factor

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/capability"
)

// ValidateExecutableScoringCapability ensures FactorRoles that the path marks as
// requiring executable Measure Scoring actually have non-empty scoring sources.
func ValidateExecutableScoringCapability(path capability.Path, factors []Factor, scoring []Scoring) []HierarchyIssue {
	if path == "" || len(factors) == 0 {
		return nil
	}
	scoringByFactor := make(map[string]Scoring, len(scoring))
	for _, rule := range scoring {
		scoringByFactor[rule.FactorCode] = rule
	}
	issues := make([]HierarchyIssue, 0)
	for _, item := range factors {
		role := item.ResolvedRole()
		if !capability.RequiresExecutableScoring(path, string(role)) {
			continue
		}
		rule, ok := scoringByFactor[item.Code]
		if ok && len(rule.Sources) > 0 {
			continue
		}
		issues = append(issues, HierarchyIssue{
			Field:   fmt.Sprintf("factors[%s]", item.Code),
			Code:    "factor.scoring.executable_required",
			Message: fmt.Sprintf("role %s 在 %s 路径上必须配置可执行 scoring sources", role, path),
		})
	}
	return issues
}
