package cachegovernance

import "github.com/FangcunMount/qs-server/internal/pkg/redisplane"

// FamilyRuntime 只暴露预热治理需要的 family 运行时能力。
type FamilyRuntime interface {
	AllowWarmup(family redisplane.Family) bool
}

type familyRuntime struct {
	handles map[redisplane.Family]*redisplane.Handle
}

// NewFamilyRuntime 基于已解析的 redis family handle 创建最小运行时视图。
func NewFamilyRuntime(handles ...*redisplane.Handle) FamilyRuntime {
	runtime := &familyRuntime{handles: make(map[redisplane.Family]*redisplane.Handle, len(handles))}
	for _, handle := range handles {
		if handle == nil {
			continue
		}
		runtime.handles[handle.Family] = handle
	}
	return runtime
}

func (r *familyRuntime) AllowWarmup(family redisplane.Family) bool {
	if r == nil {
		return false
	}
	handle := r.handles[family]
	return handle != nil && handle.AllowWarmup
}
