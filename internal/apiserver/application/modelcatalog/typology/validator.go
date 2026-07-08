package typology

import (
	"encoding/json"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/publishing"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/typology"
)

func validateDefinitionPayloadForSave(format string, data []byte) []ValidationIssue {
	if len(data) == 0 {
		return []ValidationIssue{{
			Field: "definition.payload", Message: "模型定义 payload 不能为空",
			Code: "definition.payload.required", Level: "error",
		}}
	}
	if format != "" && format != domain.PayloadFormatPersonalityTypologyV1 {
		return []ValidationIssue{{
			Field: "definition.payload_format", Message: "人格模型 payload_format 无效",
			Code: "definition.payload_format.unsupported", Level: "error",
		}}
	}
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return []ValidationIssue{{
			Field: "definition.payload", Message: "模型定义 payload 格式无效",
			Code: "definition.payload.invalid", Level: "error",
		}}
	}
	return nil
}

func validateDefinitionPayloadForPublish(model *domain.AssessmentModel) (*modeltypology.RuntimeSpec, modeltypology.RuntimeSpecValidationContext, []ValidationIssue) {
	validationContext := modeltypology.RuntimeSpecValidationContext{}
	if model == nil || model.Definition.IsEmpty() {
		return nil, validationContext, []ValidationIssue{{
			Field: "definition.payload", Message: "模型定义 payload 不能为空",
			Code: "definition.payload.required", Level: "error",
		}}
	}
	validationContext.Algorithm = model.Algorithm
	if issues := validateDefinitionPayloadForSave(model.Definition.Format, model.Definition.Data); len(issues) > 0 {
		return nil, validationContext, issues
	}
	payload, runtime, err := publishing.TypologyPayloadAndRuntimeSpecFromModel(model)
	if err != nil {
		return nil, validationContext, []ValidationIssue{{
			Field: "definition.payload", Message: err.Error(),
			Code: "definition.payload.invalid", Level: "error",
		}}
	}
	if payload != nil {
		if payload.Algorithm != "" {
			validationContext.Algorithm = payload.Algorithm
		}
		validationContext.Outcomes = append([]modeltypology.Outcome(nil), payload.Outcomes...)
	}
	return runtime, validationContext, nil
}

func mergeValidationIssues(groups ...[]ValidationIssue) []ValidationIssue {
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	if total == 0 {
		return nil
	}
	out := make([]ValidationIssue, 0, total)
	for _, group := range groups {
		out = append(out, group...)
	}
	return out
}
