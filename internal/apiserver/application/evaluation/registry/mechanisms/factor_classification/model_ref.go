package factor_classification

import (
	evaluationexecute "github.com/FangcunMount/qs-server/internal/apiserver/application/evaluation/execute"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/assessment"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/personality/typology"
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
		assessment.EvaluationModelKindPersonality,
		modelcatalog.SubKindTypology,
		payload.Algorithm,
		meta.ID(0),
		meta.NewCode(code),
		version,
		title,
	)
}
