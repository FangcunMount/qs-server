package interpretation

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	evaluationreadmodel "github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
)

func projectArchivedReportRow(po *ArchivedReportPO) evaluationreadmodel.ReportRow {
	if po == nil {
		return evaluationreadmodel.ReportRow{}
	}
	dimensions := make([]evaluationreadmodel.ReportDimensionRow, 0, len(po.Dimensions))
	for _, d := range po.Dimensions {
		dimension := evaluationreadmodel.ReportDimensionRow{
			FactorCode:     d.FactorCode,
			FactorName:     d.FactorName,
			RawScore:       d.RawScore,
			MaxScore:       d.MaxScore,
			RiskLevel:      d.RiskLevel,
			Role:           d.Role,
			ParentCode:     d.ParentCode,
			HierarchyLevel: d.HierarchyLevel,
			SortOrder:      d.SortOrder,
			Description:    d.Description,
			Suggestion:     d.Suggestion,
		}
		for _, score := range d.DerivedScores {
			dimension.DerivedScores = append(dimension.DerivedScores, evaluationreadmodel.ScoreValueRow{Kind: score.Kind, Value: score.Value, Label: score.Label, Max: score.Max})
		}
		if d.Level != nil {
			dimension.Level = &evaluationreadmodel.ResultLevelRow{Code: d.Level.Code, Label: d.Level.Label, Severity: d.Level.Severity}
		}
		if d.NormReference != nil {
			dimension.NormReference = &evaluationreadmodel.NormReferenceRow{ScoreKind: d.NormReference.ScoreKind, Benchmark: d.NormReference.Benchmark, TableVersion: d.NormReference.TableVersion, FormVariant: d.NormReference.FormVariant, MinAgeMonths: d.NormReference.MinAgeMonths, MaxAgeMonths: d.NormReference.MaxAgeMonths, Gender: d.NormReference.Gender}
		}
		dimensions = append(dimensions, dimension)
	}
	suggestions := make([]evaluationreadmodel.ReportSuggestionRow, 0, len(po.Suggestions))
	for _, s := range po.Suggestions {
		suggestions = append(suggestions, evaluationreadmodel.ReportSuggestionRow{
			Category:   s.Category,
			Content:    s.Content,
			FactorCode: s.FactorCode,
		})
	}
	modelName := po.ScaleName
	modelCode := po.ScaleCode
	if po.Model != nil {
		if po.Model.Title != "" {
			modelName = po.Model.Title
		}
		if po.Model.Code != "" {
			modelCode = po.Model.Code
		}
	}
	row := evaluationreadmodel.ReportRow{
		AssessmentID:        po.DomainID.Uint64(),
		ModelName:           modelName,
		ModelCode:           modelCode,
		TotalScore:          po.TotalScore,
		RiskLevel:           po.RiskLevel,
		Conclusion:          po.Conclusion,
		Dimensions:          dimensions,
		Suggestions:         suggestions,
		ModelExtra:          reportModelExtraPOToRow(po.ModelExtra),
		PresentationProfile: presentationProfilePOToRow(po.PresentationProfile),
		CreatedAt:           po.CreatedAt,
	}
	if po.Model != nil {
		row.Model = evaluationreadmodel.ModelIdentityRow{
			Kind:         po.Model.Kind,
			Algorithm:    po.Model.Algorithm,
			Code:         po.Model.Code,
			Version:      po.Model.Version,
			Title:        po.Model.Title,
			DecisionKind: po.Model.DecisionKind,
		}
	}
	if po.PrimaryScore != nil {
		row.PrimaryScore = &evaluationreadmodel.ScoreValueRow{
			Kind:  po.PrimaryScore.Kind,
			Value: po.PrimaryScore.Value,
			Label: po.PrimaryScore.Label,
			Max:   po.PrimaryScore.Max,
		}
	}
	if po.Level != nil {
		row.Level = &evaluationreadmodel.ResultLevelRow{
			Code:     po.Level.Code,
			Label:    po.Level.Label,
			Severity: po.Level.Severity,
		}
	}
	normalizeArchivedReportRow(&row)
	return row
}

func normalizeArchivedReportRow(row *evaluationreadmodel.ReportRow) {
	if row == nil {
		return
	}
	if row.Model.Kind == "" && row.Model.Code == "" {
		row.Model.Kind = string(modelcatalog.KindScale)
		row.Model.Code = row.ModelCode
		row.Model.Title = row.ModelName
	}
	if row.Model.Algorithm == "" {
		row.Model.StaticOnly = true
	} else if runtime, err := modelcatalog.ResolveLegacyRuntime(modelcatalog.Kind(row.Model.Kind), modelcatalog.Algorithm(row.Model.Algorithm), modelcatalog.DecisionKind(row.Model.DecisionKind)); err == nil {
		row.Model.DecisionKind = string(runtime.DecisionKind)
	} else {
		row.Model.StaticOnly = true
	}

	if row.PrimaryScore == nil && (row.TotalScore != 0 || row.RiskLevel != "") {
		row.PrimaryScore = &evaluationreadmodel.ScoreValueRow{Kind: "raw_total", Value: row.TotalScore}
	}
	if row.Level != nil {
		return
	}
	if severity := legacyRiskSeverity(row.RiskLevel); severity != "" {
		row.Level = &evaluationreadmodel.ResultLevelRow{Code: row.RiskLevel, Label: row.RiskLevel, Severity: severity}
		return
	}
	if row.ModelExtra != nil && row.ModelExtra.TypeCode != "" {
		row.Level = &evaluationreadmodel.ResultLevelRow{Code: row.ModelExtra.TypeCode, Label: row.ModelExtra.TypeCode, Severity: "none"}
	}
}

func presentationProfilePOToRow(po *PresentationProfilePO) *evaluationreadmodel.PresentationProfileRow {
	if po == nil || po.Source == "" {
		return nil
	}
	return &evaluationreadmodel.PresentationProfileRow{
		VisibleFactorCodes: append([]string(nil), po.VisibleFactorCodes...),
		Source:             po.Source,
	}
}

func legacyRiskSeverity(risk string) string {
	switch risk {
	case "severe", "high":
		return "high"
	case "medium":
		return "medium"
	case "low":
		return "low"
	case "none":
		return "none"
	default:
		return ""
	}
}

func reportModelExtraPOToRow(po *ModelExtraPO) *evaluationreadmodel.ReportModelExtraRow {
	if po == nil {
		return nil
	}
	row := &evaluationreadmodel.ReportModelExtraRow{
		Kind:           po.Kind,
		TypeCode:       po.TypeCode,
		TypeName:       po.TypeName,
		OneLiner:       po.OneLiner,
		ImageURL:       po.ImageURL,
		MatchPercent:   po.MatchPercent,
		IsSpecial:      po.IsSpecial,
		SpecialTrigger: po.SpecialTrigger,
		Commentary:     po.Commentary,
	}
	if po.Rarity != nil {
		row.Rarity = &evaluationreadmodel.ReportModelRarityRow{
			Percent: po.Rarity.Percent,
			Label:   po.Rarity.Label,
			OneInX:  po.Rarity.OneInX,
		}
	}
	return row
}
