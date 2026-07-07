package modelcatalog

// ModelFamilyCapability is the pure domain capability for executable model families.
// It excludes API/UI options and product-channel taxonomy slots.
type ModelFamilyCapability struct {
	Kind                  Kind
	CreateSupported       bool
	PublishSupported      bool
	BindQuestionnaire     bool
	RuntimeExecutable     bool
	RuntimeViaScaleLegacy bool
	ExecutionPath         ExecutionPath
}

func (c ModelFamilyCapability) CanExecute() bool {
	return c.RuntimeExecutable || c.RuntimeViaScaleLegacy
}

// Allows reports whether the model family permits a catalog operation.
func (c ModelFamilyCapability) Allows(op CatalogOperation) bool {
	switch op {
	case CatalogOpCreate:
		return c.CreateSupported
	case CatalogOpList:
		return c.CreateSupported
	case CatalogOpUpdateBasicInfo, CatalogOpDelete:
		return c.CreateSupported
	case CatalogOpPublish, CatalogOpUnpublish, CatalogOpArchive:
		return c.PublishSupported
	case CatalogOpBindQuestionnaire:
		return c.BindQuestionnaire
	case CatalogOpUpdateDefinition:
		return c.CreateSupported
	case CatalogOpPreview, CatalogOpQRCode:
		return false
	default:
		return false
	}
}

// AllowsNewDraft reports whether catalog APIs may create new draft models for this family.
func (c ModelFamilyCapability) AllowsNewDraft() bool {
	return c.CreateSupported
}

// ModelFamilyCapabilitiesV2 returns pure domain model-family capabilities.
// Product-channel slots such as behavior_ability are excluded.
func ModelFamilyCapabilitiesV2() []ModelFamilyCapability {
	out := make([]ModelFamilyCapability, 0, len(defaultCapabilities))
	for _, cap := range defaultCapabilities {
		if cap.Role != CapabilityRoleModelFamily {
			continue
		}
		out = append(out, modelFamilyCapabilityFromKind(cap))
	}
	return out
}

// ModelFamilyCapabilityByKind resolves a pure domain model-family capability.
func ModelFamilyCapabilityByKind(kind Kind) (ModelFamilyCapability, bool) {
	cap, ok := CapabilityByKind(kind)
	if !ok || cap.Role != CapabilityRoleModelFamily {
		return ModelFamilyCapability{}, false
	}
	return modelFamilyCapabilityFromKind(cap), true
}

func modelFamilyCapabilityFromKind(cap KindCapability) ModelFamilyCapability {
	return ModelFamilyCapability{
		Kind:                  cap.Kind,
		CreateSupported:       cap.CreateSupported,
		PublishSupported:      cap.PublishSupported,
		BindQuestionnaire:     cap.BindQuestionnaire,
		RuntimeExecutable:     cap.RuntimeExecutable,
		RuntimeViaScaleLegacy: cap.RuntimeViaScaleLegacy,
		ExecutionPath:         cap.ExecutionPath,
	}
}
