package cachegovernance

import "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/model"

// FamilyRuntime 只暴露预热治理需要的 family 运行时能力。
type FamilyRuntime interface {
	AllowWarmup(family cachemodel.Family) bool
}

type familyRuntime struct {
	families map[cachemodel.Family]bool
}

// NewFamilyRuntime 创建minimal 运行时 视图 required 按 缓存 governance。
func NewFamilyRuntime(families map[cachemodel.Family]bool) FamilyRuntime {
	runtime := &familyRuntime{families: make(map[cachemodel.Family]bool, len(families))}
	for family, allow := range families {
		runtime.families[family] = allow
	}
	return runtime
}

func (r *familyRuntime) AllowWarmup(family cachemodel.Family) bool {
	if r == nil {
		return false
	}
	return r.families[family]
}
