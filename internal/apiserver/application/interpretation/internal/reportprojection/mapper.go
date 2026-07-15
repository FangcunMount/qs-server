package reportprojection

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/presentation"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
)

func FromRow(row interpretationreadmodel.ReportRow, audience policy.Audience) (*Report, error) {
	dimensions := make([]Dimension, 0, len(row.Dimensions))
	for _, dimension := range row.Dimensions {
		item := Dimension{
			FactorCode: dimension.FactorCode, FactorName: dimension.FactorName,
			RawScore: dimension.RawScore, MaxScore: dimension.MaxScore, RiskLevel: dimension.RiskLevel,
			Role: dimension.Role, ParentCode: dimension.ParentCode, HierarchyLevel: dimension.HierarchyLevel,
			SortOrder: dimension.SortOrder, Description: dimension.Description, Suggestion: dimension.Suggestion,
		}
		for _, score := range dimension.DerivedScores {
			item.DerivedScores = append(item.DerivedScores, ScoreValue{Kind: score.Kind, Value: score.Value, Label: score.Label, Max: score.Max})
		}
		if dimension.Level != nil {
			item.Level = &ResultLevel{Code: dimension.Level.Code, Label: dimension.Level.Label, Severity: dimension.Level.Severity}
		}
		if dimension.NormReference != nil {
			item.NormReference = &NormReference{ScoreKind: dimension.NormReference.ScoreKind, Benchmark: dimension.NormReference.Benchmark, TableVersion: dimension.NormReference.TableVersion, FormVariant: dimension.NormReference.FormVariant, MinAgeMonths: dimension.NormReference.MinAgeMonths, MaxAgeMonths: dimension.NormReference.MaxAgeMonths, Gender: dimension.NormReference.Gender}
		}
		dimensions = append(dimensions, item)
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
	return ModelIdentity{
		Kind: row.Model.Kind, SubKind: row.Model.SubKind, Algorithm: row.Model.Algorithm,
		Code: row.Model.Code, Version: row.Model.Version, Title: row.Model.Title,
		ProductChannel: row.Model.ProductChannel, AlgorithmFamily: row.Model.AlgorithmFamily,
	}
}

func primaryScore(row interpretationreadmodel.ReportRow) *ScoreValue {
	if row.PrimaryScore != nil {
		return &ScoreValue{Kind: row.PrimaryScore.Kind, Value: row.PrimaryScore.Value, Label: row.PrimaryScore.Label, Max: row.PrimaryScore.Max}
	}
	return nil
}

func resultLevel(row interpretationreadmodel.ReportRow) *ResultLevel {
	if row.Level != nil {
		return &ResultLevel{Code: row.Level.Code, Label: row.Level.Label, Severity: row.Level.Severity}
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
