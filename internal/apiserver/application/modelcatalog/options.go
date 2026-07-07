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

// ModelCatalogOptions returns API kind options from the model-family capability matrix.
func ModelCatalogOptions() []ModelCatalogOption {
	caps := domain.DefaultCapabilities()
	options := make([]ModelCatalogOption, 0, len(caps))
	for _, cap := range caps {
		apiKind := cap.APIKind
		if apiKind == "" {
			apiKind = DomainKindToAPIKind(cap.Kind)
		}
		if apiKind == "" {
			continue
		}
		options = append(options, ModelCatalogOption{
			APIKind:        apiKind,
			DisplayName:    cap.DisplayName,
			ProductChannel: productChannelForCapability(cap),
			OptionsEnabled: cap.OptionsEnabled,
			QRCodeEnabled:  cap.QRCodeSupported,
			PreviewEnabled: cap.PreviewSupported,
		})
	}
	return options
}

func productChannelForCapability(cap domain.KindCapability) domain.Kind {
	if cap.IsProductChannel() {
		return cap.Kind
	}
	return ""
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
