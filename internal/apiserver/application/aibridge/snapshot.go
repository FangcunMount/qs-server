package aibridge

import (
	"encoding/json"
	"time"

	"github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/aiexplanation/source"
	report "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
)

// reportSnapshot projects read facts explicitly: domain value objects contain
// private fields and must never be passed directly to encoding/json. AI owns
// Profile eligibility, Prompt assembly and output validation.
func reportSnapshot(current *source.Current) ([]byte, error) {
	if current == nil || current.Report == nil || current.Outcome == nil {
		return nil, source.ErrInconsistent
	}
	r, outcome := current.Report, current.Outcome
	association := r.Association()
	if outcome.ID() != r.OutcomeID() || outcome.AssessmentID() != association.AssessmentID || outcome.TesteeID() != association.TesteeID || outcome.OrgID() != association.OrgID {
		return nil, source.ErrInconsistent
	}
	content := r.Content()
	dimensions := content.Dimensions
	if report.UsesFactorScoreVisibility(content.Model) {
		if content.PresentationProfile == nil || !content.PresentationProfile.Configured() {
			return nil, source.ErrInconsistent
		}
		dimensions = report.FilterDimensionInterprets(dimensions, content.PresentationProfile.VisibleSet())
	}
	visible := make(map[string]bool, len(dimensions))
	dimensionFacts := make([]map[string]any, 0, len(dimensions))
	for _, dimension := range dimensions {
		code := dimension.Code().String()
		visible[code] = true
		derived := make([]any, 0, len(dimension.DerivedScores()))
		for _, score := range dimension.DerivedScores() {
			derived = append(derived, snapshotScore(&score))
		}
		dimensionFacts = append(dimensionFacts, map[string]any{
			"code": code, "kind": dimension.Kind(), "name": dimension.Name(),
			"raw_score": dimension.RawScore(), "max_score": dimension.MaxScore(),
			"derived_scores": derived, "level": snapshotLevel(dimension.Level()),
			"norm_reference": snapshotNorm(dimension.NormReference()),
			"description":    dimension.Description(), "suggestion": dimension.Suggestion(),
			"role": dimension.Role(), "parent_code": dimension.ParentCode(),
			"hierarchy_level": dimension.HierarchyLevel(), "sort_order": dimension.SortOrder(),
		})
	}
	suggestions := make([]map[string]any, 0, len(content.Suggestions))
	for index, suggestion := range content.Suggestions {
		var code *string
		if suggestion.FactorCode != nil {
			value := suggestion.FactorCode.String()
			if value != "" && !visible[value] {
				continue
			}
			code = &value
		}
		// Preserve the original index for stable standard-suggestion references.
		suggestions = append(suggestions, map[string]any{
			"source_index": index, "category": suggestion.Category,
			"content": suggestion.Content, "dimension_code": code,
		})
	}
	model := outcome.Model()
	return json.Marshal(map[string]any{
		"schema_version": "qs-report-snapshot/v1",
		"source": map[string]any{
			"report_id": r.ID().String(), "outcome_id": r.OutcomeID().String(),
			"report_type": r.ReportType().String(), "report_template_version": r.TemplateVersion().String(),
			"content_schema_version": r.ContentSchemaVersion(), "builder_identity": r.BuilderIdentity(),
			"generated_at": r.GeneratedAt().UTC().Format(time.RFC3339Nano),
		},
		"model":         map[string]any{"kind": model.Kind, "algorithm": model.Algorithm, "code": model.Code, "version": model.Version, "title": model.Title},
		"runtime":       map[string]any{"decision_kind": outcome.Runtime().DecisionKind},
		"primary_score": snapshotScore(content.PrimaryScore), "level": snapshotLevel(content.Level),
		"conclusion": content.Conclusion, "dimensions": dimensionFacts, "suggestions": suggestions,
		"model_extra": content.ModelExtra,
	})
}

func snapshotScore(value *report.ScoreValue) any {
	if value == nil {
		return nil
	}
	return map[string]any{"kind": value.Kind, "value": value.Value, "label": value.Label, "max": value.Max}
}

func snapshotLevel(value *report.ResultLevel) any {
	if value == nil {
		return nil
	}
	return map[string]any{"code": value.Code, "label": value.Label, "severity": value.Severity}
}

func snapshotNorm(value *report.NormReference) any {
	if value == nil {
		return nil
	}
	return map[string]any{
		"score_kind": value.ScoreKind, "benchmark": value.Benchmark, "table_version": value.TableVersion,
		"form_variant": value.FormVariant, "min_age_months": value.MinAgeMonths,
		"max_age_months": value.MaxAgeMonths, "gender": value.Gender,
	}
}
