package runtime

import evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"

// 默认RuntimeDescriptorRegistry registers 机制 描述符 aligned 使用 materialize 因子ies。
func DefaultRuntimeDescriptorRegistry() (*evalpipeline.RuntimeDescriptorRegistry, error) {
	registry := evalpipeline.NewRuntimeDescriptorRegistry()
	descs, err := runtimeDescriptorsFromSpecs(defaultPathMaterializations())
	if err != nil {
		return nil, err
	}
	for _, desc := range descs {
		if err := registry.Register(desc); err != nil {
			return nil, err
		}
	}
	return registry, nil
}
