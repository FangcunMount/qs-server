package modelcatalog

import (
	"strconv"
	"time"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// ModelSummaryFromAssessmentModel projects aggregate metadata shared by
// management, query and publication use cases.
func ModelSummaryFromAssessmentModel(model *domain.AssessmentModel) *ModelSummary {
	if model == nil {
		return nil
	}
	result := &ModelSummary{
		Code:                 model.Code,
		Kind:                 DomainKindToAPIKind(model.Kind),
		Algorithm:            string(model.Algorithm),
		Title:                model.Title,
		Description:          model.Description,
		Status:               string(model.Status),
		Category:             model.Category,
		Stages:               append([]string(nil), model.Stages...),
		ApplicableAges:       append([]string(nil), model.ApplicableAges...),
		Reporters:            append([]string(nil), model.Reporters...),
		Tags:                 append([]string(nil), model.Tags...),
		QuestionnaireCode:    model.Binding.QuestionnaireCode,
		QuestionnaireVersion: model.Binding.QuestionnaireVersion,
		CreatedAt:            model.CreatedAt.Format(time.RFC3339),
		UpdatedAt:            model.UpdatedAt.Format(time.RFC3339),
		ReleaseState: ReleaseState{
			WorkingStatus:  string(model.Status),
			WorkingVersion: "v" + strconv.FormatInt(model.Revision(), 10),
			OnlineStatus: func() string {
				if model.IsArchived() {
					return "archived"
				}
				return "offline"
			}(),
		},
	}
	return result
}

// NormTableDetailFromDomain projects immutable norm reference material for
// administration reads without exposing persistence models.
func NormTableDetailFromDomain(table *domain.Norm) *NormTableDetail {
	if table == nil {
		return nil
	}
	out := &NormTableDetail{NormTableSummary: NormTableSummary{
		TableVersion: table.TableVersion, FormVariant: table.FormVariant,
		Kind: string(table.Kind), Algorithm: string(table.Algorithm), FactorCount: len(table.Factors),
	}, Factors: make([]NormFactorTable, 0, len(table.Factors))}
	for _, factor := range table.Factors {
		item := NormFactorTable{FactorCode: factor.FactorCode}
		for _, band := range factor.Bands {
			item.Bands = append(item.Bands, NormBand{MinAgeMonths: band.MinAgeMonths, MaxAgeMonths: band.MaxAgeMonths, Gender: band.Gender, Mean: cloneNormFloat(band.Mean), StdDev: cloneNormFloat(band.StdDev)})
		}
		for _, lookup := range factor.Lookup {
			item.Lookup = append(item.Lookup, NormLookupEntry{RawScoreMin: lookup.RawScoreMin, RawScoreMax: lookup.RawScoreMax, MinAgeMonths: lookup.MinAgeMonths, MaxAgeMonths: lookup.MaxAgeMonths, Gender: lookup.Gender, TScore: lookup.TScore, Percentile: lookup.Percentile, StandardScore: cloneNormFloat(lookup.StandardScore)})
		}
		out.Factors = append(out.Factors, item)
	}
	return out
}

func cloneNormFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
