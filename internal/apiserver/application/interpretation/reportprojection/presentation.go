package reportprojection

import (
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
)

func presentationProfileFromRow(row *interpretationreadmodel.ReportRow) *domainreport.PresentationProfile {
	if row == nil || row.PresentationProfile == nil || row.PresentationProfile.Source == "" {
		return nil
	}
	return &domainreport.PresentationProfile{
		VisibleFactorCodes: append([]string(nil), row.PresentationProfile.VisibleFactorCodes...),
		Source:             domainreport.PresentationProfileSource(row.PresentationProfile.Source),
	}
}

func modelIdentityFromRow(row interpretationreadmodel.ReportRow) domainreport.ModelIdentity {
	return domainreport.ModelIdentity{
		Kind: row.Model.Kind, Algorithm: row.Model.Algorithm,
		Code: row.Model.Code, Version: row.Model.Version, Title: row.Model.Title,
	}
}

func filterDimensionRows(rows []interpretationreadmodel.ReportDimensionRow, visible map[string]bool) []interpretationreadmodel.ReportDimensionRow {
	if len(rows) == 0 {
		return nil
	}
	filtered := make([]interpretationreadmodel.ReportDimensionRow, 0, len(rows))
	for _, row := range rows {
		if visible[row.FactorCode] {
			filtered = append(filtered, row)
		}
	}
	return filtered
}
