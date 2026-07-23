package descriptor

import (
	"fmt"

	evalrouting "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/routing"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// RuntimeDescriptorRegistry 解析运行时描述符 按 机制键。
type RuntimeDescriptorRegistry struct {
	byKey map[DescriptorKey]RuntimeDescriptor
}

// NewRuntimeDescriptorRegistry 创建空 描述符注册表。
func NewRuntimeDescriptorRegistry() *RuntimeDescriptorRegistry {
	return &RuntimeDescriptorRegistry{byKey: make(map[DescriptorKey]RuntimeDescriptor)}
}

// Register adds one exact DecisionKind descriptor. Its family is validated and
// retained only by the in-process descriptor implementation.
func (r *RuntimeDescriptorRegistry) Register(desc RuntimeDescriptor) error {
	if r == nil {
		return fmt.Errorf("runtime descriptor registry is nil")
	}
	if !desc.AlgorithmFamily.IsValid() {
		return fmt.Errorf("invalid algorithm family: %s", desc.AlgorithmFamily)
	}
	key := desc.Key
	if key.DecisionKind == "" {
		key.DecisionKind = desc.DecisionKind
	}
	if key.DecisionKind == "" {
		return fmt.Errorf("decision kind is required for runtime descriptor")
	}
	if family, ok := modelcatalog.AlgorithmFamilyFromDecisionKind(key.DecisionKind); !ok || family != desc.AlgorithmFamily {
		return fmt.Errorf("runtime descriptor identity conflict: %s", key)
	}
	if err := desc.CompletenessPolicy.Validate(); err != nil {
		return fmt.Errorf("runtime descriptor %s: %w", key, err)
	}
	if _, exists := r.byKey[key]; exists {
		return fmt.Errorf("runtime descriptor already registered for %s", key)
	}
	desc.Key = key
	desc.DecisionKind = key.DecisionKind
	r.byKey[key] = desc
	return nil
}

// Resolve 选择描述符 用于 模型路由。
func (r *RuntimeDescriptorRegistry) Resolve(route ModelRoute) (RuntimeDescriptor, error) {
	if r == nil {
		return RuntimeDescriptor{}, fmt.Errorf("runtime descriptor registry is nil")
	}
	key, err := evalrouting.DescriptorKeyFromRoute(route)
	if err != nil {
		return RuntimeDescriptor{}, err
	}
	if desc, ok := r.byKey[key]; ok {
		return desc, nil
	}
	return RuntimeDescriptor{}, fmt.Errorf("unsupported runtime descriptor key: %s", key)
}

func (r *RuntimeDescriptorRegistry) descriptorForFamily(family modelcatalog.AlgorithmFamily) (RuntimeDescriptor, bool) {
	for key, desc := range r.byKey {
		if derived, ok := modelcatalog.AlgorithmFamilyFromDecisionKind(key.DecisionKind); ok && derived == family {
			return desc, true
		}
	}
	return RuntimeDescriptor{}, false
}

// Len 返回数量 已注册 描述符。
func (r *RuntimeDescriptorRegistry) Len() int {
	if r == nil {
		return 0
	}
	return len(r.byKey)
}

// HasAlgorithmFamily 报告是否 家族-等级 描述符 是 已注册。
func (r *RuntimeDescriptorRegistry) HasAlgorithmFamily(family modelcatalog.AlgorithmFamily) bool {
	if r == nil {
		return false
	}
	_, ok := r.descriptorForFamily(family)
	return ok
}

// DescriptorForFamily returns the family-level runtime descriptor when registered.
func (r *RuntimeDescriptorRegistry) DescriptorForFamily(family modelcatalog.AlgorithmFamily) (RuntimeDescriptor, bool) {
	if r == nil {
		return RuntimeDescriptor{}, false
	}
	return r.descriptorForFamily(family)
}

// ReplaceFamilyDescriptor updates every exact decision entry in a family.
func (r *RuntimeDescriptorRegistry) ReplaceFamilyDescriptor(family modelcatalog.AlgorithmFamily, desc RuntimeDescriptor) error {
	if r == nil {
		return fmt.Errorf("runtime descriptor registry is nil")
	}
	found := false
	for key := range r.byKey {
		derived, ok := modelcatalog.AlgorithmFamilyFromDecisionKind(key.DecisionKind)
		if !ok || derived != family {
			continue
		}
		found = true
		value := desc
		value.Key = key
		value.AlgorithmFamily = family
		value.DecisionKind = key.DecisionKind
		r.byKey[key] = value
	}
	if !found {
		return fmt.Errorf("runtime descriptor is not registered for family %s", family)
	}
	return nil
}
