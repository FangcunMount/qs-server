package mbti

import (
	evaluationdomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	domainReport "github.com/FangcunMount/qs-server/internal/apiserver/domain/report"
)

func mbtiReportDetail(detail evaluationdomain.MBTIResultDetail) domainReport.MBTIReportDetail {
	dimensions := make([]domainReport.MBTIDimensionReport, 0, len(detail.Dimensions))
	for _, dim := range detail.Dimensions {
		dimensions = append(dimensions, domainReport.MBTIDimensionReport{
			Code:       dim.Code,
			Name:       dim.Name,
			LeftPole:   dim.LeftPole,
			RightPole:  dim.RightPole,
			RawScore:   dim.RawScore,
			Preference: dim.Preference,
			Strength:   dim.Strength,
		})
	}
	return domainReport.MBTIReportDetail{
		TypeCode:     detail.TypeCode,
		TypeName:     detail.TypeName,
		OneLiner:     detail.OneLiner,
		MatchPercent: detail.MatchPercent,
		ImageURL:     detail.ImageURL,
		Dimensions:   dimensions,
		Profile:      detail.Profile,
		Source:       detail.Source,
	}
}
