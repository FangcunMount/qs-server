// Package normruntime projects immutable ModelCatalog Norm assets into the
// calculation DTO consumed by evaluation runtimes.
package normruntime

import (
	"fmt"

	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	modelnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
)

// FromCatalog validates and losslessly copies one immutable catalog Norm.
// Keeping this mapper shared prevents behavioral and cognitive runtimes from
// silently diverging when the catalog contract gains a field.
func FromCatalog(table *modelnorm.Norm) (*calcnorm.NormTables, error) {
	if err := modelnorm.ValidateImport(table); err != nil {
		return nil, calcnorm.NewInvalidError("", fmt.Errorf("invalid catalog norm: %w", err))
	}
	out := &calcnorm.NormTables{
		FormVariant:      table.FormVariant,
		NormTableVersion: table.TableVersion,
		Factors:          make([]calcnorm.FactorNormTable, 0, len(table.Factors)),
	}
	for _, factor := range table.Factors {
		item := calcnorm.FactorNormTable{
			FactorCode: factor.FactorCode,
			Bands:      make([]calcnorm.NormBand, 0, len(factor.Bands)),
			Lookup:     make([]calcnorm.NormLookupEntry, 0, len(factor.Lookup)),
		}
		for _, band := range factor.Bands {
			item.Bands = append(item.Bands, calcnorm.NormBand{
				MinAgeMonths: band.MinAgeMonths,
				MaxAgeMonths: band.MaxAgeMonths,
				Gender:       band.Gender,
				Mean:         cloneFloat64(band.Mean),
				StdDev:       cloneFloat64(band.StdDev),
			})
		}
		for _, lookup := range factor.Lookup {
			item.Lookup = append(item.Lookup, calcnorm.NormLookupEntry{
				RawMin:        lookup.RawScoreMin,
				RawMax:        lookup.RawScoreMax,
				MinAgeMonths:  lookup.MinAgeMonths,
				MaxAgeMonths:  lookup.MaxAgeMonths,
				Gender:        lookup.Gender,
				TScore:        lookup.TScore,
				Percentile:    lookup.Percentile,
				StandardScore: cloneFloat64(lookup.StandardScore),
			})
		}
		out.Factors = append(out.Factors, item)
	}
	if err := calcnorm.ValidateTables(out); err != nil {
		return nil, calcnorm.NewInvalidError("", fmt.Errorf("invalid runtime norm: %w", err))
	}
	return out, nil
}

func cloneFloat64(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
