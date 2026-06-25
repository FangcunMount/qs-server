package sbti

import (
	evaluationdomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
)

func sbtiReportDetail(detail evaluationdomain.SBTIResultDetail) domainReport.SBTIReportDetail {
	dimensions := make([]domainReport.SBTIDimensionReport, 0, len(detail.Dimensions))
	for _, dim := range detail.Dimensions {
		dimensions = append(dimensions, domainReport.SBTIDimensionReport{
			Code:     dim.Code,
			Name:     dim.Name,
			Model:    dim.Model,
			RawScore: dim.RawScore,
			Level:    dim.Level,
		})
	}
	return domainReport.SBTIReportDetail{
		TypeCode:       detail.TypeCode,
		TypeName:       detail.TypeName,
		OneLiner:       detail.OneLiner,
		Pattern:        detail.Pattern,
		Similarity:     detail.Similarity,
		ImageURL:       detail.ImageURL,
		Rarity:         detail.Rarity,
		Dimensions:     dimensions,
		Outcome:        detail.Outcome,
		Source:         detail.Source,
		SpecialTrigger: detail.SpecialTrigger,
	}
}
