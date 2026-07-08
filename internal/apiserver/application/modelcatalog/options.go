package modelcatalog

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/application/modelcatalog/option"
)

// ModelCatalogOption 是 REST/BFF 展示的目录类型选项（由 option 注册表投影）。
type ModelCatalogOption = option.ModelCatalogOption

// ModelCatalogOptions 返回 API 类型选项（目录展示投影）。
func ModelCatalogOptions() []ModelCatalogOption {
	return option.DefaultOptions()
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
