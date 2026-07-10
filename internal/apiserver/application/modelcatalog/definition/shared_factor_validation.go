package definition

import (
	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	sharedpayload "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/shared"
)

// ValidateSharedFactorPayloadForPublish validates the behavioral/cognitive
// wire payload before publication and maps adapter issues to application output.
func ValidateSharedFactorPayloadForPublish(data []byte) []domain.DomainValidationIssue {
	issues, err := sharedpayload.ValidateDefinitionBodyJSONForPublish(data)
	if err != nil {
		return []domain.DomainValidationIssue{{
			Field:   "definition.payload",
			Code:    "definition.payload.invalid",
			Message: "模型定义 payload 不是有效的 factor 结构",
			Level:   domain.ValidationLevelError,
		}}
	}
	if len(issues) == 0 {
		return nil
	}
	out := make([]domain.DomainValidationIssue, 0, len(issues))
	for _, issue := range issues {
		out = append(out, domain.DomainValidationIssue{
			Field: issue.Field, Code: issue.Code, Message: issue.Message, Level: domain.ValidationLevelError,
		})
	}
	return out
}
