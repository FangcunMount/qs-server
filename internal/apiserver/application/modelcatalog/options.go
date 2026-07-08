package modelcatalog

import domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"

// ModelCatalogOption 是 REST/BFF 展示的目录类型选项（由 option 注册表投影）。
// 领域执行守卫见 domain/modelcatalog/binding.ModelFamilyCapability。
type ModelCatalogOption struct {
	APIKind        string
	DisplayName    string
	ProductChannel domain.Kind
	OptionsEnabled bool
	QRCodeEnabled  bool
	PreviewEnabled bool
}

// ModelCatalogOptions 返回API 类型 选项 从 目录选项注册表。
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
