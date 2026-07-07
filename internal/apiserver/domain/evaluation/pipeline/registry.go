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
	familyKey := RuntimeDescriptorKey{AlgorithmFamily: key.AlgorithmFamily}
	if desc, ok := r.byKey[familyKey]; ok {
		return desc, nil
	}
	return RuntimeDescriptor{}, fmt.Errorf("unsupported runtime descriptor key: %s", key)
}

// Len returns the number of registered descriptors.
func (r *RuntimeDescriptorRegistry) Len() int {
	if r == nil {
		return 0
	}
	return len(r.byKey)
}
