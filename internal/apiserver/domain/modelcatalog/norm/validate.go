package norm

import (
	"fmt"
	"math"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

// ValidateImport validates immutable norm reference material before it reaches
// persistence. Runtime lookup assumes these ranges are finite and unambiguous.
func ValidateImport(table *Norm) error {
	if table == nil {
		return invalid("norm table is required")
	}
	if table.TableVersion == "" {
		return invalid("norm table version is required")
	}
	if table.FormVariant == "" {
		return invalid("norm form variant is required")
	}
	if err := validateIdentity(table.Kind, table.Algorithm); err != nil {
		return err
	}
	if len(table.Factors) == 0 {
		return invalid("norm table requires at least one factor")
	}
	seen := make(map[string]struct{}, len(table.Factors))
	for index := range table.Factors {
		factor := &table.Factors[index]
		if factor.FactorCode == "" {
			return invalid("factor %d code is required", index)
		}
		if _, duplicate := seen[factor.FactorCode]; duplicate {
			return invalid("factor %s is duplicated", factor.FactorCode)
		}
		seen[factor.FactorCode] = struct{}{}
		if len(factor.Lookup) == 0 && len(factor.Bands) == 0 {
			return invalid("factor %s requires lookup rows or parametric bands", factor.FactorCode)
		}
		if err := validateLookup(factor.FactorCode, factor.Lookup); err != nil {
			return err
		}
		if err := validateBands(factor.FactorCode, factor.Bands); err != nil {
			return err
		}
	}
	return nil
}

func validateIdentity(kind identity.Kind, algorithm identity.Algorithm) error {
	if !kind.IsValid() {
		return invalid("norm kind %q is invalid", kind)
	}
	switch kind {
	case identity.KindBehavioralRating:
		switch algorithm {
		case identity.AlgorithmBrief2, identity.AlgorithmSPMSensory, identity.AlgorithmBehavioralRatingDefault:
			return nil
		}
	case identity.KindCognitive:
		switch algorithm {
		case identity.AlgorithmSPM:
			return nil
		}
	}
	return invalid("norm algorithm %q is incompatible with kind %q", algorithm, kind)
}

func validateLookup(factorCode string, rows []LookupEntry) error {
	for index, row := range rows {
		if !finite(row.RawScoreMin) || !finite(row.RawScoreMax) || row.RawScoreMin > row.RawScoreMax {
			return invalid("factor %s lookup %d has invalid raw-score range", factorCode, index)
		}
		if err := validateScope(factorCode, "lookup", index, row.MinAgeMonths, row.MaxAgeMonths, row.Gender); err != nil {
			return err
		}
		if !finite(row.TScore) {
			return invalid("factor %s lookup %d T score must be finite", factorCode, index)
		}
		if !finite(row.Percentile) || row.Percentile < 0 || row.Percentile > 100 {
			return invalid("factor %s lookup %d percentile must be between 0 and 100", factorCode, index)
		}
		if row.StandardScore != nil && !finite(*row.StandardScore) {
			return invalid("factor %s lookup %d standard score must be finite", factorCode, index)
		}
	}
	for left := 0; left < len(rows); left++ {
		for right := left + 1; right < len(rows); right++ {
			if rawRangesOverlap(rows[left], rows[right]) && lookupScopesAmbiguous(rows[left], rows[right]) {
				return invalid("factor %s lookup rows %d and %d overlap for the same demographic scope", factorCode, left, right)
			}
		}
	}
	return nil
}

func validateBands(factorCode string, bands []Band) error {
	for index, band := range bands {
		if err := validateScope(factorCode, "band", index, band.MinAgeMonths, band.MaxAgeMonths, band.Gender); err != nil {
			return err
		}
		if band.Mean == nil || band.StdDev == nil {
			return invalid("factor %s band %d requires mean and std_dev", factorCode, index)
		}
		if !finite(*band.Mean) || !finite(*band.StdDev) || *band.StdDev <= 0 {
			return invalid("factor %s band %d requires finite mean and positive std_dev", factorCode, index)
		}
	}
	for left := 0; left < len(bands); left++ {
		for right := left + 1; right < len(bands); right++ {
			if demographicScopesOverlap(bands[left].MinAgeMonths, bands[left].MaxAgeMonths, bands[left].Gender, bands[right].MinAgeMonths, bands[right].MaxAgeMonths, bands[right].Gender) {
				return invalid("factor %s bands %d and %d overlap for the same demographic scope", factorCode, left, right)
			}
		}
	}
	return nil
}

func rawRangesOverlap(left, right LookupEntry) bool {
	return left.RawScoreMin <= right.RawScoreMax && right.RawScoreMin <= left.RawScoreMax
}

func lookupScopesAmbiguous(left, right LookupEntry) bool {
	leftGeneric := left.MinAgeMonths == 0 && left.MaxAgeMonths == 0 && left.Gender == ""
	rightGeneric := right.MinAgeMonths == 0 && right.MaxAgeMonths == 0 && right.Gender == ""
	if leftGeneric || rightGeneric {
		// Generic lookup rows are explicit fallbacks and may cover the same raw
		// score as a more specific demographic row.
		return leftGeneric && rightGeneric
	}
	return demographicScopesOverlap(left.MinAgeMonths, left.MaxAgeMonths, left.Gender, right.MinAgeMonths, right.MaxAgeMonths, right.Gender)
}

func demographicScopesOverlap(leftMin, leftMax int, leftGender string, rightMin, rightMax int, rightGender string) bool {
	if leftGender != "" && rightGender != "" && leftGender != rightGender {
		return false
	}
	return ageRangesOverlap(leftMin, leftMax, rightMin, rightMax)
}

func ageRangesOverlap(leftMin, leftMax, rightMin, rightMax int) bool {
	leftUpper, rightUpper := leftMax, rightMax
	if leftUpper == 0 {
		leftUpper = math.MaxInt
	}
	if rightUpper == 0 {
		rightUpper = math.MaxInt
	}
	return leftMin <= rightUpper && rightMin <= leftUpper
}

func validateScope(factorCode, kind string, index, minAge, maxAge int, gender string) error {
	if minAge < 0 || maxAge < 0 || (maxAge > 0 && minAge > maxAge) {
		return invalid("factor %s %s %d has invalid age range", factorCode, kind, index)
	}
	if gender != "" && gender != "male" && gender != "female" {
		return invalid("factor %s %s %d has invalid gender %q", factorCode, kind, index, gender)
	}
	return nil
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", binding.ErrInvalidArgument, fmt.Sprintf(format, args...))
}
