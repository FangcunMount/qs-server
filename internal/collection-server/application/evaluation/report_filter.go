package evaluation

import (
	"context"
)

// FactorVisibilityResolver reads report-visible factor codes from a published
// DefinitionV2. It deliberately does not expose a scale payload or draft read.
type FactorVisibilityResolver interface {
	VisibleFactorCodes(context.Context, string) (map[string]bool, bool, error)
}

// ReportDimensionFilter 按量表可见因子过滤报告维度。
// Deprecated: IR-R017 moved dimension visibility to frozen InterpretReport
// presentation profiles projected in apiserver. Keep for legacy unit tests only.
type ReportDimensionFilter struct {
	resolver FactorVisibilityResolver
}

func NewReportDimensionFilter(resolver FactorVisibilityResolver) *ReportDimensionFilter {
	return &ReportDimensionFilter{resolver: resolver}
}

// Apply 返回只包含可见因子的报告副本；report 为 nil 时返回 nil。
// 仅量表类模型按 is_show 过滤维度；人格类模型无量表因子，维度保持不变。
func (f *ReportDimensionFilter) Apply(ctx context.Context, report *AssessmentReportResponse) (*AssessmentReportResponse, error) {
	if report == nil {
		return nil, nil
	}
	scaleCode := scaleCodeFromModel(report.Model)
	if scaleCode == "" {
		return report, nil
	}
	visible, configured := f.visibleFactorCodes(ctx, scaleCode)
	if !configured {
		return report, nil
	}
	filtered := *report
	filtered.Dimensions = filterVisibleDimensions(report.Dimensions, visible)
	return &filtered, nil
}

func (f *ReportDimensionFilter) visibleFactorCodes(ctx context.Context, scaleCode string) (map[string]bool, bool) {
	if f == nil || f.resolver == nil || scaleCode == "" {
		return nil, false
	}
	visible, configured, err := f.resolver.VisibleFactorCodes(ctx, scaleCode)
	if err != nil {
		return nil, false
	}
	return visible, configured
}

func filterVisibleDimensions(dimensions []DimensionInterpretResponse, visible map[string]bool) []DimensionInterpretResponse {
	if len(dimensions) == 0 {
		return nil
	}
	filtered := make([]DimensionInterpretResponse, 0, len(dimensions))
	for _, dim := range dimensions {
		if visible == nil || visible[dim.FactorCode] {
			filtered = append(filtered, dim)
		}
	}
	return filtered
}
