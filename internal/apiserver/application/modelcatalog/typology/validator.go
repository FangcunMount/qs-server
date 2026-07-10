package typology

import (
	"encoding/json"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
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
