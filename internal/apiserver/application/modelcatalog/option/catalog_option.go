package option

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
)

// ModelCatalogOption 是application-facing 目录展示契约。
type ModelCatalogOption struct {
	Kind             binding.Kind
	APIKind          string
	DisplayName      string
	OptionsEnabled   bool
	PreviewSupported bool
	QRCodeSupported  bool
}

// 默认Options 返回目录展示选项 用于 REST/BFF surfaces。
func DefaultOptions() []ModelCatalogOption {
	return DefaultRegistry().PresentationOptions()
}

// ByKind 解析展示选项 用于 模型家族 类型。
func ByKind(kind binding.Kind) (ModelCatalogOption, bool) {
	entry, ok := DefaultRegistry().ByKind(kind)
	if !ok {
		return ModelCatalogOption{}, false
	}
	return entry.catalogOption(), true
}
