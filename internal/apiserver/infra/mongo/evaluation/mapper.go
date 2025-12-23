package evaluation

import (
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/report"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

// ==================== Report Mapper ====================

// ReportMapper 报告映射器
type ReportMapper struct{}

// NewReportMapper 创建报告映射器
func NewReportMapper() *ReportMapper {
	return &ReportMapper{}
}

// ToPO 将领域对象转换为持久化对象
func (m *ReportMapper) ToPO(domain *report.InterpretReport, testeeID uint64) *InterpretReportPO {
	if domain == nil {
		return nil
	}

	// 转换维度列表
	dimensions := make([]DimensionInterpretPO, len(domain.Dimensions()))
	for i, d := range domain.Dimensions() {
		dimensions[i] = DimensionInterpretPO{
			FactorCode:  d.FactorCode().String(),
			FactorName:  d.FactorName(),
			RawScore:    d.RawScore(),
			MaxScore:    d.MaxScore(),
			RiskLevel:   string(d.RiskLevel()),
			Description: d.Description(),
			Suggestion:  d.Suggestion(),
		}
	}

	po := &InterpretReportPO{
		ScaleName:   domain.ScaleName(),
		ScaleCode:   domain.ScaleCode(),
		TesteeID:    testeeID,
		TotalScore:  domain.TotalScore(),
		RiskLevel:   string(domain.RiskLevel()),
		Conclusion:  domain.Conclusion(),
		Dimensions:  dimensions,
		Suggestions: toSuggestionPOs(domain.Suggestions()),
	}

	// 设置 DomainID（与 AssessmentID 一致）
	if !domain.ID().IsZero() {
		po.DomainID = meta.ID(domain.ID())
	}

	return po
}

// ToDomain 将持久化对象转换为领域对象
func (m *ReportMapper) ToDomain(po *InterpretReportPO) *report.InterpretReport {
	if po == nil {
		return nil
	}

	// 转换维度列表
	dimensions := make([]report.DimensionInterpret, len(po.Dimensions))
	for i, d := range po.Dimensions {
		dimensions[i] = report.NewDimensionInterpret(
			report.NewFactorCode(d.FactorCode),
			d.FactorName,
			d.RawScore,
			d.MaxScore,
			report.RiskLevel(d.RiskLevel),
			d.Description,
			d.Suggestion,
		)
	}

	// 处理更新时间
	var updatedAt *time.Time
	if !po.UpdatedAt.IsZero() && po.UpdatedAt != po.CreatedAt {
		updatedAt = &po.UpdatedAt
	}

	return report.ReconstructInterpretReport(
		report.ID(po.DomainID),
		po.ScaleName,
		po.ScaleCode,
		po.TotalScore,
		report.RiskLevel(po.RiskLevel),
		po.Conclusion,
		dimensions,
		toDomainSuggestions(po.Suggestions),
		po.CreatedAt,
		updatedAt,
	)
}

func toSuggestionPOs(items []report.Suggestion) []SuggestionPO {
	if len(items) == 0 {
		return nil
	}
	result := make([]SuggestionPO, len(items))
	for i, s := range items {
		var fc *string
		if s.FactorCode != nil {
			code := s.FactorCode.String()
			fc = &code
		}
		result[i] = SuggestionPO{
			Category:   string(s.Category),
			Content:    s.Content,
			FactorCode: fc,
		}
	}
	return result
}

func toDomainSuggestions(items []SuggestionPO) []report.Suggestion {
	if len(items) == 0 {
		return nil
	}
	result := make([]report.Suggestion, len(items))
	for i, s := range items {
		var fc *report.FactorCode
		if s.FactorCode != nil {
			code := report.NewFactorCode(*s.FactorCode)
			fc = &code
		}
		result[i] = report.Suggestion{
			Category:   report.SuggestionCategory(s.Category),
			Content:    s.Content,
			FactorCode: fc,
		}
	}
	return result
}

// ToDomainList 批量转换持久化对象为领域对象
func (m *ReportMapper) ToDomainList(pos []*InterpretReportPO) []*report.InterpretReport {
	if len(pos) == 0 {
		return nil
	}

	result := make([]*report.InterpretReport, 0, len(pos))
	for _, po := range pos {
		if domain := m.ToDomain(po); domain != nil {
			result = append(result, domain)
		}
	}
	return result
}
