package outcome

import (
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

// ModelRouteFromInput 构建运行时路由 从 resolved 评估输入。
// Prefer frozen RuntimeIdentity on ModelSnapshot; only fall back for incomplete legacy inputs.
func ModelRouteFromInput(input *evaluationinput.InputSnapshot) (evalpipeline.ModelRoute, bool) {
	if input == nil {
		return evalpipeline.ModelRoute{}, false
	}
	if input.Model == nil {
		if scale, ok := evaluationinput.ScalePayload(input); ok {
			input.Model = evaluationinput.NewScaleModelSnapshot(scale)
		}
	}
	if input.Model == nil {
		return evalpipeline.ModelRoute{}, false
	}
	model := input.Model
	kind := modelcatalog.Kind(model.Kind)
	subKind := modelcatalog.SubKind(model.SubKind)
	algorithm := modelcatalog.Algorithm(model.Algorithm)

	decisionKind := modelcatalog.DecisionKind(model.DecisionKind)
	if decisionKind == "" {
		if payload, ok := evaluationinput.TypologyPayload(input); ok && payload.HasExplicitRuntime() && payload.Runtime.Decision.Kind != "" {
			decisionKind = payload.Runtime.Decision.Kind
		}
	}
	payloadFormat := model.PayloadFormat
	family := modelcatalog.AlgorithmFamily(model.AlgorithmFamily)

	route := evalpipeline.ModelRoute{
		Kind:            kind,
		SubKind:         subKind,
		Algorithm:       algorithm,
		AlgorithmFamily: family,
		DecisionKind:    decisionKind,
		PayloadFormat:   payloadFormat,
	}
	if route.HasFrozenRuntime() {
		return route, true
	}
	// Legacy incomplete inputs may still draft payload format; counted in DescriptorKeyFromRoute.
	if route.PayloadFormat == "" {
		route.PayloadFormat = modelcatalog.DraftPayloadFormatForModel(kind, algorithm)
	}
	return route, true
}
