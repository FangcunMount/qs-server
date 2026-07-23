package outcome

import (
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// ModelRouteFromInput builds a route only from the frozen published identity.
func ModelRouteFromInput(input *evaluationinput.InputSnapshot) (evalpipeline.ModelRoute, bool) {
	if input == nil || input.Model == nil {
		return evalpipeline.ModelRoute{}, false
	}
	route := evalpipeline.ModelRoute{DecisionKind: modelcatalog.DecisionKind(input.Model.DecisionKind)}
	return route, route.HasFrozenRuntime()
}
