package execute

import (
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func modelRouteFromInput(input *evaluationinput.InputSnapshot) (evalpipeline.ModelRoute, bool) {
	return evaloutcome.ModelRouteFromInput(input)
}

func modelRouteFromAssessment(a *assessment.Assessment) (evalpipeline.ModelRoute, bool) {
	if a == nil || a.EvaluationModelRef() == nil || a.EvaluationModelRef().IsEmpty() {
		return evalpipeline.ModelRoute{}, false
	}
	ref := a.EvaluationModelRef()
	return evalpipeline.ModelRoute{
		Kind: ref.Kind(), SubKind: ref.SubKind(), Algorithm: ref.Algorithm(),
	}, true
}
