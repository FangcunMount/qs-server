package request

import (
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
	modelnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
)

type ImportNormTableRequest struct {
	TableVersion string            `json:"table_version" binding:"required"`
	FormVariant  string            `json:"form_variant" binding:"required"`
	Kind         string            `json:"kind" binding:"required"`
	Algorithm    string            `json:"algorithm" binding:"required"`
	Factors      []NormFactorTable `json:"factors" binding:"required,min=1,dive"`
}

type NormFactorTable struct {
	FactorCode string            `json:"factor_code" binding:"required"`
	Bands      []NormBand        `json:"bands,omitempty" binding:"omitempty,dive"`
	Lookup     []NormLookupEntry `json:"lookup,omitempty" binding:"omitempty,dive"`
}

type NormBand struct {
	MinAgeMonths int      `json:"min_age_months,omitempty"`
	MaxAgeMonths int      `json:"max_age_months,omitempty"`
	Gender       string   `json:"gender,omitempty"`
	Mean         *float64 `json:"mean,omitempty" binding:"required"`
	StdDev       *float64 `json:"std_dev,omitempty" binding:"required"`
}

type NormLookupEntry struct {
	RawScoreMin   *float64 `json:"raw_score_min" binding:"required"`
	RawScoreMax   *float64 `json:"raw_score_max" binding:"required"`
	MinAgeMonths  int      `json:"min_age_months,omitempty"`
	MaxAgeMonths  int      `json:"max_age_months,omitempty"`
	Gender        string   `json:"gender,omitempty"`
	TScore        *float64 `json:"t_score" binding:"required"`
	Percentile    *float64 `json:"percentile" binding:"required"`
	StandardScore *float64 `json:"standard_score,omitempty"`
}

func (r ImportNormTableRequest) ToDomain() *domain.Norm {
	table := &domain.Norm{TableVersion: r.TableVersion, FormVariant: r.FormVariant, Kind: identity.Kind(r.Kind), Algorithm: identity.Algorithm(r.Algorithm), Factors: make([]modelnorm.FactorTable, 0, len(r.Factors))}
	for _, factor := range r.Factors {
		item := modelnorm.FactorTable{FactorCode: factor.FactorCode}
		for _, band := range factor.Bands {
			item.Bands = append(item.Bands, modelnorm.Band{MinAgeMonths: band.MinAgeMonths, MaxAgeMonths: band.MaxAgeMonths, Gender: band.Gender, Mean: cloneNormRequestFloat(band.Mean), StdDev: cloneNormRequestFloat(band.StdDev)})
		}
		for _, lookup := range factor.Lookup {
			item.Lookup = append(item.Lookup, modelnorm.LookupEntry{RawScoreMin: normRequestFloat(lookup.RawScoreMin), RawScoreMax: normRequestFloat(lookup.RawScoreMax), MinAgeMonths: lookup.MinAgeMonths, MaxAgeMonths: lookup.MaxAgeMonths, Gender: lookup.Gender, TScore: normRequestFloat(lookup.TScore), Percentile: normRequestFloat(lookup.Percentile), StandardScore: cloneNormRequestFloat(lookup.StandardScore)})
		}
		table.Factors = append(table.Factors, item)
	}
	return table
}

func normRequestFloat(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func cloneNormRequestFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
