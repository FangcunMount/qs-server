package adapter

import (
	"fmt"

	outcometypology "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome/typology"
	evalinput "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
)

// ModelAdapter 计算类型学载荷 通过 人格画像流水线。
type ModelAdapter interface {
	Algorithm() modelcatalog.Algorithm
	Score(
		payload *modeltypology.Payload,
		sheet *evalinput.AnswerSheet,
	) (outcometypology.ScoringResult, error)
}

// Registry 解析人格模型适配器 按 算法。
type Registry struct {
	adapters map[modelcatalog.Algorithm]ModelAdapter
}

// NewRegistry 构建类型学 adapter 注册表 从 配置化 adapters。
func NewRegistry(adapters ...ModelAdapter) Registry {
	registry := Registry{adapters: make(map[modelcatalog.Algorithm]ModelAdapter, len(adapters))}
	for _, adapter := range adapters {
		if adapter == nil {
			continue
		}
		registry.adapters[adapter.Algorithm()] = adapter
	}
	return registry
}

func (r Registry) Resolve(algorithm modelcatalog.Algorithm) (ModelAdapter, error) {
	if adapter, ok := r.adapters[algorithm]; ok {
		return adapter, nil
	}
	return nil, fmt.Errorf("unsupported typology algorithm: %s", algorithm)
}

func (r Registry) Algorithms() []modelcatalog.Algorithm {
	out := make([]modelcatalog.Algorithm, 0, len(r.adapters))
	for algorithm := range r.adapters {
		out = append(out, algorithm)
	}
	return out
}
