// Package validation applies deterministic AIExplanationOutput v1 gates before
// any provider result can become an immutable Artifact. Semantic safety is a
// separate required gate owned by the execution pipeline.
package validation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	appinput "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/input"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/output"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
)

var (
	ErrSchema    = errors.New("AI explanation output schema validation failed")
	ErrReference = errors.New("AI explanation output reference validation failed")
	ErrProfile   = errors.New("AI explanation output Profile validation failed")
)

type SchemaViolation string

const (
	SchemaViolationObjectRequired  SchemaViolation = "object_required"
	SchemaViolationJSONSyntax      SchemaViolation = "json_syntax_invalid"
	SchemaViolationUnknownField    SchemaViolation = "unknown_field"
	SchemaViolationFieldType       SchemaViolation = "field_type_invalid"
	SchemaViolationDecode          SchemaViolation = "json_decode_invalid"
	SchemaViolationTrailingContent SchemaViolation = "trailing_content"
	SchemaViolationContentContract SchemaViolation = "content_contract_invalid"
)

type schemaViolationError struct {
	violation SchemaViolation
	detail    string
}

func (e *schemaViolationError) Error() string {
	return fmt.Sprintf("%s: %s", ErrSchema, e.detail)
}

func (e *schemaViolationError) Unwrap() error { return ErrSchema }

// SchemaViolationOf returns a reviewed, low-cardinality reason suitable for
// evaluation evidence and metrics. The original decoder detail remains local
// and must not become a metric label or an operator-facing remote error.
func SchemaViolationOf(err error) SchemaViolation {
	var violation *schemaViolationError
	if errors.As(err, &violation) {
		return violation.violation
	}
	return ""
}

func schemaViolation(violation SchemaViolation, detail string) error {
	return &schemaViolationError{violation: violation, detail: detail}
}

const (
	SchemaValidatorVersion    = "ai-explanation-output-typed/v1"
	ReferenceValidatorVersion = "ai-explanation-reference/v1"
	ProfileValidatorVersion   = "ai-explanation-profile-output/v1"
)

type Result struct {
	Content                   output.Content
	SchemaValidatorVersion    string
	ReferenceValidatorVersion string
	ProfileValidatorVersion   string
}

func Validate(raw []byte, input appinput.Document, definition domainprofile.Definition) (*Result, error) {
	if err := definition.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProfile, err)
	}
	if utf8.RuneCount(raw) > definition.GenerationPolicy.MaxOutputCharacters {
		return nil, fmt.Errorf("%w: output exceeds %d characters", ErrProfile, definition.GenerationPolicy.MaxOutputCharacters)
	}
	content, err := ParseTypedContent(raw)
	if err != nil {
		return nil, err
	}
	if err := validateReferences(content, input); err != nil {
		return nil, err
	}
	if err := validateProfilePolicy(content, input, definition); err != nil {
		return nil, err
	}
	return &Result{
		Content: content.Clone(), SchemaValidatorVersion: SchemaValidatorVersion,
		ReferenceValidatorVersion: ReferenceValidatorVersion, ProfileValidatorVersion: ProfileValidatorVersion,
	}, nil
}

// ParseTypedContent applies only the frozen JSON shape and Content contract.
// Reference, Profile and safety checks are intentionally excluded so release
// evaluation can retain a structurally valid Candidate as negative quality
// evidence instead of replacing it with a more favorable generation.
func ParseTypedContent(raw []byte) (output.Content, error) {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '{' || !utf8.Valid(raw) {
		return output.Content{}, schemaViolation(SchemaViolationObjectRequired, "output must be one UTF-8 JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var content output.Content
	if err := decoder.Decode(&content); err != nil {
		return output.Content{}, schemaViolation(classifyDecodeViolation(err), err.Error())
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != nil && !errors.Is(err, io.EOF) {
		return output.Content{}, schemaViolation(SchemaViolationTrailingContent, "trailing content: "+err.Error())
	} else if err == nil {
		return output.Content{}, schemaViolation(SchemaViolationTrailingContent, "trailing content")
	}
	if err := content.Validate(); err != nil {
		return output.Content{}, schemaViolation(SchemaViolationContentContract, err.Error())
	}
	return content.Clone(), nil
}

func classifyDecodeViolation(err error) SchemaViolation {
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return SchemaViolationJSONSyntax
	}
	var syntax *json.SyntaxError
	if errors.As(err, &syntax) {
		return SchemaViolationJSONSyntax
	}
	var fieldType *json.UnmarshalTypeError
	if errors.As(err, &fieldType) {
		return SchemaViolationFieldType
	}
	if strings.HasPrefix(err.Error(), "json: unknown field ") {
		return SchemaViolationUnknownField
	}
	return SchemaViolationDecode
}

func validateReferences(content output.Content, input appinput.Document) error {
	dimensionRefs := make(map[string]struct{}, len(input.Facts.Dimensions))
	for _, dimension := range input.Facts.Dimensions {
		dimensionRefs[dimension.Ref] = struct{}{}
	}
	suggestionRefs := make(map[string]struct{}, len(input.Facts.StandardSuggestions))
	for _, suggestion := range input.Facts.StandardSuggestions {
		suggestionRefs[suggestion.Ref] = struct{}{}
	}
	resolve := func(ref output.EvidenceRef) bool {
		switch ref.Kind {
		case output.EvidenceKindDimension:
			_, ok := dimensionRefs[ref.Ref]
			return ok
		case output.EvidenceKindStandardSuggestion:
			_, ok := suggestionRefs[ref.Ref]
			return ok
		case output.EvidenceKindOverallResult:
			return ref.Ref == "overall_result"
		case output.EvidenceKindModelResult:
			return ref.Ref == "model_result" && input.Facts.ModelResult != nil
		default:
			return false
		}
	}
	for index, insight := range content.IntegratedInsights {
		for _, ref := range insight.EvidenceRefs {
			if !resolve(ref) {
				return fmt.Errorf("%w: insight[%d] ref %s/%s does not resolve", ErrReference, index, ref.Kind, ref.Ref)
			}
		}
	}
	for index, suggestion := range content.Suggestions {
		for _, ref := range suggestion.EvidenceRefs {
			if !resolve(ref) {
				return fmt.Errorf("%w: suggestion[%d] ref %s/%s does not resolve", ErrReference, index, ref.Kind, ref.Ref)
			}
		}
		for _, ref := range suggestion.SourceSuggestionRefs {
			if _, ok := suggestionRefs[ref]; !ok {
				return fmt.Errorf("%w: suggestion[%d] source ref %s does not resolve", ErrReference, index, ref)
			}
		}
	}
	return nil
}

func validateProfilePolicy(content output.Content, input appinput.Document, definition domainprofile.Definition) error {
	if len(content.IntegratedInsights) < definition.InsightPolicy.MinItems || len(content.IntegratedInsights) > definition.InsightPolicy.MaxItems {
		return fmt.Errorf("%w: insight count is outside published policy", ErrProfile)
	}
	allowedKinds := make(map[output.InsightKind]struct{}, len(definition.InsightPolicy.AllowedKinds))
	for _, kind := range definition.InsightPolicy.AllowedKinds {
		allowedKinds[kind] = struct{}{}
	}
	parents := make(map[string]string, len(input.Facts.Dimensions))
	for _, dimension := range input.Facts.Dimensions {
		if dimension.ParentRef != nil {
			parents[dimension.Ref] = *dimension.ParentRef
		}
	}
	for index, insight := range content.IntegratedInsights {
		if _, ok := allowedKinds[insight.Kind]; !ok {
			return fmt.Errorf("%w: insight[%d] kind is not allowed", ErrProfile, index)
		}
		dimensions := make(map[string]struct{})
		for _, ref := range insight.EvidenceRefs {
			if ref.Kind == output.EvidenceKindDimension {
				dimensions[ref.Ref] = struct{}{}
			}
		}
		if len(dimensions) < definition.InsightPolicy.MinDimensionRefsPerItem || len(dimensions) > definition.InsightPolicy.MaxDimensionRefsPerItem {
			return fmt.Errorf("%w: insight[%d] distinct dimension count is outside policy", ErrProfile, index)
		}
		if !definition.InputPolicy.HierarchyPolicy.AllowParentChildInSameInsight {
			for dimension, parent := range parents {
				_, hasDimension := dimensions[dimension]
				_, hasParent := dimensions[parent]
				if hasDimension && hasParent {
					return fmt.Errorf("%w: insight[%d] combines parent and child dimensions", ErrProfile, index)
				}
			}
		}
	}

	if len(content.Suggestions) < definition.SuggestionPolicy.MinItems || len(content.Suggestions) > definition.SuggestionPolicy.MaxItems {
		return fmt.Errorf("%w: suggestion count is outside published policy", ErrProfile)
	}
	allowedOrigins := make(map[output.SuggestionOrigin]struct{}, len(definition.SuggestionPolicy.AllowedOrigins))
	for _, origin := range definition.SuggestionPolicy.AllowedOrigins {
		allowedOrigins[origin] = struct{}{}
	}
	allowedCategories := make(map[string]struct{}, len(definition.SuggestionPolicy.AllowedCategories))
	for _, category := range definition.SuggestionPolicy.AllowedCategories {
		allowedCategories[category] = struct{}{}
	}
	for index, suggestion := range content.Suggestions {
		if _, ok := allowedOrigins[suggestion.Origin]; !ok {
			return fmt.Errorf("%w: suggestion[%d] origin is not allowed", ErrProfile, index)
		}
		if _, ok := allowedCategories[suggestion.Category]; !ok {
			return fmt.Errorf("%w: suggestion[%d] category is not allowed", ErrProfile, index)
		}
		if len(suggestion.Actions) > definition.SuggestionPolicy.MaxActionsPerItem {
			return fmt.Errorf("%w: suggestion[%d] has too many actions", ErrProfile, index)
		}
		if definition.SuggestionPolicy.RequireEvidenceRefs && len(suggestion.EvidenceRefs) == 0 {
			return fmt.Errorf("%w: suggestion[%d] requires evidence", ErrProfile, index)
		}
		if suggestion.Origin == output.SuggestionOriginStandardDerived && definition.SuggestionPolicy.RequireStandardRefsForStandardDerived && len(suggestion.SourceSuggestionRefs) == 0 {
			return fmt.Errorf("%w: suggestion[%d] requires a standard source", ErrProfile, index)
		}
	}
	return nil
}
