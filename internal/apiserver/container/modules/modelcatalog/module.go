package modelcatalog

import "github.com/FangcunMount/qs-server/internal/apiserver/container/modules"

const Name = modules.PackageModelCatalog

// RegisterNames 列出模型目录的注册模块键
var RegisterNames = []string{string(Name)}

// Descriptor 标识容器组合中的模型目录模块
type Descriptor struct {
	Name          modules.PackageName
	RegisterNames []string
}

// Describe 返回模型目录模块描述符
func Describe() Descriptor {
	return Descriptor{
		Name:          Name,
		RegisterNames: append([]string(nil), RegisterNames...),
	}
}
