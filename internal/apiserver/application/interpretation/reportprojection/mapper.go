package reportprojection

import (
	"context"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/presentation"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/interpretationreadmodel"
)

// Mapper projects read-model rows into audience-aware report DTOs.
type Mapper struct {
	Legacy domainreport.LegacyDimensionVisibilityResolver
}

func (m Mapper) FromRow(ctx context.Context, row interpretationreadmodel.ReportRow, audience policy.Audience) (*Report, error) {
	model := modelIdentityFromRow(row)
	profile, configured, err := domainreport.ResolvePresentationProfile(ctx, model, presentationProfileFromRow(&row), m.Legacy)
	if err != nil {
		return nil, err
	}
	dimensions := row.Dimensions
	if configured {
		dimensions = filterDimensionRows(row.Dimensions, profile.VisibleSet())
	}
	return fromProjectedRow(row, dimensions, audience, profileSource(configured, profile))
}

func profileSource(configured bool, profile domainreport.PresentationProfile) string {
	if !configured {
		return ""
	}
	return string(profile.Source)
}

// FromRow keeps the historical call shape for tests that do not need legacy fallback.
func FromRow(row interpretationreadmodel.ReportRow, audience policy.Audience) (*Report, error) {
	return Mapper{}.FromRow(context.Background(), row, audience)
}

func fromProjectedRow(row interpretationreadmodel.ReportRow, dimensions []interpretationreadmodel.ReportDimensionRow, audience policy.Audience, presentationSource string) (*Report, error) {
	projected := make([]Dimension, 0, len(dimensions))
	for _, dimension := range dimensions {
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
		projected = append(projected, item)
	}
	suggestions := make([]Suggestion, 0, len(row.Suggestions))
	for _, suggestion := range row.Suggestions {
		suggestions = append(suggestions, Suggestion{Category: suggestion.Category, Content: suggestion.Content, FactorCode: suggestion.FactorCode})
	}
	result := &Report{
		AssessmentID: row.AssessmentID, Model: modelIdentity(row), PrimaryScore: primaryScore(row), Level: resultLevel(row),
		Conclusion: row.Conclusion, Dimensions: projected, Suggestions: suggestions,
		ModelExtra: modelExtra(row.ModelExtra), CreatedAt: row.CreatedAt,
		PresentationSource: presentationSource,
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
		Kind: row.Model.Kind, Algorithm: row.Model.Algorithm,
		Code: row.Model.Code, Version: row.Model.Version, Title: row.Model.Title,
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
