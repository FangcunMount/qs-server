package execute

import (
	evaloutcome "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/outcome"
	evalpipeline "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
)

func modelRouteFromInput(input *evaluationinput.InputSnapshot) (evalpipeline.ModelRoute, bool) {
	return evaloutcome.ModelRouteFromInput(input)
}
