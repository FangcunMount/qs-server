package definition

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// ValidateDefinitionV2ForPublish translates target-domain validation errors to
// the application validation contract and verifies external norm references.
func ValidateDefinitionV2ForPublish(ctx context.Context, value *modeldefinition.Definition, norms port.NormRepository) []domain.DomainValidationIssue {
	if value == nil {
		return []domain.DomainValidationIssue{{
			Field: "definition_v2", Code: "definition_v2.required", Message: "DefinitionV2 不能为空", Level: domain.ValidationLevelError,
		}}
	}
	issues := make([]domain.DomainValidationIssue, 0)
	for _, item := range modeldefinition.Validate(*value) {
		issues = append(issues, domain.DomainValidationIssue{
			Field: item.Field, Code: item.Code, Message: item.Message, Level: domain.ValidationLevelError,
		})
	}
	for _, ref := range value.Calibration.NormRefs {
		if ref.NormTableVersion == "" {
			continue
		}
		if norms == nil {
			issues = append(issues, domain.DomainValidationIssue{
				Field: "calibration.norm_refs", Code: "norm.repository.required", Message: "常模仓储未配置", Level: domain.ValidationLevelError,
			})
			break
		}
		if _, err := norms.FindNorm(ctx, ref.NormTableVersion); err != nil {
			issues = append(issues, domain.DomainValidationIssue{
				Field: "calibration.norm_refs", Code: "norm.not_found", Message: "常模表不存在: " + ref.NormTableVersion, Level: domain.ValidationLevelError,
			})
		}
	}
	return issues
}
