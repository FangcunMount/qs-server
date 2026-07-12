package typology

import (
	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/runtime/descriptor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/typology"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func modelRefFromExecutionInput(input evaluationexecute.ExecutionInput, payload *modeltypology.Payload) assessment.EvaluationModelRef {
	if input.Assessment != nil && input.Assessment.EvaluationModelRef() != nil {
		return *input.Assessment.EvaluationModelRef()
	}
	code := payload.Code
	version := payload.Version
	title := payload.Title
	if code == "" {
		code = string(payload.Algorithm)
	}
	return assessment.NewEvaluationModelRefWithIdentity(
		assessment.EvaluationModelKindTypology,
		modelcatalog.SubKindTypology,
		payload.Algorithm,
		meta.ID(0),
		meta.NewCode(code),
		version,
		title,
	)
}
