package publishing

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
)

type (
	ValidationLevel        = binding.ValidationLevel
	DomainValidationIssue  = binding.DomainValidationIssue
	DomainValidationResult = binding.DomainValidationResult
)

const (
	ValidationLevelError   = binding.ValidationLevelError
	ValidationLevelWarning = binding.ValidationLevelWarning
)

// ValidateBasic 检查required draft 字段 在之前 publish。
func (m *AssessmentModel) ValidateBasic() DomainValidationResult {
	var issues []DomainValidationIssue
	if m == nil {
		return DomainValidationResult{Issues: []DomainValidationIssue{{
			Field: "model", Message: "model is nil", Code: "model.required", Level: ValidationLevelError,
		}}}
	}
	if m.Code == "" {
		issues = append(issues, DomainValidationIssue{Field: "code", Message: "code is required", Code: "code.required", Level: ValidationLevelError})
	}
	if m.Title == "" {
		issues = append(issues, DomainValidationIssue{Field: "title", Message: "title is required", Code: "title.required", Level: ValidationLevelError})
	}
	if !m.Kind.IsValid() {
		issues = append(issues, DomainValidationIssue{Field: "kind", Message: "kind is invalid", Code: "kind.invalid", Level: ValidationLevelError})
	}
	if m.Binding.QuestionnaireCode == "" {
		issues = append(issues, DomainValidationIssue{Field: "binding.questionnaire_code", Message: "questionnaire code is required", Code: "binding.questionnaire_code.required", Level: ValidationLevelError})
	}
	if m.Binding.QuestionnaireVersion == "" {
		issues = append(issues, DomainValidationIssue{Field: "binding.questionnaire_version", Message: "questionnaire version is required", Code: "binding.questionnaire_version.required", Level: ValidationLevelError})
	}
	if m.Definition.IsEmpty() {
		issues = append(issues, DomainValidationIssue{Field: "definition", Message: "definition payload is required", Code: "definition.required", Level: ValidationLevelError})
	}
	if m.Kind == binding.KindPersonality {
		if m.SubKind != binding.SubKindTypology {
			issues = append(issues, DomainValidationIssue{Field: "sub_kind", Message: "personality models require sub_kind typology", Code: "sub_kind.typology.required", Level: ValidationLevelError})
		}
		if m.Algorithm == "" {
			issues = append(issues, DomainValidationIssue{Field: "algorithm", Message: "algorithm is required", Code: "algorithm.required", Level: ValidationLevelError})
		}
		if m.Definition.Format != "" && m.Definition.Format != PayloadFormatPersonalityTypologyV1 {
			issues = append(issues, DomainValidationIssue{Field: "definition.format", Message: "unsupported personality payload format", Code: "definition.format.unsupported", Level: ValidationLevelError})
		}
	}
	return DomainValidationResult{Issues: issues}
}

// ValidateForPublish 检查publish readiness including 共享 因子 层级 rules。
func (m *AssessmentModel) ValidateForPublish() DomainValidationResult {
	result := m.ValidateBasic()
	if m != nil && m.IsArchived() {
		result.Issues = append(result.Issues, DomainValidationIssue{
			Field: "status", Message: "archived model cannot be published", Code: "status.archived", Level: ValidationLevelError,
		})
	}
	if m != nil && usesSharedFactorDefinitionBody(m.Kind) && !m.Definition.IsEmpty() {
		result.Issues = append(result.Issues, validateSharedFactorDefinitionForPublish(m.Definition.Data)...)
	}
	return result
}

func usesSharedFactorDefinitionBody(kind binding.Kind) bool {
	switch kind {
	case binding.KindBehavioralRating, binding.KindCognitive:
		return true
	default:
		return false
	}
}

func validateSharedFactorDefinitionForPublish(data []byte) []DomainValidationIssue {
	issues, err := factor.ValidateDefinitionBodyJSONForPublish(data)
	if err != nil {
		return []DomainValidationIssue{{
			Field:   "definition.payload",
			Code:    "definition.payload.invalid",
			Message: "模型定义 payload 不是有效的 factor 结构",
			Level:   ValidationLevelError,
		}}
	}
	return hierarchyIssuesToDomain(issues)
}

func hierarchyIssuesToDomain(issues []factor.HierarchyIssue) []DomainValidationIssue {
	if len(issues) == 0 {
		return nil
	}
	out := make([]DomainValidationIssue, 0, len(issues))
	for _, issue := range issues {
		out = append(out, DomainValidationIssue{
			Field:   issue.Field,
			Code:    issue.Code,
			Message: issue.Message,
			Level:   ValidationLevelError,
		})
	}
	return out
}
