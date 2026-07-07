package projection

import (
	"sort"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation"
)

// HierarchyProjection 标注维度结果 使用 计分节点 层级 元数据。
type HierarchyProjection struct {
	Nodes []calculation.ScoreNode
}

func (p HierarchyProjection) Apply(result *calculation.Result) *calculation.Result {
	if result == nil || len(p.Nodes) == 0 {
		return result
	}
	byCode := nodesByCode(p.Nodes)
	for i := range result.Dimensions {
		meta, ok := byCode[result.Dimensions[i].Code]
		if !ok {
			continue
		}
		applyNodeMetadata(&result.Dimensions[i], meta)
	}
	sortDimensionsByHierarchy(result.Dimensions)
	return result
}

func nodesByCode(nodes []calculation.ScoreNode) map[string]calculation.ScoreNode {
	byCode := make(map[string]calculation.ScoreNode, len(nodes))
	for _, node := range nodes {
		byCode[node.Code] = node
	}
	return byCode
}

func applyNodeMetadata(dim *calculation.DimensionResult, node calculation.ScoreNode) {
	if dim == nil {
		return
	}
	dim.Role = node.Role
	dim.ParentCode = node.ParentCode
	dim.HierarchyLevel = node.Level
	dim.SortOrder = node.SortOrder
	if dim.Kind == "" {
		dim.Kind = node.Kind
		if dim.Kind == "" {
			dim.Kind = calculation.DimensionKindFactor
		}
	}
	if dim.Name == "" {
		dim.Name = node.Name
	}
}

func sortDimensionsByHierarchy(dimensions []calculation.DimensionResult) {
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
