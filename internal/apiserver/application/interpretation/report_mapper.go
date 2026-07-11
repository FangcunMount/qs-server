package interpretation

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationreadmodel"
)

func reportRowToResult(row evaluationreadmodel.ReportRow) *ReportResult {
	dimensions := make([]DimensionResult, 0, len(row.Dimensions))
	for _, dimension := range row.Dimensions {
		dimensions = append(dimensions, DimensionResult{
			FactorCode: dimension.FactorCode, FactorName: dimension.FactorName,
			RawScore: dimension.RawScore, MaxScore: dimension.MaxScore, RiskLevel: dimension.RiskLevel,
			Role: dimension.Role, ParentCode: dimension.ParentCode, HierarchyLevel: dimension.HierarchyLevel,
			SortOrder: dimension.SortOrder, Description: dimension.Description, Suggestion: dimension.Suggestion,
		})
	}
	suggestions := make([]SuggestionDTO, 0, len(row.Suggestions))
	for _, suggestion := range row.Suggestions {
		suggestions = append(suggestions, SuggestionDTO{Category: suggestion.Category, Content: suggestion.Content, FactorCode: suggestion.FactorCode})
	}
	return &ReportResult{
		AssessmentID: row.AssessmentID, ModelName: row.ModelName, ModelCode: row.ModelCode,
		TotalScore: row.TotalScore, RiskLevel: row.RiskLevel, Conclusion: row.Conclusion,
		Dimensions: dimensions, Suggestions: suggestions, ModelExtra: reportModelExtraRowToResult(row.ModelExtra), CreatedAt: row.CreatedAt,
	}
}

func reportRowToOutcomeResult(row evaluationreadmodel.ReportRow) *ReportOutcomeResult {
	base := reportRowToResult(row)
	if base == nil {
		return nil
	}
	return &ReportOutcomeResult{
		AssessmentID: base.AssessmentID, Model: modelIdentityFromReportRow(row),
		PrimaryScore: primaryScoreFromReportRow(row), Level: levelFromReportRow(row),
		Conclusion: base.Conclusion, Dimensions: base.Dimensions, Suggestions: base.Suggestions,
		ModelExtra: base.ModelExtra, CreatedAt: base.CreatedAt,
	}
}

func reportModelExtraRowToResult(row *evaluationreadmodel.ReportModelExtraRow) *ModelExtraResult {
	if row == nil {
		return nil
	}
	result := &ModelExtraResult{
		Kind: row.Kind, TypeCode: row.TypeCode, TypeName: row.TypeName, OneLiner: row.OneLiner,
		ImageURL: row.ImageURL, MatchPercent: row.MatchPercent, IsSpecial: row.IsSpecial,
		SpecialTrigger: row.SpecialTrigger, Commentary: row.Commentary,
	}
	if row.Rarity != nil {
		result.Rarity = &ModelRarityResult{Percent: row.Rarity.Percent, Label: row.Rarity.Label, OneInX: row.Rarity.OneInX}
	}
	return result
}

func modelIdentityFromReportRow(row evaluationreadmodel.ReportRow) ModelIdentityResult {
	if row.Model.Kind != "" || row.Model.Code != "" {
		return enrichModelIdentity(ModelIdentityResult{
			Kind: row.Model.Kind, SubKind: row.Model.SubKind, Algorithm: row.Model.Algorithm,
			Code: row.Model.Code, Version: row.Model.Version, Title: row.Model.Title,
			ProductChannel: row.Model.ProductChannel, AlgorithmFamily: row.Model.AlgorithmFamily,
		}, row.Model.ProductChannel)
	}
	return enrichModelIdentity(ModelIdentityResult{Kind: string(modelcatalog.KindScale), Code: row.ModelCode, Title: row.ModelName}, "")
}

func enrichModelIdentity(model ModelIdentityResult, explicitProductChannel string) ModelIdentityResult {
	kind := binding.Kind(model.Kind)
	productChannel := explicitProductChannel
	if productChannel == "" {
		productChannel = model.ProductChannel
	}
	model.ProductChannel = binding.ProductChannelForIdentity(kind, productChannel)
	model.AlgorithmFamily = binding.AlgorithmFamilyStringFromIdentity(kind, binding.SubKind(model.SubKind), binding.Algorithm(model.Algorithm))
	return model
}

func primaryScoreFromReportRow(row evaluationreadmodel.ReportRow) *ScoreValueResult {
	if row.PrimaryScore != nil {
		return &ScoreValueResult{Kind: row.PrimaryScore.Kind, Value: row.PrimaryScore.Value, Label: row.PrimaryScore.Label, Max: row.PrimaryScore.Max}
	}
	if row.TotalScore != 0 || row.RiskLevel != "" {
		return &ScoreValueResult{Kind: "raw_total", Value: row.TotalScore}
	}
	return nil
}

func levelFromReportRow(row evaluationreadmodel.ReportRow) *ResultLevelResult {
	if row.Level != nil {
		return &ResultLevelResult{Code: row.Level.Code, Label: row.Level.Label, Severity: row.Level.Severity}
	}
	if row.RiskLevel != "" {
		if level := reportRiskLevelResult(row.RiskLevel); level != nil {
			return level
		}
	}
	if row.ModelExtra != nil && row.ModelExtra.TypeCode != "" {
		return &ResultLevelResult{Code: row.ModelExtra.TypeCode, Label: row.ModelExtra.TypeCode, Severity: "none"}
	}
	return nil
}

func reportRiskLevelResult(code string) *ResultLevelResult {
	var severity string
	switch code {
	case "severe", "high":
		severity = "high"
	case "medium":
		severity = "medium"
	case "low":
		severity = "low"
	case "none":
		severity = "none"
	default:
		return nil
	}
	return &ResultLevelResult{Code: code, Label: code, Severity: severity}
}
