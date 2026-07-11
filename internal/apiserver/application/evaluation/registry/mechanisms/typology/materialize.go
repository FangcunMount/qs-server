package typology

import (
	"fmt"

	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	evaldomain "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// MaterializeEvaluator 构建或 reuses 配置化 类型学 executor 用于 描述符。
func MaterializeEvaluator(
	desc evaldomain.ModelDescriptor,
	registry ModuleRegistry,
	shared **Executor,
) (evaluationexecute.Evaluator, error) {
	if shared == nil {
		return nil, fmt.Errorf("shared typology executor holder is required")
	}
	if desc.Algorithm != modelcatalog.AlgorithmPersonalityTypology {
		return nil, fmt.Errorf("unsupported typology descriptor: kind=%s algorithm=%s", desc.Kind, desc.Algorithm)
	}
	if *shared == nil {
		executor, err := NewConfiguredTypologyExecutorWithRegistry(registry)
		if err != nil {
			return nil, err
		}
		*shared = executor
	}
	return *shared, nil
}
