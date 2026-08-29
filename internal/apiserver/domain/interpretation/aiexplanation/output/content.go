// Package output owns the provider-independent AIExplanationOutput v1 value
// object. Profile-specific and source-reference validation is performed by the
// application validation pipeline before Artifact construction.
package output

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
)

var (
	dimensionRefPattern  = regexp.MustCompile(`^dimension:[^\s]+$`)
	suggestionRefPattern = regexp.MustCompile(`^suggestion:[^\s]+$`)
)

type EvidenceKind string

const (
	EvidenceKindDimension          EvidenceKind = "dimension"
	EvidenceKindOverallResult      EvidenceKind = "overall_result"
	EvidenceKindModelResult        EvidenceKind = "model_result"
	EvidenceKindStandardSuggestion EvidenceKind = "standard_suggestion"
)

type EvidenceRef struct {
	Kind EvidenceKind `json:"kind" bson:"kind"`
	Ref  string       `json:"ref" bson:"ref"`
}

func (r EvidenceRef) Validate() error {
	if utf8.RuneCountInString(r.Ref) > 266 {
		return fmt.Errorf("AI explanation evidence ref is too long")
	}
	switch r.Kind {
	case EvidenceKindDimension:
		if !dimensionRefPattern.MatchString(r.Ref) {
			return fmt.Errorf("AI explanation dimension evidence ref is invalid")
		}
	case EvidenceKindStandardSuggestion:
		if !suggestionRefPattern.MatchString(r.Ref) {
			return fmt.Errorf("AI explanation suggestion evidence ref is invalid")
		}
	case EvidenceKindOverallResult:
		if r.Ref != "overall_result" {
			return fmt.Errorf("AI explanation overall result evidence ref is invalid")
		}
	case EvidenceKindModelResult:
		if r.Ref != "model_result" {
			return fmt.Errorf("AI explanation model result evidence ref is invalid")
		}
	default:
		return fmt.Errorf("AI explanation evidence kind is invalid")
	}
	return nil
}

type InsightKind string

const (
	InsightKindReinforcingPattern InsightKind = "reinforcing_pattern"
	InsightKindContrastingPattern InsightKind = "contrasting_pattern"
	InsightKindCombinedStrength   InsightKind = "combined_strength"
	InsightKindCombinedAttention  InsightKind = "combined_attention"
	InsightKindContextDependent   InsightKind = "context_dependent_pattern"
)

func (k InsightKind) IsValid() bool {
	switch k {
	case InsightKindReinforcingPattern, InsightKindContrastingPattern, InsightKindCombinedStrength, InsightKindCombinedAttention, InsightKindContextDependent:
		return true
	default:
		return false
	}
}

type IntegratedInsight struct {
	Kind         InsightKind   `json:"kind" bson:"kind"`
	Title        string        `json:"title" bson:"title"`
	Content      string        `json:"content" bson:"content"`
	WhyItMatters string        `json:"why_it_matters" bson:"why_it_matters"`
	EvidenceRefs []EvidenceRef `json:"evidence_refs" bson:"evidence_refs"`
}

func (i IntegratedInsight) Validate() error {
	if !i.Kind.IsValid() || !validPlainText(i.Title, 200, true) || !validPlainText(i.Content, 2000, true) || !validPlainText(i.WhyItMatters, 1000, true) {
		return fmt.Errorf("AI explanation integrated insight content is invalid")
	}
	if len(i.EvidenceRefs) < 2 || len(i.EvidenceRefs) > 6 {
		return fmt.Errorf("AI explanation integrated insight evidence count is invalid")
	}
	return validateUniqueEvidence(i.EvidenceRefs)
}

type SuggestionOrigin string

const (
	SuggestionOriginStandardDerived  SuggestionOrigin = "standard_derived"
	SuggestionOriginGeneratedLowRisk SuggestionOrigin = "generated_low_risk"
)

type Suggestion struct {
	Origin               SuggestionOrigin `json:"origin" bson:"origin"`
	Category             string           `json:"category" bson:"category"`
	Title                string           `json:"title" bson:"title"`
	Goal                 string           `json:"goal" bson:"goal"`
	Actions              []string         `json:"actions" bson:"actions"`
	Rationale            string           `json:"rationale" bson:"rationale"`
	EvidenceRefs         []EvidenceRef    `json:"evidence_refs" bson:"evidence_refs"`
	SourceSuggestionRefs []string         `json:"source_suggestion_refs" bson:"source_suggestion_refs"`
	Caution              string           `json:"caution" bson:"caution"`
}

func (s Suggestion) Validate() error {
	if s.Origin != SuggestionOriginStandardDerived && s.Origin != SuggestionOriginGeneratedLowRisk {
		return fmt.Errorf("AI explanation suggestion origin is invalid")
	}
	if !routeLikeCode(s.Category) || !validPlainText(s.Title, 200, true) || !validPlainText(s.Goal, 1000, true) || !validPlainText(s.Rationale, 1000, true) || !validPlainText(s.Caution, 1000, false) {
		return fmt.Errorf("AI explanation suggestion content is invalid")
	}
	if len(s.Actions) < 1 || len(s.Actions) > 5 {
		return fmt.Errorf("AI explanation suggestion action count is invalid")
	}
	for _, action := range s.Actions {
		if !validPlainText(action, 500, true) {
			return fmt.Errorf("AI explanation suggestion action is invalid")
		}
	}
	if len(s.EvidenceRefs) < 1 || len(s.EvidenceRefs) > 6 {
		return fmt.Errorf("AI explanation suggestion evidence count is invalid")
	}
	if err := validateUniqueEvidence(s.EvidenceRefs); err != nil {
		return err
	}
	if len(s.SourceSuggestionRefs) > 6 {
		return fmt.Errorf("AI explanation source suggestion count is invalid")
	}
	seen := make(map[string]struct{}, len(s.SourceSuggestionRefs))
	for _, ref := range s.SourceSuggestionRefs {
		if utf8.RuneCountInString(ref) > 266 || !suggestionRefPattern.MatchString(ref) {
			return fmt.Errorf("AI explanation source suggestion ref is invalid")
		}
		if _, exists := seen[ref]; exists {
			return fmt.Errorf("AI explanation source suggestion ref is duplicated")
		}
		seen[ref] = struct{}{}
	}
	if s.Origin == SuggestionOriginStandardDerived && len(s.SourceSuggestionRefs) == 0 {
		return fmt.Errorf("standard-derived AI explanation suggestion requires a source suggestion")
	}
	return nil
}

type Content struct {
	SchemaVersion      string              `json:"schema_version" bson:"schema_version"`
	Summary            string              `json:"summary" bson:"summary"`
	IntegratedInsights []IntegratedInsight `json:"integrated_insights" bson:"integrated_insights"`
	Suggestions        []Suggestion        `json:"suggestions" bson:"suggestions"`
	Limitations        []string            `json:"limitations" bson:"limitations"`
}

func (c Content) Validate() error {
	if c.SchemaVersion != aiexplanation.OutputSchemaVersionV1 || !validPlainText(c.Summary, 2000, true) {
		return fmt.Errorf("AI explanation output schema or summary is invalid")
	}
	if len(c.IntegratedInsights) < 1 || len(c.IntegratedInsights) > 8 || len(c.Suggestions) < 1 || len(c.Suggestions) > 8 || len(c.Limitations) < 1 || len(c.Limitations) > 5 {
		return fmt.Errorf("AI explanation output item counts are invalid")
	}
	for _, insight := range c.IntegratedInsights {
		if err := insight.Validate(); err != nil {
			return err
		}
	}
	for _, suggestion := range c.Suggestions {
		if err := suggestion.Validate(); err != nil {
			return err
		}
	}
	seenLimitations := make(map[string]struct{}, len(c.Limitations))
	for _, limitation := range c.Limitations {
		if !validPlainText(limitation, 1000, true) {
			return fmt.Errorf("AI explanation limitation is invalid")
		}
		if _, exists := seenLimitations[limitation]; exists {
			return fmt.Errorf("AI explanation limitation is duplicated")
		}
		seenLimitations[limitation] = struct{}{}
	}
	return nil
}

func (c Content) Clone() Content {
	cloned := c
	cloned.IntegratedInsights = cloneSlice(c.IntegratedInsights)
	for index, insight := range c.IntegratedInsights {
		cloned.IntegratedInsights[index].EvidenceRefs = cloneSlice(insight.EvidenceRefs)
	}
	cloned.Suggestions = cloneSlice(c.Suggestions)
	for index, suggestion := range c.Suggestions {
		cloned.Suggestions[index].Actions = cloneSlice(suggestion.Actions)
		cloned.Suggestions[index].EvidenceRefs = cloneSlice(suggestion.EvidenceRefs)
		cloned.Suggestions[index].SourceSuggestionRefs = cloneSlice(suggestion.SourceSuggestionRefs)
	}
	cloned.Limitations = cloneSlice(c.Limitations)
	return cloned
}

// cloneSlice preserves the nil-versus-empty distinction. Both are valid Go
// slices, but AIExplanationOutput v1 requires JSON arrays for every collection
// field. Turning a provider-supplied [] into nil would make the normalized
// evidence marshal as null and drift away from the document that passed the
// frozen output contract.
func cloneSlice[T any](values []T) []T {
	if values == nil {
		return nil
	}
	cloned := make([]T, len(values))
	copy(cloned, values)
	return cloned
}

func validateUniqueEvidence(refs []EvidenceRef) error {
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if err := ref.Validate(); err != nil {
			return err
		}
		key := string(ref.Kind) + "\x00" + ref.Ref
		if _, exists := seen[key]; exists {
			return fmt.Errorf("AI explanation evidence ref is duplicated")
		}
		seen[key] = struct{}{}
	}
	return nil
}

func validPlainText(value string, maxRunes int, required bool) bool {
	if required && value == "" {
		return false
	}
	return utf8.RuneCountInString(value) <= maxRunes && !strings.ContainsAny(value, "<>")
}

func routeLikeCode(value string) bool {
	if value == "" || len(value) > 64 || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, r := range value[1:] {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '.' && r != '_' && r != '-' {
			return false
		}
	}
	return true
}
