package modelcatalog

// ValidationLevel classifies a validation issue severity.
type ValidationLevel string

const (
	ValidationLevelError   ValidationLevel = "error"
	ValidationLevelWarning ValidationLevel = "warning"
)

// DomainValidationIssue is a structured validation finding at domain layer.
type DomainValidationIssue struct {
	Field   string
	Message string
	Code    string
	Level   ValidationLevel
}

// DomainValidationResult aggregates domain validation findings.
type DomainValidationResult struct {
	Issues []DomainValidationIssue
}

func (r DomainValidationResult) Passed() bool {
	if len(r.Issues) == 0 {
		return true
	}
	for _, issue := range r.Issues {
		if issue.Level == "" || issue.Level == ValidationLevelError {
			return false
		}
	}
	return true
}

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
	if m.Kind == KindPersonality {
		if m.SubKind != SubKindTypology {
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

func (m *AssessmentModel) ValidateForPublish() DomainValidationResult {
	result := m.ValidateBasic()
	if m != nil && m.IsArchived() {
		result.Issues = append(result.Issues, DomainValidationIssue{
			Field: "status", Message: "archived model cannot be published", Code: "status.archived", Level: ValidationLevelError,
		})
	}
	return result
}
