package execute

import (
	"fmt"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// ExecutionPathEvaluator 暴露物化路径 用于 evaluator。
type ExecutionPathEvaluator interface {
	ExecutionPath() modelcatalog.ExecutionPath
}

// ExecutionPathForEvaluator 解析执行路径 用于 evaluator。
func ExecutionPathForEvaluator(evaluator Evaluator) (modelcatalog.ExecutionPath, error) {
	if evaluator == nil {
		return "", fmt.Errorf("evaluation evaluator is nil")
	}
	if pathEvaluator, ok := evaluator.(ExecutionPathEvaluator); ok {
		return pathEvaluator.ExecutionPath(), nil
	}
	return evaluation.ExecutionPathForDescriptor(evaluation.ModelDescriptorFromIdentity(evaluator.ExecutionIdentity()))
}
