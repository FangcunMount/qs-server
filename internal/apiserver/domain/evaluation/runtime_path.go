package evaluation

import (
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/pipeline"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
)

// ExecutionPathForDescriptor maps a runtime descriptor to its materialization path.
func ExecutionPathForDescriptor(desc ModelDescriptor) (modelcatalog.ExecutionPath, error) {
	return evalpipeline.ExecutionPathForModelKind(evalpipeline.ModelKind(desc.Kind))
}
