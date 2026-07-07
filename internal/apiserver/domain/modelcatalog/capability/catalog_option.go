package capability

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"

// CatalogOption carries presentation and API-facing catalog options for a model family.
type CatalogOption struct {
	Kind             identity.Kind
	APIKind          string
	DisplayName      string
	OptionsEnabled   bool
	PreviewSupported bool
	QRCodeSupported  bool
}

var defaultCatalogOptions = []CatalogOption{
	{Kind: identity.KindPersonality, APIKind: "personality", DisplayName: "人格测评", OptionsEnabled: true, PreviewSupported: true, QRCodeSupported: true},
	{Kind: identity.KindBehavioralRating, APIKind: string(identity.KindBehavioralRating), DisplayName: "行为评分", OptionsEnabled: true, QRCodeSupported: true},
	{Kind: identity.KindScale, APIKind: "medical_scale", DisplayName: "医学量表", OptionsEnabled: true},
	{Kind: identity.KindCognitive, APIKind: "cognitive", DisplayName: "认知测评", OptionsEnabled: true, QRCodeSupported: true},
	{Kind: identity.KindCustom, APIKind: "custom", DisplayName: "自定义测评"},
}

// DefaultCatalogOptions returns API/presentation options for model catalog surfaces.
func DefaultCatalogOptions() []CatalogOption {
	out := make([]CatalogOption, len(defaultCatalogOptions))
	copy(out, defaultCatalogOptions)
	return out
}

// CatalogOptionByKind resolves presentation options for a model family kind.
func CatalogOptionByKind(kind identity.Kind) (CatalogOption, bool) {
	for _, option := range defaultCatalogOptions {
		if option.Kind == kind {
			return option, true
		}
	}
	return CatalogOption{}, false
}

func mergeKindCapability(family ModelFamilyCapability, option CatalogOption) KindCapability {
	return KindCapability{
		Kind:                      family.Kind,
		Role:                      family.Role,
		APIKind:                   option.APIKind,
		DisplayName:               option.DisplayName,
		OptionsEnabled:            option.OptionsEnabled,
		CreateSupported:           family.CreateSupported,
		ListSupported:             family.ListSupported,
		PublishSupported:          family.PublishSupported,
		BindQuestionnaire:         family.BindQuestionnaire,
		DefinitionUpdateSupported: family.DefinitionUpdateSupported,
		PreviewSupported:          option.PreviewSupported,
		QRCodeSupported:           option.QRCodeSupported,
		RuntimeExecutable:         family.RuntimeExecutable,
		ExecutionPath:             family.ExecutionPath,
	}
}
