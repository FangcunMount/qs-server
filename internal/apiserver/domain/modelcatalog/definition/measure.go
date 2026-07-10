package definition

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"

// ValidateMeasureSpec checks measure-layer invariants.
func ValidateMeasureSpec(measure MeasureSpec) []factor.HierarchyIssue {
	return factor.ValidateMeasureSpecParts(measure.Factors, measure.FactorGraph, measure.Scoring)
}
