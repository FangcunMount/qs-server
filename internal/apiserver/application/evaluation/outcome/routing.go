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
	model := input.Model
	kind := modelcatalog.Kind(model.Kind)
	subKind := modelcatalog.SubKind(model.SubKind)
	algorithm := modelcatalog.Algorithm(model.Algorithm)

	decisionKind := modelcatalog.DecisionKind(model.DecisionKind)
	family := modelcatalog.AlgorithmFamily(model.AlgorithmFamily)

	route := evalpipeline.ModelRoute{
		Kind:            kind,
		SubKind:         subKind,
		Algorithm:       algorithm,
		AlgorithmFamily: family,
		DecisionKind:    decisionKind,
	}
	return route, route.HasFrozenRuntime()
}
