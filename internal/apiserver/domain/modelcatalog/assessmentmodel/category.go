package assessmentmodel

import "strings"

const (
	ScaleCategoryADHD               = "adhd"
	ScaleCategoryTicDisorder        = "td"
	ScaleCategoryAutismSpectrum     = "asd"
	ScaleCategoryPressure           = "pressure"
	ScaleCategorySensoryIntegration = "sii"
	ScaleCategoryExecutiveFunction  = "efn"
	ScaleCategoryEmotion            = "emt"
	ScaleCategorySleep              = "slp"
)

var medicalScaleCategories = []string{
	ScaleCategoryADHD,
	ScaleCategoryTicDisorder,
	ScaleCategoryAutismSpectrum,
	ScaleCategoryPressure,
	ScaleCategorySensoryIntegration,
	ScaleCategoryExecutiveFunction,
	ScaleCategoryEmotion,
	ScaleCategorySleep,
}

// MedicalScaleCategories returns the canonical writable categories for scale
// catalogue metadata. Callers receive a copy so the domain vocabulary cannot
// be mutated by presentation layers.
func MedicalScaleCategories() []string {
	return append([]string(nil), medicalScaleCategories...)
}

// IsMedicalScaleCategory reports whether value is a canonical writable scale
// category. Historical compatibility values are intentionally excluded.
func IsMedicalScaleCategory(value string) bool {
	value = strings.TrimSpace(value)
	for _, category := range medicalScaleCategories {
		if value == category {
			return true
		}
	}
	return false
}
