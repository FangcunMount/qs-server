// Package input assembles AIExplanationInput v1 from one validated current
// standard report, its immutable Outcome runtime identity and a published
// Profile. It never reads answers, history or participant attributes.
package input

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	appsource "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/source"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation"
	domaininput "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/input"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/policy"
	domainreport "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

var (
	ErrNotApplicable   = errors.New("AI explanation input is not applicable")
	ErrProfileMismatch = errors.New("AI explanation profile does not match input")
	ErrInvalidInput    = errors.New("AI explanation input is invalid")

	localePattern = regexp.MustCompile(`^[A-Za-z]{2,3}(?:-[A-Za-z0-9]{2,8})*$`)
)

type Request struct {
	Source     *appsource.Current
	Profile    *domainprofile.AIExplanationProfile
	Audience   policy.Audience
	Locale     string
	FocusAreas []string
}

type Result struct {
	Document        Document
	Snapshot        domaininput.Snapshot
	ProviderPayload []byte
}

// Document is the server-side AIExplanationInput v1 envelope. Source and
// Profile are audit facts and are deliberately omitted from ProviderPayload.
type Document struct {
	SchemaVersion string     `json:"schema_version"`
	Source        Source     `json:"source"`
	Profile       ProfileRef `json:"profile"`
	Context       Context    `json:"context"`
	Facts         Facts      `json:"facts"`
}

type Source struct {
	ReportID             string `json:"report_id"`
	OutcomeID            string `json:"outcome_id"`
	ReportType           string `json:"report_type"`
	TemplateVersion      string `json:"report_template_version"`
	ContentSchemaVersion string `json:"content_schema_version"`
	BuilderIdentity      string `json:"builder_identity"`
	GeneratedAt          string `json:"generated_at"`
}

type ProfileRef struct {
	ProfileID          string `json:"profile_id"`
	ProfileVersion     string `json:"profile_version"`
	ProfileFingerprint string `json:"profile_fingerprint"`
}

type Context struct {
	Scope                string   `json:"scope"`
	Audience             string   `json:"audience"`
	Locale               string   `json:"locale"`
	PersonalizationScope string   `json:"personalization_scope"`
	FocusAreas           []string `json:"focus_areas"`
}

type Facts struct {
	Runtime             Runtime              `json:"runtime"`
	Model               Model                `json:"model"`
	OverallResult       OverallResult        `json:"overall_result"`
	Dimensions          []Dimension          `json:"dimensions"`
	StandardSuggestions []StandardSuggestion `json:"standard_suggestions"`
	ModelResult         *ModelResult         `json:"model_result"`
}

type Runtime struct {
	DecisionKind string `json:"decision_kind"`
}

type Model struct {
	Kind      string `json:"kind"`
	Algorithm string `json:"algorithm"`
	Code      string `json:"code"`
	Version   string `json:"version"`
	Title     string `json:"title"`
}

type ScoreValue struct {
	Kind  string   `json:"kind"`
	Value float64  `json:"value"`
	Label string   `json:"label"`
	Max   *float64 `json:"max"`
}

type ResultLevel struct {
	Code     string `json:"code"`
	Label    string `json:"label"`
	Severity string `json:"severity"`
}

type OverallResult struct {
	PrimaryScore       *ScoreValue  `json:"primary_score"`
	Level              *ResultLevel `json:"level"`
	StandardConclusion string       `json:"standard_conclusion"`
}

type NormContext struct {
	ScoreKind    string  `json:"score_kind"`
	Benchmark    float64 `json:"benchmark"`
	TableVersion string  `json:"table_version"`
	FormVariant  string  `json:"form_variant"`
	MinAgeMonths int     `json:"min_age_months"`
	MaxAgeMonths int     `json:"max_age_months"`
	Gender       string  `json:"gender"`
}

type Dimension struct {
	Ref                    string       `json:"ref"`
	Code                   string       `json:"code"`
	Kind                   string       `json:"kind"`
	Name                   string       `json:"name"`
	Role                   string       `json:"role"`
	ParentRef              *string      `json:"parent_ref"`
	HierarchyLevel         int          `json:"hierarchy_level"`
	SortOrder              int          `json:"sort_order"`
	RawScore               ScoreValue   `json:"raw_score"`
	DerivedScores          []ScoreValue `json:"derived_scores"`
	Level                  *ResultLevel `json:"level"`
	NormContext            *NormContext `json:"norm_context"`
	StandardDescription    string       `json:"standard_description"`
	StandardSuggestionRefs []string     `json:"standard_suggestion_refs"`
}

type StandardSuggestion struct {
	Ref           string   `json:"ref"`
	Category      string   `json:"category"`
	Content       string   `json:"content"`
	DimensionRefs []string `json:"dimension_refs"`
}

type ModelResult struct {
	Kind         string  `json:"kind"`
	TypeCode     string  `json:"type_code"`
	TypeName     string  `json:"type_name"`
	OneLiner     string  `json:"one_liner"`
	MatchPercent float64 `json:"match_percent"`
	Commentary   string  `json:"commentary"`
}

type ProviderDocument struct {
	Context Context `json:"context"`
	Facts   Facts   `json:"facts"`
}

func Assemble(request Request) (*Result, error) {
	if request.Source == nil || request.Source.Report == nil || request.Source.Outcome == nil || request.Profile == nil {
		return nil, fmt.Errorf("%w: source and published profile are required", ErrInvalidInput)
	}
	if request.Profile.Status() != domainprofile.StatusPublished {
		return nil, fmt.Errorf("%w: profile is not published", ErrProfileMismatch)
	}
	if request.Audience != policy.AudienceParticipant {
		return nil, fmt.Errorf("%w: v1 audience %s is unsupported", ErrNotApplicable, request.Audience)
	}
	if !localePattern.MatchString(request.Locale) || utf8.RuneCountInString(request.Locale) > 35 {
		return nil, fmt.Errorf("%w: locale is invalid", ErrInvalidInput)
	}
	report := request.Source.Report
	outcome := request.Source.Outcome
	model := outcome.Model()
	decisionKind := outcome.Runtime().DecisionKind
	if model.Kind != modelcatalog.KindScale || decisionKind != modelcatalog.DecisionKindScoreRange {
		return nil, fmt.Errorf("%w: v1 supports participant scale + score_range only", ErrNotApplicable)
	}
	query := domainprofile.ResolveQuery{
		Audience: request.Audience, ModelKind: model.Kind, DecisionKind: decisionKind,
		ModelCode: model.Code, ModelVersion: model.Version,
	}
	if !request.Profile.Selector().Matches(query) {
		return nil, fmt.Errorf("%w: selector does not match current source", ErrProfileMismatch)
	}
	definition := request.Profile.Definition()
	contextValue, err := buildContext(request.Locale, request.FocusAreas, definition.InputPolicy.AllowedFocusAreas, request.Audience)
	if err != nil {
		return nil, err
	}
	facts, err := buildFacts(report.Content(), model, decisionKind, definition)
	if err != nil {
		return nil, err
	}
	document := Document{
		SchemaVersion: aiexplanation.InputSchemaVersionV1,
		Source: Source{
			ReportID: report.ID().String(), OutcomeID: report.OutcomeID().String(), ReportType: report.ReportType().String(),
			TemplateVersion: report.TemplateVersion().String(), ContentSchemaVersion: report.ContentSchemaVersion(),
			BuilderIdentity: report.BuilderIdentity(), GeneratedAt: report.GeneratedAt().UTC().Format("2006-01-02T15:04:05.999999999Z"),
		},
		Profile: ProfileRef{
			ProfileID: request.Profile.ProfileID(), ProfileVersion: request.Profile.Version(), ProfileFingerprint: request.Profile.Fingerprint().String(),
		},
		Context: contextValue,
		Facts:   facts,
	}
	canonicalJSON, err := json.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal input: %v", ErrInvalidInput, err)
	}
	snapshot, err := domaininput.NewSnapshot(canonicalJSON)
	if err != nil {
		return nil, fmt.Errorf("%w: create input snapshot: %v", ErrInvalidInput, err)
	}
	providerPayload, err := json.Marshal(ProviderDocument{Context: document.Context, Facts: document.Facts})
	if err != nil {
		return nil, fmt.Errorf("%w: marshal provider payload: %v", ErrInvalidInput, err)
	}
	return &Result{Document: document, Snapshot: snapshot, ProviderPayload: providerPayload}, nil
}

// Restore rebuilds the provider-safe projection from an immutable Generation
// snapshot. It is used by asynchronous workers so retries never re-read or
// silently change the source report.
func Restore(snapshot domaininput.Snapshot) (*Result, error) {
	if err := snapshot.Validate(); err != nil {
		return nil, fmt.Errorf("%w: invalid frozen snapshot: %v", ErrInvalidInput, err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(snapshot.CanonicalJSON())))
	decoder.DisallowUnknownFields()
	var document Document
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("%w: decode frozen snapshot: %v", ErrInvalidInput, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: frozen snapshot trailing content: %v", ErrInvalidInput, err)
	} else if err == nil {
		return nil, fmt.Errorf("%w: frozen snapshot has trailing content", ErrInvalidInput)
	}
	if err := validateRestoredDocument(document); err != nil {
		return nil, err
	}
	providerPayload, err := json.Marshal(ProviderDocument{Context: document.Context, Facts: document.Facts})
	if err != nil {
		return nil, fmt.Errorf("%w: marshal restored provider payload: %v", ErrInvalidInput, err)
	}
	return &Result{Document: document, Snapshot: snapshot, ProviderPayload: providerPayload}, nil
}

func validateRestoredDocument(document Document) error {
	if document.SchemaVersion != aiexplanation.InputSchemaVersionV1 {
		return fmt.Errorf("%w: frozen input schema is unsupported", ErrInvalidInput)
	}
	reportID, reportErr := meta.ParseID(document.Source.ReportID)
	outcomeID, outcomeErr := meta.ParseID(document.Source.OutcomeID)
	generatedAt, timeErr := time.Parse(time.RFC3339Nano, document.Source.GeneratedAt)
	if reportErr != nil || outcomeErr != nil || reportID.IsZero() || outcomeID.IsZero() || timeErr != nil || generatedAt.IsZero() ||
		document.Source.ReportType != policy.ReportTypeStandard.String() || document.Source.TemplateVersion == "" ||
		document.Source.ContentSchemaVersion == "" || document.Source.BuilderIdentity == "" {
		return fmt.Errorf("%w: frozen source provenance is invalid", ErrInvalidInput)
	}
	if document.Profile.ProfileID == "" || document.Profile.ProfileVersion == "" {
		return fmt.Errorf("%w: frozen Profile reference is invalid", ErrInvalidInput)
	}
	if _, err := aiexplanation.ParseFingerprint(document.Profile.ProfileFingerprint); err != nil {
		return fmt.Errorf("%w: frozen Profile fingerprint is invalid", ErrInvalidInput)
	}
	if !localePattern.MatchString(document.Context.Locale) || document.Context.Scope != "current_assessment_only" ||
		document.Context.Audience != string(policy.AudienceParticipant) ||
		(document.Context.PersonalizationScope != "assessment_result_only" && document.Context.PersonalizationScope != "assessment_result_and_focus_areas") {
		return fmt.Errorf("%w: frozen context is invalid", ErrInvalidInput)
	}
	if document.Facts.Runtime.DecisionKind != string(modelcatalog.DecisionKindScoreRange) || document.Facts.Model.Kind != string(modelcatalog.KindScale) ||
		document.Facts.Model.Algorithm == "" || document.Facts.Model.Code == "" || document.Facts.Model.Version == "" || len(document.Facts.Dimensions) < 2 {
		return fmt.Errorf("%w: frozen facts are outside v1 scope", ErrInvalidInput)
	}
	return nil
}

func buildContext(locale string, focusAreas, allowed []string, audience policy.Audience) (Context, error) {
	if len(focusAreas) > 3 {
		return Context{}, fmt.Errorf("%w: at most three focus areas are allowed", ErrInvalidInput)
	}
	allowedSet := stringSet(allowed)
	seen := map[string]struct{}{}
	cloned := make([]string, 0, len(focusAreas))
	for _, focus := range focusAreas {
		if focus == "" || !routeLikeCode(focus) {
			return Context{}, fmt.Errorf("%w: focus area %q is invalid", ErrInvalidInput, focus)
		}
		if _, ok := allowedSet[focus]; !ok {
			return Context{}, fmt.Errorf("%w: focus area %q is not allowed by profile", ErrInvalidInput, focus)
		}
		if _, duplicate := seen[focus]; duplicate {
			return Context{}, fmt.Errorf("%w: focus area %q is duplicated", ErrInvalidInput, focus)
		}
		seen[focus] = struct{}{}
		cloned = append(cloned, focus)
	}
	scope := "assessment_result_only"
	if len(cloned) > 0 {
		scope = "assessment_result_and_focus_areas"
	}
	return Context{Scope: "current_assessment_only", Audience: string(audience), Locale: locale, PersonalizationScope: scope, FocusAreas: cloned}, nil
}

func buildFacts(content domainreport.Content, modelFact evaluationfact.ModelIdentity, decisionKind modelcatalog.DecisionKind, definition domainprofile.Definition) (Facts, error) {
	return buildFactsFromModel(content, modelFact.Kind, modelFact.Algorithm, modelFact.Code, modelFact.Version, modelFact.Title, decisionKind, definition)
}

func buildFactsFromModel(content domainreport.Content, modelKind modelcatalog.Kind, algorithm modelcatalog.Algorithm, code, version, title string, decisionKind modelcatalog.DecisionKind, definition domainprofile.Definition) (Facts, error) {
	if algorithm == "" || code == "" || version == "" || !validPlainText(title, 2000, true) {
		return Facts{}, fmt.Errorf("%w: model identity is incomplete", ErrInvalidInput)
	}
	dimensions, refsByCode, err := eligibleDimensions(content, definition)
	if err != nil {
		return Facts{}, err
	}
	suggestions, suggestionRefsByDimension, err := standardSuggestions(content, refsByCode)
	if err != nil {
		return Facts{}, err
	}
	for i := range dimensions {
		dimensions[i].StandardSuggestionRefs = append([]string(nil), suggestionRefsByDimension[dimensions[i].Code]...)
	}
	var modelResult *ModelResult
	if definition.InputPolicy.IncludeModelResult && content.ModelExtra != nil {
		modelResult, err = toModelResult(content.ModelExtra)
		if err != nil {
			return Facts{}, err
		}
	}
	return Facts{
		Runtime:       Runtime{DecisionKind: string(decisionKind)},
		Model:         Model{Kind: string(modelKind), Algorithm: string(algorithm), Code: code, Version: version, Title: title},
		OverallResult: OverallResult{PrimaryScore: toScoreValue(content.PrimaryScore), Level: toLevel(content.Level), StandardConclusion: content.Conclusion},
		Dimensions:    dimensions, StandardSuggestions: suggestions, ModelResult: modelResult,
	}, nil
}

func eligibleDimensions(content domainreport.Content, definition domainprofile.Definition) ([]Dimension, map[string]string, error) {
	dimensions := content.Dimensions
	if domainreport.UsesFactorScoreVisibility(content.Model) {
		if content.PresentationProfile == nil || !content.PresentationProfile.Configured() {
			return nil, nil, fmt.Errorf("%w: frozen participant presentation profile is required", ErrInvalidInput)
		}
		dimensions = domainreport.FilterDimensionInterprets(dimensions, content.PresentationProfile.VisibleSet())
	}
	eligible := stringSet(definition.Eligibility.EligibleDimensionCodes)
	excluded := stringSet(definition.Eligibility.ExcludedDimensionCodes)
	selected := make([]domainreport.DimensionInterpret, 0, len(dimensions))
	seenCodes := map[string]struct{}{}
	for _, dimension := range dimensions {
		code := dimension.Code().String()
		if code == "" {
			return nil, nil, fmt.Errorf("%w: dimension code is required", ErrInvalidInput)
		}
		if _, duplicate := seenCodes[code]; duplicate {
			return nil, nil, fmt.Errorf("%w: dimension code %q is duplicated", ErrInvalidInput, code)
		}
		seenCodes[code] = struct{}{}
		if len(eligible) > 0 {
			if _, ok := eligible[code]; !ok {
				continue
			}
		}
		if _, skip := excluded[code]; skip {
			continue
		}
		selected = append(selected, dimension)
	}
	if len(selected) < definition.Eligibility.MinEligibleDimensions {
		return nil, nil, fmt.Errorf("%w: only %d eligible dimensions", ErrNotApplicable, len(selected))
	}
	if len(selected) > definition.Eligibility.MaxInputDimensions {
		return nil, nil, fmt.Errorf("%w: %d dimensions exceed profile maximum", ErrNotApplicable, len(selected))
	}
	refsByCode := make(map[string]string, len(selected))
	for _, dimension := range selected {
		refsByCode[dimension.Code().String()] = dimensionRef(dimension.Code().String())
	}
	result := make([]Dimension, 0, len(selected))
	for _, dimension := range selected {
		code := dimension.Code().String()
		var parentRef *string
		if ref, ok := refsByCode[dimension.ParentCode()]; ok {
			value := ref
			parentRef = &value
		}
		derived := make([]ScoreValue, 0, len(dimension.DerivedScores()))
		for _, score := range dimension.DerivedScores() {
			derived = append(derived, *toScoreValue(&score))
		}
		var norm *NormContext
		if definition.InputPolicy.IncludeNormContext {
			norm = toNormContext(dimension.NormReference())
		}
		item := Dimension{
			Ref: refsByCode[code], Code: code, Kind: string(dimension.Kind()), Name: dimension.Name(), Role: dimension.Role(), ParentRef: parentRef,
			HierarchyLevel: dimension.HierarchyLevel(), SortOrder: dimension.SortOrder(),
			RawScore:      ScoreValue{Kind: "raw_score", Value: dimension.RawScore(), Max: dimension.MaxScore()},
			DerivedScores: derived, Level: toLevel(dimension.Level()), NormContext: norm, StandardDescription: dimension.Description(), StandardSuggestionRefs: []string{},
		}
		if err := validateDimension(item); err != nil {
			return nil, nil, err
		}
		result = append(result, item)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].SortOrder != result[j].SortOrder {
			return result[i].SortOrder < result[j].SortOrder
		}
		return result[i].Code < result[j].Code
	})
	return result, refsByCode, nil
}

func standardSuggestions(content domainreport.Content, dimensionRefs map[string]string) ([]StandardSuggestion, map[string][]string, error) {
	result := make([]StandardSuggestion, 0, len(content.Suggestions)+len(content.Dimensions))
	byDimension := map[string][]string{}
	seenSemantic := map[string]struct{}{}
	appendSuggestion := func(ref, category, text, dimensionCode string) error {
		if category == "" || !validPlainText(text, 2000, true) {
			return fmt.Errorf("%w: standard suggestion is invalid", ErrInvalidInput)
		}
		key := category + "\x00" + text + "\x00" + dimensionCode
		if _, duplicate := seenSemantic[key]; duplicate {
			return nil
		}
		seenSemantic[key] = struct{}{}
		dimensionRefsForSuggestion := []string{}
		if dimensionCode != "" {
			dimensionRefValue, ok := dimensionRefs[dimensionCode]
			if !ok {
				return nil
			}
			dimensionRefsForSuggestion = append(dimensionRefsForSuggestion, dimensionRefValue)
			byDimension[dimensionCode] = append(byDimension[dimensionCode], ref)
		}
		result = append(result, StandardSuggestion{Ref: ref, Category: category, Content: text, DimensionRefs: dimensionRefsForSuggestion})
		return nil
	}
	for index, suggestion := range content.Suggestions {
		code := ""
		if suggestion.FactorCode != nil {
			code = suggestion.FactorCode.String()
		}
		if err := appendSuggestion(fmt.Sprintf("suggestion:report:%d", index+1), string(suggestion.Category), suggestion.Content, code); err != nil {
			return nil, nil, err
		}
	}
	for _, dimension := range content.Dimensions {
		if strings.TrimSpace(dimension.Suggestion()) == "" {
			continue
		}
		code := dimension.Code().String()
		if err := appendSuggestion("suggestion:dimension:"+url.PathEscape(code), "dimension", dimension.Suggestion(), code); err != nil {
			return nil, nil, err
		}
	}
	return result, byDimension, nil
}

func toScoreValue(value *domainreport.ScoreValue) *ScoreValue {
	if value == nil {
		return nil
	}
	result := &ScoreValue{Kind: value.Kind, Value: value.Value, Label: value.Label}
	if value.Max != nil {
		max := *value.Max
		result.Max = &max
	}
	return result
}

func toLevel(value *domainreport.ResultLevel) *ResultLevel {
	if value == nil {
		return nil
	}
	return &ResultLevel{Code: value.Code, Label: value.Label, Severity: value.Severity}
}

func toNormContext(value *domainreport.NormReference) *NormContext {
	if value == nil {
		return nil
	}
	return &NormContext{ScoreKind: value.ScoreKind, Benchmark: value.Benchmark, TableVersion: value.TableVersion, FormVariant: value.FormVariant, MinAgeMonths: value.MinAgeMonths, MaxAgeMonths: value.MaxAgeMonths, Gender: value.Gender}
}

func toModelResult(value *domainreport.ModelExtra) (*ModelResult, error) {
	if value == nil {
		return nil, nil
	}
	if value.Kind == "" || value.TypeCode == "" || !validPlainText(value.TypeName, 2000, true) || !validPlainText(value.OneLiner, 2000, true) || value.MatchPercent < 0 || value.MatchPercent > 100 {
		return nil, fmt.Errorf("%w: model result is incomplete", ErrInvalidInput)
	}
	return &ModelResult{Kind: value.Kind, TypeCode: value.TypeCode, TypeName: value.TypeName, OneLiner: value.OneLiner, MatchPercent: value.MatchPercent, Commentary: value.Commentary}, nil
}

func validateDimension(value Dimension) error {
	if value.Ref == "" || value.Code == "" || value.Kind == "" || !validPlainText(value.Name, 2000, true) || value.HierarchyLevel < 0 || value.SortOrder < 0 {
		return fmt.Errorf("%w: dimension %q is invalid", ErrInvalidInput, value.Code)
	}
	if value.RawScore.Kind == "" || !validPlainText(value.StandardDescription, 4000, false) {
		return fmt.Errorf("%w: dimension %q score or description is invalid", ErrInvalidInput, value.Code)
	}
	return nil
}

func dimensionRef(code string) string { return "dimension:" + url.PathEscape(code) }

func stringSet(items []string) map[string]struct{} {
	result := make(map[string]struct{}, len(items))
	for _, item := range items {
		result[item] = struct{}{}
	}
	return result
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
