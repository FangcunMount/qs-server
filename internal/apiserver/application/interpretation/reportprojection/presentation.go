package reportprojection

import (
	"context"

	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modelcatalogport "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
)

// ModelCatalogLegacyVisibility resolves current published factor visibility for
// artifacts that predate frozen presentation profiles.
type ModelCatalogLegacyVisibility struct {
	Lister modelcatalogport.PublishedModelLister
}

func NewModelCatalogLegacyVisibility(lister modelcatalogport.PublishedModelLister) ModelCatalogLegacyVisibility {
	return ModelCatalogLegacyVisibility{Lister: lister}
}

func (a ModelCatalogLegacyVisibility) VisibleFactorCodes(ctx context.Context, model domainreport.ModelIdentity) (map[string]bool, bool, error) {
	if a.Lister == nil || model.Code == "" {
		return nil, false, nil
	}
	published, err := a.Lister.FindPublishedModelByCode(ctx, modelcatalog.Kind(model.Kind), model.Code)
	if err != nil || published == nil || published.DefinitionV2 == nil {
		return nil, false, err
	}
	codes, configured := published.DefinitionV2.ReportMap.FactorScoreSources()
	if !configured {
		return nil, false, nil
	}
	visible := make(map[string]bool, len(codes))
	for _, code := range codes {
		if code != "" {
			visible[code] = true
		}
	}
	return visible, true, nil
}

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
		Kind: row.Model.Kind, SubKind: row.Model.SubKind, Algorithm: row.Model.Algorithm,
		Code: row.Model.Code, Version: row.Model.Version, Title: row.Model.Title,
		ProductChannel: row.Model.ProductChannel, AlgorithmFamily: row.Model.AlgorithmFamily,
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
