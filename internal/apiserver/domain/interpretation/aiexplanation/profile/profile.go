// Package profile owns versioned, published AI explanation policy releases.
package profile

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/output"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
	StatusDisabled  Status = "disabled"
)

func (s Status) IsValid() bool {
	switch s {
	case StatusDraft, StatusPublished, StatusDisabled:
		return true
	default:
		return false
	}
}

type Selector struct {
	Audience     policy.Audience           `json:"audience" bson:"audience"`
	ModelKind    modelcatalog.Kind         `json:"model_kind" bson:"model_kind"`
	DecisionKind modelcatalog.DecisionKind `json:"decision_kind" bson:"decision_kind"`
	ModelCode    *string                   `json:"model_code" bson:"model_code"`
	ModelVersion *string                   `json:"model_version" bson:"model_version"`
}

func (s Selector) Validate() error {
	if err := aiexplanation.ValidateAudience(s.Audience); err != nil {
		return err
	}
	if s.Audience != policy.AudienceParticipant {
		return fmt.Errorf("AI explanation Profile v1 only supports participant audience")
	}
	if s.ModelKind != modelcatalog.KindScale {
		return fmt.Errorf("AI explanation Profile v1 only supports scale model kind")
	}
	if s.DecisionKind != modelcatalog.DecisionKindScoreRange {
		return fmt.Errorf("AI explanation Profile v1 only supports score_range decision kind")
	}
	if s.ModelVersion != nil && s.ModelCode == nil {
		return fmt.Errorf("AI explanation profile model version requires model code")
	}
	if s.ModelCode != nil && strings.TrimSpace(*s.ModelCode) == "" {
		return fmt.Errorf("AI explanation profile model code is invalid")
	}
	if s.ModelVersion != nil {
		if err := aiexplanation.ValidateVersion(*s.ModelVersion); err != nil {
			return err
		}
	}
	return nil
}

func (s Selector) Specificity() int {
	if s.ModelCode == nil {
		return 0
	}
	if s.ModelVersion == nil {
		return 1
	}
	return 2
}

func (s Selector) Matches(query ResolveQuery) bool {
	if s.Audience != query.Audience || s.ModelKind != query.ModelKind || s.DecisionKind != query.DecisionKind {
		return false
	}
	if s.ModelCode != nil && *s.ModelCode != query.ModelCode {
		return false
	}
	return s.ModelVersion == nil || *s.ModelVersion == query.ModelVersion
}

type EligibilityPolicy struct {
	MinEligibleDimensions  int      `json:"min_eligible_dimensions" bson:"min_eligible_dimensions"`
	EligibleDimensionCodes []string `json:"eligible_dimension_codes" bson:"eligible_dimension_codes"`
	ExcludedDimensionCodes []string `json:"excluded_dimension_codes" bson:"excluded_dimension_codes"`
	MaxInputDimensions     int      `json:"max_input_dimensions" bson:"max_input_dimensions"`
	OnDimensionOverflow    string   `json:"on_dimension_overflow" bson:"on_dimension_overflow"`
}

type HierarchyPolicy struct {
	AllowParentChildInSameInsight bool `json:"allow_parent_child_in_same_insight" bson:"allow_parent_child_in_same_insight"`
}

type InputPolicy struct {
	ContextScope       string          `json:"context_scope" bson:"context_scope"`
	IncludeNormContext bool            `json:"include_norm_context" bson:"include_norm_context"`
	IncludeModelResult bool            `json:"include_model_result" bson:"include_model_result"`
	AllowedFocusAreas  []string        `json:"allowed_focus_areas" bson:"allowed_focus_areas"`
	HierarchyPolicy    HierarchyPolicy `json:"hierarchy_policy" bson:"hierarchy_policy"`
}

type InsightPolicy struct {
	AllowedKinds            []output.InsightKind `json:"allowed_kinds" bson:"allowed_kinds"`
	MinItems                int                  `json:"min_items" bson:"min_items"`
	MaxItems                int                  `json:"max_items" bson:"max_items"`
	MinDimensionRefsPerItem int                  `json:"min_dimension_refs_per_item" bson:"min_dimension_refs_per_item"`
	MaxDimensionRefsPerItem int                  `json:"max_dimension_refs_per_item" bson:"max_dimension_refs_per_item"`
	AllowCausalClaims       bool                 `json:"allow_causal_claims" bson:"allow_causal_claims"`
}

type SuggestionPolicy struct {
	AllowedOrigins                        []output.SuggestionOrigin `json:"allowed_origins" bson:"allowed_origins"`
	AllowedCategories                     []string                  `json:"allowed_categories" bson:"allowed_categories"`
	MinItems                              int                       `json:"min_items" bson:"min_items"`
	MaxItems                              int                       `json:"max_items" bson:"max_items"`
	MaxActionsPerItem                     int                       `json:"max_actions_per_item" bson:"max_actions_per_item"`
	RequireEvidenceRefs                   bool                      `json:"require_evidence_refs" bson:"require_evidence_refs"`
	RequireStandardRefsForStandardDerived bool                      `json:"require_standard_refs_for_standard_derived" bson:"require_standard_refs_for_standard_derived"`
}

type SafetyPolicy struct {
	PolicyVersion     string   `json:"policy_version" bson:"policy_version"`
	ForbiddenClaims   []string `json:"forbidden_claims" bson:"forbidden_claims"`
	DisclaimerVersion string   `json:"disclaimer_version" bson:"disclaimer_version"`
}

type GenerationPolicy struct {
	PromptTemplateID    string `json:"prompt_template_id" bson:"prompt_template_id"`
	PromptVersion       string `json:"prompt_version" bson:"prompt_version"`
	ProviderRoute       string `json:"provider_route" bson:"provider_route"`
	InputSchemaVersion  string `json:"input_schema_version" bson:"input_schema_version"`
	OutputSchemaVersion string `json:"output_schema_version" bson:"output_schema_version"`
	MaxOutputCharacters int    `json:"max_output_characters" bson:"max_output_characters"`
}

// Definition is the policy content covered by the Profile fingerprint. Status
// and fingerprint are lifecycle/envelope fields and are intentionally excluded.
type Definition struct {
	SchemaVersion    string            `json:"schema_version" bson:"schema_version"`
	ProfileID        string            `json:"profile_id" bson:"profile_id"`
	Version          string            `json:"version" bson:"version"`
	Selector         Selector          `json:"selector" bson:"selector"`
	Eligibility      EligibilityPolicy `json:"eligibility" bson:"eligibility"`
	InputPolicy      InputPolicy       `json:"input_policy" bson:"input_policy"`
	InsightPolicy    InsightPolicy     `json:"insight_policy" bson:"insight_policy"`
	SuggestionPolicy SuggestionPolicy  `json:"suggestion_policy" bson:"suggestion_policy"`
	SafetyPolicy     SafetyPolicy      `json:"safety_policy" bson:"safety_policy"`
	GenerationPolicy GenerationPolicy  `json:"generation_policy" bson:"generation_policy"`
}

func (d Definition) Validate() error {
	if d.SchemaVersion != aiexplanation.ProfileSchemaVersionV1 || strings.TrimSpace(d.ProfileID) == "" {
		return fmt.Errorf("AI explanation profile schema and id are required")
	}
	if err := aiexplanation.ValidateVersion(d.Version); err != nil {
		return err
	}
	if err := d.Selector.Validate(); err != nil {
		return err
	}
	if err := validateEligibility(d.Eligibility); err != nil {
		return err
	}
	if err := validateInputPolicy(d.InputPolicy); err != nil {
		return err
	}
	if err := validateInsightPolicy(d.InsightPolicy); err != nil {
		return err
	}
	if err := validateSuggestionPolicy(d.SuggestionPolicy); err != nil {
		return err
	}
	if err := validateSafetyPolicy(d.SafetyPolicy); err != nil {
		return err
	}
	return validateGenerationPolicy(d.GenerationPolicy)
}

func (d Definition) Fingerprint() (aiexplanation.Fingerprint, error) {
	if err := d.Validate(); err != nil {
		return "", err
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return "", fmt.Errorf("marshal AI explanation profile definition: %w", err)
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return "", fmt.Errorf("normalize AI explanation profile definition: %w", err)
	}
	canonical, err := json.Marshal(generic)
	if err != nil {
		return "", fmt.Errorf("canonicalize AI explanation profile definition: %w", err)
	}
	return aiexplanation.NewFingerprint(canonical), nil
}

type AIExplanationProfile struct {
	id                     meta.ID
	definition             Definition
	fingerprint            aiexplanation.Fingerprint
	status                 Status
	createdAt              time.Time
	createdBy              string
	createdReason          string
	updatedAt              time.Time
	publishedAt            *time.Time
	publishedBy            string
	publishedReason        string
	publishedEvidenceRunID meta.ID
	disabledAt             *time.Time
	disabledBy             string
	disabledReason         string
}

func NewDraft(id meta.ID, definition Definition, createdAt time.Time) (*AIExplanationProfile, error) {
	if id.IsZero() || createdAt.IsZero() {
		return nil, fmt.Errorf("AI explanation profile id and created time are required")
	}
	fingerprint, err := definition.Fingerprint()
	if err != nil {
		return nil, err
	}
	return &AIExplanationProfile{
		id: id, definition: cloneDefinition(definition), fingerprint: fingerprint,
		status: StatusDraft, createdAt: createdAt, updatedAt: createdAt,
	}, nil
}

// NewDraftForRelease creates an operator-authored draft and retains the
// creation audit alongside later publish/disable evidence. Synthetic
// evaluation fixtures continue to use NewDraft and are never persisted as
// release assets.
func NewDraftForRelease(id meta.ID, definition Definition, actor, reason string, createdAt time.Time) (*AIExplanationProfile, error) {
	actor = strings.TrimSpace(actor)
	reason = strings.TrimSpace(reason)
	if actor == "" || reason == "" || len(reason) > 1000 {
		return nil, fmt.Errorf("AI explanation profile draft creation audit is required")
	}
	profile, err := NewDraft(id, definition, createdAt)
	if err != nil {
		return nil, err
	}
	profile.createdBy = actor
	profile.createdReason = reason
	return profile, nil
}

type PersistedInput struct {
	ID                     meta.ID
	Definition             Definition
	Fingerprint            aiexplanation.Fingerprint
	Status                 Status
	CreatedAt              time.Time
	CreatedBy              string
	CreatedReason          string
	UpdatedAt              time.Time
	PublishedAt            *time.Time
	PublishedBy            string
	PublishedReason        string
	PublishedEvidenceRunID meta.ID
	DisabledAt             *time.Time
	DisabledBy             string
	DisabledReason         string
}

func Restore(input PersistedInput) (*AIExplanationProfile, error) {
	profile, err := NewDraft(input.ID, input.Definition, input.CreatedAt)
	if err != nil {
		return nil, err
	}
	if input.Fingerprint != profile.fingerprint || !input.Status.IsValid() || input.UpdatedAt.IsZero() || input.UpdatedAt.Before(input.CreatedAt) {
		return nil, fmt.Errorf("AI explanation profile persistence state is invalid")
	}
	if err := validateLifecycle(input); err != nil {
		return nil, err
	}
	profile.status = input.Status
	profile.createdBy = input.CreatedBy
	profile.createdReason = input.CreatedReason
	profile.updatedAt = input.UpdatedAt
	profile.publishedAt = copyTimePtr(input.PublishedAt)
	profile.publishedBy = input.PublishedBy
	profile.publishedReason = input.PublishedReason
	profile.publishedEvidenceRunID = input.PublishedEvidenceRunID
	profile.disabledAt = copyTimePtr(input.DisabledAt)
	profile.disabledBy = input.DisabledBy
	profile.disabledReason = input.DisabledReason
	return profile, nil
}

func (p *AIExplanationProfile) Publish(evidenceRunID meta.ID, actor, reason string, at time.Time) error {
	if p == nil || p.status != StatusDraft || evidenceRunID.IsZero() || strings.TrimSpace(actor) == "" || strings.TrimSpace(reason) == "" || at.IsZero() || at.Before(p.createdAt) {
		return fmt.Errorf("draft AI explanation profile and publish audit are required")
	}
	p.status = StatusPublished
	p.updatedAt = at
	p.publishedAt = copyTime(at)
	p.publishedBy = strings.TrimSpace(actor)
	p.publishedReason = strings.TrimSpace(reason)
	p.publishedEvidenceRunID = evidenceRunID
	return nil
}

func (p *AIExplanationProfile) Disable(actor, reason string, at time.Time) error {
	if p == nil || p.status != StatusPublished || strings.TrimSpace(actor) == "" || strings.TrimSpace(reason) == "" || at.IsZero() || p.publishedAt == nil || at.Before(*p.publishedAt) {
		return fmt.Errorf("published AI explanation profile and disable audit are required")
	}
	p.status = StatusDisabled
	p.updatedAt = at
	p.disabledAt = copyTime(at)
	p.disabledBy = strings.TrimSpace(actor)
	p.disabledReason = strings.TrimSpace(reason)
	return nil
}

func (p *AIExplanationProfile) ID() meta.ID                            { return p.id }
func (p *AIExplanationProfile) ProfileID() string                      { return p.definition.ProfileID }
func (p *AIExplanationProfile) Version() string                        { return p.definition.Version }
func (p *AIExplanationProfile) Selector() Selector                     { return cloneSelector(p.definition.Selector) }
func (p *AIExplanationProfile) Definition() Definition                 { return cloneDefinition(p.definition) }
func (p *AIExplanationProfile) Fingerprint() aiexplanation.Fingerprint { return p.fingerprint }
func (p *AIExplanationProfile) Status() Status                         { return p.status }
func (p *AIExplanationProfile) CreatedAt() time.Time                   { return p.createdAt }
func (p *AIExplanationProfile) CreatedBy() string                      { return p.createdBy }
func (p *AIExplanationProfile) CreatedReason() string                  { return p.createdReason }
func (p *AIExplanationProfile) UpdatedAt() time.Time                   { return p.updatedAt }
func (p *AIExplanationProfile) PublishedAt() *time.Time                { return copyTimePtr(p.publishedAt) }
func (p *AIExplanationProfile) PublishedBy() string                    { return p.publishedBy }
func (p *AIExplanationProfile) PublishedReason() string                { return p.publishedReason }
func (p *AIExplanationProfile) PublishedEvidenceRunID() meta.ID        { return p.publishedEvidenceRunID }
func (p *AIExplanationProfile) DisabledAt() *time.Time                 { return copyTimePtr(p.disabledAt) }
func (p *AIExplanationProfile) DisabledBy() string                     { return p.disabledBy }
func (p *AIExplanationProfile) DisabledReason() string                 { return p.disabledReason }

type ResolveQuery struct {
	Audience     policy.Audience
	ModelKind    modelcatalog.Kind
	DecisionKind modelcatalog.DecisionKind
	ModelCode    string
	ModelVersion string
}

func Resolve(ctx context.Context, repository Repository, query ResolveQuery) (*AIExplanationProfile, error) {
	if repository == nil {
		return nil, fmt.Errorf("AI explanation profile repository is required")
	}
	base := Selector{Audience: query.Audience, ModelKind: query.ModelKind, DecisionKind: query.DecisionKind}
	if err := base.Validate(); err != nil {
		return nil, err
	}
	candidates, err := repository.ListPublishedByBaseSelector(ctx, query.Audience, query.ModelKind, query.DecisionKind)
	if err != nil {
		return nil, err
	}
	matches := map[int][]*AIExplanationProfile{}
	for _, candidate := range candidates {
		if candidate == nil || candidate.Status() != StatusPublished || !candidate.Selector().Matches(query) {
			continue
		}
		specificity := candidate.Selector().Specificity()
		matches[specificity] = append(matches[specificity], candidate)
	}
	for specificity := 2; specificity >= 0; specificity-- {
		if len(matches[specificity]) > 1 {
			return nil, ErrAmbiguousSelector
		}
		if len(matches[specificity]) == 1 {
			return matches[specificity][0], nil
		}
	}
	return nil, ErrNotFound
}

func validateEligibility(value EligibilityPolicy) error {
	if value.MinEligibleDimensions < 2 || value.MaxInputDimensions < value.MinEligibleDimensions || value.MaxInputDimensions > 50 || value.OnDimensionOverflow != "reject" {
		return fmt.Errorf("AI explanation profile eligibility is invalid")
	}
	eligible, err := uniqueStrings(value.EligibleDimensionCodes, 50, false)
	if err != nil {
		return err
	}
	excluded, err := uniqueStrings(value.ExcludedDimensionCodes, 50, false)
	if err != nil {
		return err
	}
	for code := range eligible {
		if _, exists := excluded[code]; exists {
			return fmt.Errorf("AI explanation eligible and excluded dimensions overlap")
		}
	}
	return nil
}

func validateInputPolicy(value InputPolicy) error {
	if value.ContextScope != "current_assessment_only" || len(value.AllowedFocusAreas) > 20 {
		return fmt.Errorf("AI explanation input policy is invalid")
	}
	_, err := uniqueStrings(value.AllowedFocusAreas, 20, true)
	return err
}

func validateInsightPolicy(value InsightPolicy) error {
	if len(value.AllowedKinds) == 0 || value.MinItems < 1 || value.MaxItems < value.MinItems || value.MaxItems > 8 || value.MinDimensionRefsPerItem < 2 || value.MaxDimensionRefsPerItem < value.MinDimensionRefsPerItem || value.MaxDimensionRefsPerItem > 6 || value.AllowCausalClaims {
		return fmt.Errorf("AI explanation insight policy is invalid")
	}
	seen := map[output.InsightKind]struct{}{}
	for _, kind := range value.AllowedKinds {
		if !kind.IsValid() {
			return fmt.Errorf("AI explanation allowed insight kind is invalid")
		}
		if _, exists := seen[kind]; exists {
			return fmt.Errorf("AI explanation allowed insight kind is duplicated")
		}
		seen[kind] = struct{}{}
	}
	return nil
}

func validateSuggestionPolicy(value SuggestionPolicy) error {
	if len(value.AllowedOrigins) == 0 || len(value.AllowedCategories) == 0 || value.MinItems < 1 || value.MaxItems < value.MinItems || value.MaxItems > 8 || value.MaxActionsPerItem < 1 || value.MaxActionsPerItem > 5 || !value.RequireEvidenceRefs || !value.RequireStandardRefsForStandardDerived {
		return fmt.Errorf("AI explanation suggestion policy is invalid")
	}
	seenOrigins := map[output.SuggestionOrigin]struct{}{}
	for _, origin := range value.AllowedOrigins {
		if origin != output.SuggestionOriginStandardDerived && origin != output.SuggestionOriginGeneratedLowRisk {
			return fmt.Errorf("AI explanation allowed suggestion origin is invalid")
		}
		if _, exists := seenOrigins[origin]; exists {
			return fmt.Errorf("AI explanation allowed suggestion origin is duplicated")
		}
		seenOrigins[origin] = struct{}{}
	}
	_, err := uniqueStrings(value.AllowedCategories, 0, true)
	return err
}

func validateSafetyPolicy(value SafetyPolicy) error {
	if err := aiexplanation.ValidateVersion(value.PolicyVersion); err != nil {
		return err
	}
	if err := aiexplanation.ValidateVersion(value.DisclaimerVersion); err != nil {
		return err
	}
	required := []string{"diagnosis", "causality", "medication", "treatment_plan", "risk_reclassification", "identity_inference", "deterministic_future_prediction"}
	if len(value.ForbiddenClaims) != len(required) {
		return fmt.Errorf("AI explanation safety policy must retain every forbidden claim")
	}
	actual := append([]string(nil), value.ForbiddenClaims...)
	sort.Strings(actual)
	sort.Strings(required)
	for index := range required {
		if actual[index] != required[index] {
			return fmt.Errorf("AI explanation safety policy must retain every forbidden claim")
		}
	}
	return nil
}

func validateGenerationPolicy(value GenerationPolicy) error {
	if strings.TrimSpace(value.PromptTemplateID) == "" || value.InputSchemaVersion != aiexplanation.InputSchemaVersionV1 || value.OutputSchemaVersion != aiexplanation.OutputSchemaVersionV1 || value.MaxOutputCharacters < 512 || value.MaxOutputCharacters > 20000 {
		return fmt.Errorf("AI explanation generation policy is invalid")
	}
	if err := aiexplanation.ValidateVersion(value.PromptVersion); err != nil {
		return err
	}
	return aiexplanation.ValidateRouteKey(value.ProviderRoute)
}

func uniqueStrings(values []string, maxItems int, requireCode bool) (map[string]struct{}, error) {
	if maxItems > 0 && len(values) > maxItems {
		return nil, fmt.Errorf("AI explanation profile code list exceeds its limit")
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 255 {
			return nil, fmt.Errorf("AI explanation profile code is invalid")
		}
		if requireCode {
			if err := aiexplanation.ValidateRouteKey(value); err != nil {
				return nil, fmt.Errorf("AI explanation profile code is invalid: %w", err)
			}
		}
		if _, exists := seen[value]; exists {
			return nil, fmt.Errorf("AI explanation profile code is duplicated")
		}
		seen[value] = struct{}{}
	}
	return seen, nil
}

func validateLifecycle(input PersistedInput) error {
	createdBy := strings.TrimSpace(input.CreatedBy)
	createdReason := strings.TrimSpace(input.CreatedReason)
	if (createdBy == "") != (createdReason == "") || len(createdReason) > 1000 {
		return fmt.Errorf("AI explanation profile creation audit is invalid")
	}
	published := input.PublishedAt != nil && !input.PublishedAt.IsZero() && !input.PublishedAt.Before(input.CreatedAt) && strings.TrimSpace(input.PublishedBy) != "" && strings.TrimSpace(input.PublishedReason) != "" && !input.PublishedEvidenceRunID.IsZero()
	disabled := input.DisabledAt != nil && !input.DisabledAt.IsZero() && strings.TrimSpace(input.DisabledBy) != "" && strings.TrimSpace(input.DisabledReason) != ""
	switch input.Status {
	case StatusDraft:
		if input.PublishedAt != nil || input.PublishedBy != "" || input.PublishedReason != "" || !input.PublishedEvidenceRunID.IsZero() || input.DisabledAt != nil || input.DisabledBy != "" || input.DisabledReason != "" {
			return fmt.Errorf("draft AI explanation profile contains lifecycle audit")
		}
	case StatusPublished:
		if !published || disabled || input.DisabledAt != nil || input.DisabledBy != "" || input.DisabledReason != "" {
			return fmt.Errorf("published AI explanation profile lifecycle audit is invalid")
		}
	case StatusDisabled:
		if !published || !disabled || input.DisabledAt.Before(*input.PublishedAt) {
			return fmt.Errorf("disabled AI explanation profile lifecycle audit is invalid")
		}
	}
	return nil
}

func cloneDefinition(value Definition) Definition {
	cloned := value
	cloned.Selector = cloneSelector(value.Selector)
	cloned.Eligibility.EligibleDimensionCodes = append([]string(nil), value.Eligibility.EligibleDimensionCodes...)
	cloned.Eligibility.ExcludedDimensionCodes = append([]string(nil), value.Eligibility.ExcludedDimensionCodes...)
	cloned.InputPolicy.AllowedFocusAreas = append([]string(nil), value.InputPolicy.AllowedFocusAreas...)
	cloned.InsightPolicy.AllowedKinds = append([]output.InsightKind(nil), value.InsightPolicy.AllowedKinds...)
	cloned.SuggestionPolicy.AllowedOrigins = append([]output.SuggestionOrigin(nil), value.SuggestionPolicy.AllowedOrigins...)
	cloned.SuggestionPolicy.AllowedCategories = append([]string(nil), value.SuggestionPolicy.AllowedCategories...)
	cloned.SafetyPolicy.ForbiddenClaims = append([]string(nil), value.SafetyPolicy.ForbiddenClaims...)
	return cloned
}

func cloneSelector(value Selector) Selector {
	cloned := value
	if value.ModelCode != nil {
		copy := *value.ModelCode
		cloned.ModelCode = &copy
	}
	if value.ModelVersion != nil {
		copy := *value.ModelVersion
		cloned.ModelVersion = &copy
	}
	return cloned
}

func copyTime(value time.Time) *time.Time {
	copy := value
	return &copy
}

func copyTimePtr(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	return copyTime(*value)
}
