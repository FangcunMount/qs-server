package modelcatalog

import domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"

// ModelCatalogOption is an application-facing model-catalog kind option.
// It includes API/UI metadata and legacy product-channel slots.
type ModelCatalogOption struct {
	APIKind        string
	DisplayName    string
	ProductChannel domain.Kind
	OptionsEnabled bool
	QRCodeEnabled  bool
	PreviewEnabled bool
}

// ModelCatalogOptions returns API kind options from the catalog option registry.
func ModelCatalogOptions() []ModelCatalogOption {
	presentation := catalogRegistry.PresentationOptions()
	options := make([]ModelCatalogOption, 0, len(presentation))
	for _, item := range presentation {
		options = append(options, ModelCatalogOption{
			APIKind:        item.APIKind,
			DisplayName:    item.DisplayName,
			ProductChannel: "",
			OptionsEnabled: item.OptionsEnabled,
			QRCodeEnabled:  item.QRCodeSupported,
			PreviewEnabled: item.PreviewSupported,
		})
	}
	return options
}

func apiKindOptions() []Option {
	options := ModelCatalogOptions()
	out := make([]Option, 0, len(options))
	for _, opt := range options {
		out = append(out, Option{
			Label:    opt.DisplayName,
			Value:    opt.APIKind,
			Disabled: !opt.OptionsEnabled,
		})
	}
	return out
}
