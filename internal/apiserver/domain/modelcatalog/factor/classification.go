package factor

// ClassificationSpec carries pole-composition metadata for factor_classification models.
// Personality typology still uses its own runtime graph; this type is the shared catalog seam.
type ClassificationSpec struct {
	PositivePole string
	NegativePole string
	DecisionRule string
	TieBreakRule string
}
