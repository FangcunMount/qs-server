package factor

// NormRef points to algorithm-specific norm tables without embedding table bodies.
type NormRef struct {
	FactorCode       string
	NormTableVersion string
}
