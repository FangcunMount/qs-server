// Package prompt safely renders a published Profile into an executable Prompt
// request. Assessment facts remain an independent JSON data block and are
// never interpolated into instruction messages.
package prompt

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	appinput "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/input"
	appport "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/port"
	domainprofile "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/aiexplanation/profile"
)

var (
	ErrInvalidRender = errors.New("AI explanation Prompt render is invalid")
	placeholderRE    = regexp.MustCompile(`\{\{[a-z0-9_]+\}\}`)
)

type Messages struct {
	SystemMessage string
	TaskMessage   string
	DataPreamble  string
	DataJSON      []byte
}

func Render(pkg appport.PromptPackage, definition domainprofile.Definition, assembled *appinput.Result) (Messages, error) {
	if err := pkg.Validate(); err != nil {
		return Messages{}, fmt.Errorf("%w: %v", ErrInvalidRender, err)
	}
	if err := definition.Validate(); err != nil {
		return Messages{}, fmt.Errorf("%w: invalid Profile: %v", ErrInvalidRender, err)
	}
	if assembled == nil {
		return Messages{}, fmt.Errorf("%w: assembled input is required", ErrInvalidRender)
	}
	if pkg.Ref.TemplateID != definition.GenerationPolicy.PromptTemplateID || pkg.Ref.Version != definition.GenerationPolicy.PromptVersion {
		return Messages{}, fmt.Errorf("%w: Prompt and Profile identity mismatch", ErrInvalidRender)
	}
	if strings.Contains(pkg.SystemMessage, "{{") || strings.Contains(pkg.DataPreamble, "{{") {
		return Messages{}, fmt.Errorf("%w: dynamic data is forbidden in system and data preamble", ErrInvalidRender)
	}

	values, err := renderValues(definition, assembled.Document.Context)
	if err != nil {
		return Messages{}, err
	}
	allowed := make(map[string]struct{}, len(pkg.AllowedPlaceholders))
	for _, placeholder := range pkg.AllowedPlaceholders {
		allowed[placeholder] = struct{}{}
	}
	task := placeholderRE.ReplaceAllStringFunc(pkg.TaskTemplate, func(placeholder string) string {
		if _, ok := allowed[placeholder]; !ok {
			return placeholder
		}
		return values[placeholder]
	})
	for _, placeholder := range placeholderRE.FindAllString(pkg.TaskTemplate, -1) {
		if _, ok := allowed[placeholder]; !ok {
			return Messages{}, fmt.Errorf("%w: placeholder %s is not allowed", ErrInvalidRender, placeholder)
		}
		if _, ok := values[placeholder]; !ok {
			return Messages{}, fmt.Errorf("%w: placeholder %s has no value", ErrInvalidRender, placeholder)
		}
	}
	if placeholderRE.MatchString(task) {
		return Messages{}, fmt.Errorf("%w: unresolved placeholder", ErrInvalidRender)
	}
	dataJSON, err := validateProviderPayload(assembled.ProviderPayload)
	if err != nil {
		return Messages{}, err
	}
	return Messages{
		SystemMessage: pkg.SystemMessage,
		TaskMessage:   task,
		DataPreamble:  pkg.DataPreamble,
		DataJSON:      dataJSON,
	}, nil
}

func renderValues(definition domainprofile.Definition, context appinput.Context) (map[string]string, error) {
	jsonValue := func(value any) (string, error) {
		raw, err := json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("%w: encode controlled Prompt value: %v", ErrInvalidRender, err)
		}
		return string(raw), nil
	}
	insightKinds, err := jsonValue(definition.InsightPolicy.AllowedKinds)
	if err != nil {
		return nil, err
	}
	suggestionOrigins, err := jsonValue(definition.SuggestionPolicy.AllowedOrigins)
	if err != nil {
		return nil, err
	}
	suggestionCategories, err := jsonValue(definition.SuggestionPolicy.AllowedCategories)
	if err != nil {
		return nil, err
	}
	focusAreas, err := jsonValue(context.FocusAreas)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"{{locale}}":                             context.Locale,
		"{{focus_areas_json}}":                   focusAreas,
		"{{allowed_insight_kinds_json}}":         insightKinds,
		"{{insight_min_items}}":                  strconv.Itoa(definition.InsightPolicy.MinItems),
		"{{insight_max_items}}":                  strconv.Itoa(definition.InsightPolicy.MaxItems),
		"{{min_dimension_refs}}":                 strconv.Itoa(definition.InsightPolicy.MinDimensionRefsPerItem),
		"{{max_dimension_refs}}":                 strconv.Itoa(definition.InsightPolicy.MaxDimensionRefsPerItem),
		"{{allow_parent_child_in_same_insight}}": strconv.FormatBool(definition.InputPolicy.HierarchyPolicy.AllowParentChildInSameInsight),
		"{{allowed_suggestion_origins_json}}":    suggestionOrigins,
		"{{allowed_suggestion_categories_json}}": suggestionCategories,
		"{{suggestion_min_items}}":               strconv.Itoa(definition.SuggestionPolicy.MinItems),
		"{{suggestion_max_items}}":               strconv.Itoa(definition.SuggestionPolicy.MaxItems),
		"{{max_actions_per_item}}":               strconv.Itoa(definition.SuggestionPolicy.MaxActionsPerItem),
		"{{max_output_characters}}":              strconv.Itoa(definition.GenerationPolicy.MaxOutputCharacters),
	}, nil
}

func validateProviderPayload(raw []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document struct {
		Context json.RawMessage `json:"context"`
		Facts   json.RawMessage `json:"facts"`
	}
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("%w: decode provider payload: %v", ErrInvalidRender, err)
	}
	if decoder.More() || len(bytes.TrimSpace(document.Context)) == 0 || len(bytes.TrimSpace(document.Facts)) == 0 {
		return nil, fmt.Errorf("%w: provider payload must contain context and facts only", ErrInvalidRender)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: provider payload has trailing content", ErrInvalidRender)
	} else if err == nil {
		return nil, fmt.Errorf("%w: provider payload has trailing content", ErrInvalidRender)
	}
	compact := &bytes.Buffer{}
	if err := json.Compact(compact, raw); err != nil {
		return nil, fmt.Errorf("%w: compact provider payload: %v", ErrInvalidRender, err)
	}
	return append([]byte(nil), compact.Bytes()...), nil
}
