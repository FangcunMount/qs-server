package mbti

import (
	evaluationmbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/mbti"
	reportmbti "github.com/FangcunMount/qs-server/internal/apiserver/domain/report/mbti"
)

func mbtiReportDetail(detail evaluationmbti.ResultDetail) reportmbti.ReportDetail {
	dimensions := make([]reportmbti.DimensionReport, 0, len(detail.Dimensions))
	for _, dim := range detail.Dimensions {
		dimensions = append(dimensions, reportmbti.DimensionReport{
			Code:       dim.Code,
			Name:       dim.Name,
			LeftPole:   dim.LeftPole,
			RightPole:  dim.RightPole,
			RawScore:   dim.RawScore,
			Preference: dim.Preference,
			Strength:   dim.Strength,
		})
	}
	return reportmbti.ReportDetail{
		TypeCode:     detail.TypeCode,
		TypeName:     detail.TypeName,
		OneLiner:     detail.OneLiner,
		MatchPercent: detail.MatchPercent,
		ImageURL:     detail.ImageURL,
		Dimensions:   dimensions,
		Profile: reportmbti.ProfileReport{
			TypeCode:    detail.Profile.TypeCode,
			TypeName:    detail.Profile.TypeName,
			OneLiner:    detail.Profile.OneLiner,
			Summary:     detail.Profile.Summary,
			Traits:      append([]string(nil), detail.Profile.Traits...),
			Strengths:   append([]string(nil), detail.Profile.Strengths...),
			Weaknesses:  append([]string(nil), detail.Profile.Weaknesses...),
			Suggestions: append([]string(nil), detail.Profile.Suggestions...),
			ImageURL:    detail.Profile.ImageURL,
		},
		Source: reportmbti.SourceReport{
			QuestionsRepo: detail.Source.QuestionsRepo,
			SourceSite:    detail.Source.SourceSite,
			License:       detail.Source.License,
			Attribution:   detail.Source.Attribution,
			NonCommercial: detail.Source.NonCommercial,
		},
	}
}
