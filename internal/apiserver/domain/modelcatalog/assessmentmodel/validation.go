package assessmentmodel

import (
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
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

// ValidateBasic checks required draft fields before publish.
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
	if m.Definition.Format != "" {
		if err := validatePublishPayloadFormat(m.Definition.Format); err != nil {
			issues = append(issues, DomainValidationIssue{
				Field: "definition.format", Message: err.Error(), Code: "definition.format.legacy_decode_only", Level: ValidationLevelError,
			})
		}
	}
	if m.Kind == binding.KindTypology {
		if m.SubKind != binding.SubKindTypology {
			issues = append(issues, DomainValidationIssue{Field: "sub_kind", Message: "typology models require sub_kind typology", Code: "sub_kind.typology.required", Level: ValidationLevelError})
		}
		if m.Algorithm == "" {
			issues = append(issues, DomainValidationIssue{Field: "algorithm", Message: "algorithm is required", Code: "algorithm.required", Level: ValidationLevelError})
		}
		if m.Definition.Format != "" && m.Definition.Format != "assessmentmodel.personality.typology.v1" {
			issues = append(issues, DomainValidationIssue{Field: "definition.format", Message: "unsupported personality payload format", Code: "definition.format.unsupported", Level: ValidationLevelError})
		}
	}
	return DomainValidationResult{Issues: issues}
}

func validatePublishPayloadFormat(format string) error {
	for _, legacy := range []string{
		"ruleset.scale.v1",
		"ruleset.mbti.v1",
		"ruleset.sbti.v1",
		"evaluationinput.scale.v1",
		"evaluationinput.mbti.v1",
		"evaluationinput.sbti.v1",
		"assessmentmodel.behavioral_rating.brief2.v1",
		"assessmentmodel.cognitive.spm.v1",
	} {
		if format == legacy {
			return &legacyPayloadFormatError{format: format}
		}
	}
	return nil
}

type legacyPayloadFormatError struct {
	format string
}

func (e *legacyPayloadFormatError) Error() string {
	return "payload format " + `"` + e.format + `"` + " is legacy-decode-only and cannot be published"
}

// ValidateForPublish checks domain-owned publish readiness. Family wire payload
// validation is performed by the application definition handler.
func (m *AssessmentModel) ValidateForPublish() DomainValidationResult {
	result := m.ValidateBasic()
	if m != nil && m.IsArchived() {
		result.Issues = append(result.Issues, DomainValidationIssue{
			Field: "status", Message: "archived model cannot be published", Code: "status.archived", Level: ValidationLevelError,
		})
	}
	return result
}
