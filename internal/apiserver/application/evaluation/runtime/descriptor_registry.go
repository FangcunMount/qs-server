package runtime

import evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"

// DefaultRuntimeDescriptorRegistry registers mechanism descriptors aligned with materialize factories.
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
