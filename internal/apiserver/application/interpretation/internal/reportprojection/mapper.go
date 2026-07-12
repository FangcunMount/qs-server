package reportprojection

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/presentation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
)

func FromRow(row interpretationreadmodel.ReportRow, audience policy.Audience) (*Report, error) {
	dimensions := make([]Dimension, 0, len(row.Dimensions))
	for _, dimension := range row.Dimensions {
		dimensions = append(dimensions, Dimension{
			FactorCode: dimension.FactorCode, FactorName: dimension.FactorName,
			RawScore: dimension.RawScore, MaxScore: dimension.MaxScore, RiskLevel: dimension.RiskLevel,
			Role: dimension.Role, ParentCode: dimension.ParentCode, HierarchyLevel: dimension.HierarchyLevel,
			SortOrder: dimension.SortOrder, Description: dimension.Description, Suggestion: dimension.Suggestion,
		})
	}
	suggestions := make([]Suggestion, 0, len(row.Suggestions))
	for _, suggestion := range row.Suggestions {
		suggestions = append(suggestions, Suggestion{Category: suggestion.Category, Content: suggestion.Content, FactorCode: suggestion.FactorCode})
	}
	result := &Report{
		AssessmentID: row.AssessmentID, Model: modelIdentity(row), PrimaryScore: primaryScore(row), Level: resultLevel(row),
		Conclusion: row.Conclusion, Dimensions: dimensions, Suggestions: suggestions,
		ModelExtra: modelExtra(row.ModelExtra), CreatedAt: row.CreatedAt,
	}
	visible, err := (presentation.Presenter{}).Allows(audience, presentation.SectionModelExtra)
	if err != nil {
		return nil, err
	}
	if !visible {
		result.ModelExtra = nil
	}
	return result, nil
}

func modelIdentity(row interpretationreadmodel.ReportRow) ModelIdentity {
	model := ModelIdentity{
		Kind: row.Model.Kind, SubKind: row.Model.SubKind, Algorithm: row.Model.Algorithm,
		Code: row.Model.Code, Version: row.Model.Version, Title: row.Model.Title,
		ProductChannel: row.Model.ProductChannel, AlgorithmFamily: row.Model.AlgorithmFamily,
	}
	if model.Kind == "" && model.Code == "" {
		model.Kind, model.Code, model.Title = string(modelcatalog.KindScale), row.ModelCode, row.ModelName
	}
	kind := binding.Kind(model.Kind)
	model.ProductChannel = binding.ProductChannelForIdentity(kind, model.ProductChannel)
	model.AlgorithmFamily = binding.AlgorithmFamilyStringFromIdentity(kind, binding.SubKind(model.SubKind), binding.Algorithm(model.Algorithm))
	return model
}

func primaryScore(row interpretationreadmodel.ReportRow) *ScoreValue {
	if row.PrimaryScore != nil {
		return &ScoreValue{Kind: row.PrimaryScore.Kind, Value: row.PrimaryScore.Value, Label: row.PrimaryScore.Label, Max: row.PrimaryScore.Max}
	}
	if row.TotalScore != 0 || row.RiskLevel != "" {
		return &ScoreValue{Kind: "raw_total", Value: row.TotalScore}
	}
	return nil
}

func resultLevel(row interpretationreadmodel.ReportRow) *ResultLevel {
	if row.Level != nil {
		return &ResultLevel{Code: row.Level.Code, Label: row.Level.Label, Severity: row.Level.Severity}
	}
	if row.RiskLevel != "" {
		severity := map[string]string{"severe": "high", "high": "high", "medium": "medium", "low": "low", "none": "none"}[row.RiskLevel]
		if severity != "" {
			return &ResultLevel{Code: row.RiskLevel, Label: row.RiskLevel, Severity: severity}
		}
	}
	if row.ModelExtra != nil && row.ModelExtra.TypeCode != "" {
		return &ResultLevel{Code: row.ModelExtra.TypeCode, Label: row.ModelExtra.TypeCode, Severity: "none"}
	}
	return nil
}

func modelExtra(row *interpretationreadmodel.ReportModelExtraRow) *ModelExtra {
	if row == nil {
		return nil
	}
	result := &ModelExtra{Kind: row.Kind, TypeCode: row.TypeCode, TypeName: row.TypeName, OneLiner: row.OneLiner, ImageURL: row.ImageURL, MatchPercent: row.MatchPercent, IsSpecial: row.IsSpecial, SpecialTrigger: row.SpecialTrigger, Commentary: row.Commentary}
	if row.Rarity != nil {
		result.Rarity = &ModelRarity{Percent: row.Rarity.Percent, Label: row.Rarity.Label, OneInX: row.Rarity.OneInX}
	}
	return result
}
