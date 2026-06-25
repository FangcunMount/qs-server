package sbti

import (
	evaluationsbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/sbti"
	reportsbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/report/sbti"
)

func sbtiReportDetail(detail evaluationsbti.ResultDetail) reportsbti.ReportDetail {
	dimensions := make([]reportsbti.DimensionReport, 0, len(detail.Dimensions))
	for _, dim := range detail.Dimensions {
		dimensions = append(dimensions, reportsbti.DimensionReport{
			Code:     dim.Code,
			Name:     dim.Name,
			Model:    dim.Model,
			RawScore: dim.RawScore,
			Level:    dim.Level,
		})
	}
	return reportsbti.ReportDetail{
		TypeCode:   detail.TypeCode,
		TypeName:   detail.TypeName,
		OneLiner:   detail.OneLiner,
		Pattern:    detail.Pattern,
		Similarity: detail.Similarity,
		ImageURL:   detail.ImageURL,
		Rarity: reportsbti.RarityReport{
			Percent: detail.Rarity.Percent,
			Label:   detail.Rarity.Label,
			OneInX:  detail.Rarity.OneInX,
		},
		Dimensions: dimensions,
		Outcome: reportsbti.OutcomeReport{
			Code:     detail.Outcome.Code,
			Name:     detail.Outcome.Name,
			OneLiner: detail.Outcome.OneLiner,
			Pattern:  detail.Outcome.Pattern,
			Image:    detail.Outcome.Image,
			Rarity: reportsbti.RarityReport{
				Percent: detail.Outcome.Rarity.Percent,
				Label:   detail.Outcome.Rarity.Label,
				OneInX:  detail.Outcome.Rarity.OneInX,
			},
			IsSpecial:  detail.Outcome.IsSpecial,
			Trigger:    detail.Outcome.Trigger,
			Commentary: detail.Outcome.Commentary,
		},
		Source: reportsbti.SourceReport{
			WikiRepo:      detail.Source.WikiRepo,
			SourceSite:    detail.Source.SourceSite,
			License:       detail.Source.License,
			Attribution:   detail.Source.Attribution,
			ImageBaseURL:  detail.Source.ImageBaseURL,
			NonCommercial: detail.Source.NonCommercial,
		},
		SpecialTrigger: detail.SpecialTrigger,
	}
}
