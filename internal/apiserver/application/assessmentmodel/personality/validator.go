package personality

import (
	"encoding/json"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel"
	modeltypology "github.com/FangcunMount/qs-server/internal/apiserver/domain/assessmentmodel/personality/typology"
)

func validateDefinitionPayload(format string, algorithm domain.Algorithm, data []byte) []ValidationIssue {
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
	var payload modeltypology.Payload
	if err := json.Unmarshal(data, &payload); err == nil && (payload.HasExplicitRuntime() || payload.Algorithm != "" || len(payload.Dimensions) > 0) {
		if payload.Algorithm == "" {
			payload.Algorithm = algorithm
		}
		if _, err := payload.ToRuntimeSpec(); err != nil {
			return []ValidationIssue{{
				Field: "definition.payload", Message: err.Error(),
				Code: "definition.payload.invalid", Level: "error",
			}}
		}
		return nil
	}
	var runtime modeltypology.RuntimeSpec
	if err := json.Unmarshal(data, &runtime); err != nil {
		return []ValidationIssue{{
			Field: "definition.payload", Message: "模型定义 payload 格式无效",
			Code: "definition.payload.invalid", Level: "error",
		}}
	}
	wrapped := &modeltypology.Payload{Algorithm: algorithm, Runtime: &runtime}
	if _, err := wrapped.ToRuntimeSpec(); err != nil {
		return []ValidationIssue{{
			Field: "definition.payload", Message: err.Error(),
			Code: "definition.payload.invalid", Level: "error",
		}}
	}
	return nil
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
