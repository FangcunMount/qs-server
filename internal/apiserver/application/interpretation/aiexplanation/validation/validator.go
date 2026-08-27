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
	if len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '{' || !utf8.Valid(raw) {
		return nil, fmt.Errorf("%w: output must be one UTF-8 JSON object", ErrSchema)
	}
	if utf8.RuneCount(raw) > definition.GenerationPolicy.MaxOutputCharacters {
		return nil, fmt.Errorf("%w: output exceeds %d characters", ErrProfile, definition.GenerationPolicy.MaxOutputCharacters)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var content output.Content
	if err := decoder.Decode(&content); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSchema, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: trailing content: %v", ErrSchema, err)
	} else if err == nil {
		return nil, fmt.Errorf("%w: trailing content", ErrSchema)
	}
	if err := content.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSchema, err)
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
