package modelcatalog

import (
	evalregistry "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/registry"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
)

// DefaultTypologyModules 返回内置的模型算法别名
func DefaultTypologyModules() []evalregistry.TypologyModule {
	return evalregistry.DefaultTypologyModules()
}

// DefaultTypologyRegistry 构建模型目录的运行时注册表
func DefaultTypologyRegistry() (evalregistry.TypologyRegistry, error) {
	return evalregistry.DefaultTypologyRegistry()
}

// TypologyRegistryWith 构建模型目录的模块注册表
func TypologyRegistryWith(opts evalregistry.TypologyRuntimeOptions) (evalregistry.TypologyRegistry, error) {
	return evalregistry.TypologyRegistryWith(opts)
}

// DefaultTypologyDescriptors 配置模型目录的描述符
func DefaultTypologyDescriptors() []evaldomain.ModelDescriptor {
	return evalregistry.DefaultTypologyDescriptors()
}

// DefaultEvaluationDescriptors 返回所有能力支持的执行路径的运行时描述符
func DefaultEvaluationDescriptors() []evaldomain.ModelDescriptor {
	return evalregistry.DefaultEvaluationDescriptors()
}
