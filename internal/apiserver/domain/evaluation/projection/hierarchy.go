package projection

import (
	"sort"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

// HierarchyProjection annotates dimension results with factor hierarchy metadata.
type HierarchyProjection struct {
	Factors []factor.FactorSnapshot
}

func (p HierarchyProjection) Apply(outcome *assessment.AssessmentOutcome) *assessment.AssessmentOutcome {
	if outcome == nil || len(p.Factors) == 0 {
		return outcome
	}
	byCode := factorSnapshotsByCode(factor.InferParentCodesFromChildrenPolicy(p.Factors))
	for i := range outcome.Dimensions {
		meta, ok := byCode[outcome.Dimensions[i].Code]
		if !ok {
			continue
		}
		applyFactorMetadata(&outcome.Dimensions[i], meta)
	}
	sortDimensionsForDisplay(outcome.Dimensions)
	return outcome
}

func factorSnapshotsByCode(factors []factor.FactorSnapshot) map[string]factor.FactorSnapshot {
	byCode := make(map[string]factor.FactorSnapshot, len(factors))
	for _, item := range factors {
		byCode[item.Code] = item
	}
	return byCode
}

func applyFactorMetadata(dim *assessment.DimensionResult, meta factor.FactorSnapshot) {
	if dim == nil {
		return
	}
	role := meta.ResolvedRole()
	dim.Role = string(role)
	dim.ParentCode = meta.ParentCode
	dim.HierarchyLevel = meta.Level
	dim.SortOrder = meta.SortOrder
	if dim.Kind == "" {
		dim.Kind = dimensionKindForRole(role)
	}
	if dim.Name == "" {
		dim.Name = meta.Title
	}
}

func sortDimensionsForDisplay(dimensions []assessment.DimensionResult) {
	sort.SliceStable(dimensions, func(i, j int) bool {
		left, right := dimensions[i], dimensions[j]
		if left.HierarchyLevel != right.HierarchyLevel {
			return left.HierarchyLevel < right.HierarchyLevel
		}
		if left.ParentCode != right.ParentCode {
			return left.ParentCode < right.ParentCode
		}
		if left.SortOrder != right.SortOrder {
			return left.SortOrder < right.SortOrder
		}
		return left.Code < right.Code
	})
}
