package option

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/capability"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/identity"
)

// ModelCatalogOption 是application-facing 目录展示契约。
type ModelCatalogOption = capability.CatalogOption

// 默认Options 返回目录展示选项 用于 REST/BFF surfaces。
func DefaultOptions() []ModelCatalogOption {
	return capability.DefaultCatalogOptions()
}

// ByKind 解析展示选项 用于 模型家族 类型。
func ByKind(kind identity.Kind) (ModelCatalogOption, bool) {
	return capability.CatalogOptionByKind(kind)
}
