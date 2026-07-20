package definition

import (
	"context"

	domain "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	modeldefinition "github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	port "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog"
)

// ValidateDefinitionV2 将目标域验证错误转换为应用验证合同，无需咨询外部存储库
func ValidateDefinitionV2(value *modeldefinition.Definition) []domain.DomainValidationIssue {
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
	return issues
}

// ValidateDefinitionV2ForPublish 验证 DefinitionV2 并验证外部常模引用
func ValidateDefinitionV2ForPublish(ctx context.Context, value *modeldefinition.Definition, norms port.NormRepository) []domain.DomainValidationIssue {
	return ValidateDefinitionV2ForPublishWithModel(ctx, nil, value, norms)
}

// ValidateDefinitionV2ForPublishWithModel 在发布时校验常模存在性及 Model/Norm 兼容性。
func ValidateDefinitionV2ForPublishWithModel(ctx context.Context, model *domain.AssessmentModel, value *modeldefinition.Definition, norms port.NormRepository) []domain.DomainValidationIssue {
	issues := ValidateDefinitionV2(value)
	if value == nil {
		return issues
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
		table, err := norms.FindNorm(ctx, ref.NormTableVersion)
		if err != nil {
			issues = append(issues, domain.DomainValidationIssue{
				Field: "calibration.norm_refs", Code: "norm.not_found", Message: "常模表不存在: " + ref.NormTableVersion, Level: domain.ValidationLevelError,
			})
			continue
		}
		if model != nil {
			issues = append(issues, CheckNormCompatibility(model, table, ref)...)
		}
	}
	return issues
}
