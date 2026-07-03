package evaluation

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/scale"
)

// ReportDimensionFilter 按量表可见因子过滤报告维度。
type ReportDimensionFilter struct {
	scaleCatalog scale.CatalogReader
}

func NewReportDimensionFilter(scaleCatalog scale.CatalogReader) *ReportDimensionFilter {
	return &ReportDimensionFilter{scaleCatalog: scaleCatalog}
}

// Apply 返回只包含可见因子的报告副本；report 为 nil 时返回 nil。
func (f *ReportDimensionFilter) Apply(ctx context.Context, report *AssessmentReportResponse) (*AssessmentReportResponse, error) {
	if report == nil {
		return nil, nil
	}
	visible := f.visibleFactorCodes(ctx, report.ScaleCode)
	return &AssessmentReportResponse{
		AssessmentID: report.AssessmentID,
		ScaleCode:    report.ScaleCode,
		ScaleName:    report.ScaleName,
		TotalScore:   report.TotalScore,
		RiskLevel:    report.RiskLevel,
		Conclusion:   report.Conclusion,
		Dimensions:   filterVisibleDimensions(report.Dimensions, visible),
		Suggestions:  report.Suggestions,
		CreatedAt:    report.CreatedAt,
	}, nil
}

func (f *ReportDimensionFilter) visibleFactorCodes(ctx context.Context, scaleCode string) map[string]bool {
	if f == nil || f.scaleCatalog == nil || scaleCode == "" {
		return nil
	}
	scaleDetail, err := f.scaleCatalog.GetScale(ctx, scaleCode)
	if err != nil || scaleDetail == nil {
		return nil
	}
	visible := make(map[string]bool, len(scaleDetail.Factors))
	for _, factor := range scaleDetail.Factors {
		visible[factor.Code] = true
	}
	return visible
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
