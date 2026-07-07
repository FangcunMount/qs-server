package factor

// Brief2NormContext carries Brief-2 norm metadata without embedding norm table bodies.
type Brief2NormContext struct {
	NormTableVersion string
	IndexCodes       []string
	ValidityCodes    []string
	NormFactorCodes  []string
}

// SPMNormContext carries SPM norm/task metadata without embedding norm table bodies.
type SPMNormContext struct {
	NormTableVersion string
	ItemSetCodes     []string
}

// ApplyBrief2NormMetadata annotates canonical factors with Brief-2 roles and norm references.
func ApplyBrief2NormMetadata(factors []FactorSnapshot, ctx Brief2NormContext) []FactorSnapshot {
	if len(factors) == 0 {
		return factors
	}
	indexCodes := stringSet(ctx.IndexCodes)
	validityCodes := stringSet(ctx.ValidityCodes)
	normFactorCodes := stringSet(ctx.NormFactorCodes)
	out := make([]FactorSnapshot, len(factors))
	for i, factor := range factors {
		out[i] = factor
		switch {
		case indexCodes[factor.Code]:
			out[i].Role = FactorRoleIndex
		case validityCodes[factor.Code]:
			out[i].Role = FactorRoleValidity
		}
		if normFactorCodes[factor.Code] && ctx.NormTableVersion != "" {
			out[i].Norm = &NormRef{
				FactorCode:       factor.Code,
				NormTableVersion: ctx.NormTableVersion,
			}
		}
	}
	return out
}

// ApplySPMNormMetadata annotates canonical factors with SPM task-set roles and norm references.
func ApplySPMNormMetadata(factors []FactorSnapshot, ctx SPMNormContext) []FactorSnapshot {
	if len(factors) == 0 {
		return factors
	}
	itemSetCodes := stringSet(ctx.ItemSetCodes)
	out := make([]FactorSnapshot, len(factors))
	for i, factor := range factors {
		out[i] = factor
		if itemSetCodes[factor.Code] {
			out[i].Role = FactorRoleTaskSet
		}
		if ctx.NormTableVersion != "" && (factor.IsTotalScore || itemSetCodes[factor.Code]) {
			out[i].Norm = &NormRef{
				FactorCode:       factor.Code,
				NormTableVersion: ctx.NormTableVersion,
			}
		}
	}
	return out
}

func stringSet(values []string) map[string]bool {
	if len(values) == 0 {
		return nil
	}
	set := make(map[string]bool, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		set[value] = true
	}
	return set
}
