package pipeline

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// RuntimeDescriptorRegistry resolves runtime descriptors by mechanism keys.
type RuntimeDescriptorRegistry struct {
	byKey map[RuntimeDescriptorKey]RuntimeDescriptor
}

// NewRuntimeDescriptorRegistry creates an empty descriptor registry.
func NewRuntimeDescriptorRegistry() *RuntimeDescriptorRegistry {
	return &RuntimeDescriptorRegistry{byKey: make(map[RuntimeDescriptorKey]RuntimeDescriptor)}
}

// Register adds a runtime descriptor. PayloadFormat may be empty to match any format within the family.
func (r *RuntimeDescriptorRegistry) Register(desc RuntimeDescriptor) error {
	if r == nil {
		return fmt.Errorf("runtime descriptor registry is nil")
	}
	if !desc.AlgorithmFamily.IsValid() {
		return fmt.Errorf("invalid algorithm family: %s", desc.AlgorithmFamily)
	}
	key := desc.Key
	if key.AlgorithmFamily == "" {
		key.AlgorithmFamily = desc.AlgorithmFamily
	}
	if key.PayloadFormat == "" {
		key.PayloadFormat = desc.PayloadFormat
	}
	if key.DecisionKind == "" {
		key.DecisionKind = desc.DecisionKind
	}
	if _, exists := r.byKey[key]; exists {
		return fmt.Errorf("runtime descriptor already registered for %s", key)
	}
	r.byKey[key] = desc
	return nil
}

// Resolve selects a descriptor for a published model snapshot.
func (r *RuntimeDescriptorRegistry) Resolve(snapshot modelcatalog.PublishedModelSnapshot) (RuntimeDescriptor, error) {
	if r == nil {
		return RuntimeDescriptor{}, fmt.Errorf("runtime descriptor registry is nil")
	}
	key, err := RuntimeDescriptorKeyFromSnapshot(snapshot)
	if err != nil {
		return RuntimeDescriptor{}, err
	}
	if desc, ok := r.byKey[key]; ok {
		return desc, nil
	}
	formatKey := RuntimeDescriptorKey{
		AlgorithmFamily: key.AlgorithmFamily,
		PayloadFormat:   key.PayloadFormat,
	}
	if desc, ok := r.byKey[formatKey]; ok {
		return desc, nil
	}
	if desc, ok := r.descriptorForFamily(key.AlgorithmFamily); ok {
		return desc, nil
	}
	return RuntimeDescriptor{}, fmt.Errorf("unsupported runtime descriptor key: %s", key)
}

func (r *RuntimeDescriptorRegistry) descriptorForFamily(family modelcatalog.AlgorithmFamily) (RuntimeDescriptor, bool) {
	familyKey := RuntimeDescriptorKey{AlgorithmFamily: family}
	if desc, ok := r.byKey[familyKey]; ok {
		return desc, true
	}
	for key, desc := range r.byKey {
		if key.AlgorithmFamily == family {
			return desc, true
		}
	}
	return RuntimeDescriptor{}, false
}

// Len returns the number of registered descriptors.
func (r *RuntimeDescriptorRegistry) Len() int {
	if r == nil {
		return 0
	}
	return len(r.byKey)
}

// HasAlgorithmFamily reports whether a family-level descriptor is registered.
func (r *RuntimeDescriptorRegistry) HasAlgorithmFamily(family modelcatalog.AlgorithmFamily) bool {
	if r == nil {
		return false
	}
	_, ok := r.descriptorForFamily(family)
	return ok
}

// ExecutionPathForDescriptor returns the registered execution path for a family, if any.
func (r *RuntimeDescriptorRegistry) ExecutionPathForFamily(family modelcatalog.AlgorithmFamily) (modelcatalog.ExecutionPath, bool) {
	if r == nil {
		return "", false
	}
	desc, ok := r.descriptorForFamily(family)
	if !ok {
		return "", false
	}
	return desc.ExecutionPath, true
}
