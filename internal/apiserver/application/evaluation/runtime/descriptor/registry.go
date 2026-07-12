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

// Register 添加运行时描述符. 载荷格式 可以是 空 到 match 任意 格式 在 家族。
func (r *RuntimeDescriptorRegistry) Register(desc RuntimeDescriptor) error {
	if r == nil {
		return fmt.Errorf("runtime descriptor registry is nil")
	}
	if !desc.AlgorithmFamily.IsValid() {
		return fmt.Errorf("invalid algorithm family: %s", desc.AlgorithmFamily)
	}
	key := desc.Key
	explicitKey := !key.IsZero()
	if key.AlgorithmFamily == "" {
		key.AlgorithmFamily = desc.AlgorithmFamily
	}
	if !explicitKey && key.PayloadFormat == "" {
		key.PayloadFormat = desc.PayloadFormat
	}
	if !explicitKey && key.DecisionKind == "" {
		key.DecisionKind = desc.DecisionKind
	}
	if _, exists := r.byKey[key]; exists {
		return fmt.Errorf("runtime descriptor already registered for %s", key)
	}
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
	formatKey := DescriptorKey{
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
	familyKey := DescriptorKey{AlgorithmFamily: family}
	if desc, ok := r.byKey[familyKey]; ok {
		return desc, true
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

// ExecutionPathForFamily returns the registered execution path for a family.
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

// DescriptorForFamily returns the family-level runtime descriptor when registered.
func (r *RuntimeDescriptorRegistry) DescriptorForFamily(family modelcatalog.AlgorithmFamily) (RuntimeDescriptor, bool) {
	if r == nil {
		return RuntimeDescriptor{}, false
	}
	return r.descriptorForFamily(family)
}

// ReplaceFamilyDescriptor updates the family-level descriptor entry in-place.
func (r *RuntimeDescriptorRegistry) ReplaceFamilyDescriptor(family modelcatalog.AlgorithmFamily, desc RuntimeDescriptor) error {
	if r == nil {
		return fmt.Errorf("runtime descriptor registry is nil")
	}
	familyKey := DescriptorKey{AlgorithmFamily: family}
	if _, ok := r.byKey[familyKey]; !ok {
		return fmt.Errorf("runtime descriptor is not registered for family %s", family)
	}
	r.byKey[familyKey] = desc
	return nil
}
