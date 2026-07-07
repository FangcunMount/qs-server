package typology

import (
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// Module 描述类型学算法 别名 exposed 到 评估 装配。
type Module struct {
	Algorithm     modelcatalog.Algorithm
	CategoryLabel string
}

// Descriptor 返回评估 注册表条目 用于 这个module。
func (m Module) Descriptor() evaldomain.ModelDescriptor {
	return evaldomain.ModelDescriptor{
		Kind:      evaldomain.ModelKindTypology,
		Algorithm: m.Algorithm,
	}
}

// ModuleDescriptors 投影已注册 类型学 modules 为 评估 描述符。
func ModuleDescriptors(modules []Module) []evaldomain.ModelDescriptor {
	out := make([]evaldomain.ModelDescriptor, 0, len(modules))
	for _, module := range modules {
		if module.Algorithm == "" {
			continue
		}
		out = append(out, module.Descriptor())
	}
	return out
}

// ConfiguredTypologyDescriptor 返回通用 配置化 类型学 路由 描述符。
func ConfiguredTypologyDescriptor() evaldomain.ModelDescriptor {
	return evaldomain.ModelDescriptor{
		Kind:      evaldomain.ModelKindTypology,
		Algorithm: modelcatalog.AlgorithmPersonalityTypology,
	}
}

// 默认TypologyDescriptors 返回单一 配置化 类型学 路由 描述符。
func DefaultTypologyDescriptors() []evaldomain.ModelDescriptor {
	return []evaldomain.ModelDescriptor{ConfiguredTypologyDescriptor()}
}
