package evaluation

import (
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// ExecutionPathForDescriptor 映射运行时描述符 到 its 物化路径。
func ExecutionPathForDescriptor(desc ModelDescriptor) (modelcatalog.ExecutionPath, error) {
	return evalpipeline.ExecutionPathForModelKind(evalpipeline.ModelKind(desc.Kind))
}
