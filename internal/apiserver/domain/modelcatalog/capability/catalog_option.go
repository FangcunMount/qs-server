package capability

import "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"

// CatalogOption 携带呈现 和 API-facing 目录选项 用于 模型家族。
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

// CatalogOptionByKind resolves presentation options for a model-family kind.
func CatalogOptionByKind(kind identity.Kind) (CatalogOption, bool) {
	for _, option := range defaultCatalogOptions {
		if option.Kind == kind {
			return option, true
		}
	}
	return CatalogOption{}, false
}
