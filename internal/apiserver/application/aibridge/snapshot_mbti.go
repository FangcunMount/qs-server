package aibridge

import (
	"encoding/json"
	"math"
	"time"

	source "github.com/FangcunMount/qs-server/internal/apiserver/application/interpretation/reportsource"
	report "github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
)

// mbtiReportSnapshot only projects frozen report facts. Missing legacy facts
// never fall back to narrative parsing, the latest model or a scale snapshot.
func mbtiReportSnapshot(current *source.Current) ([]byte, error) {
	r, outcome := current.Report, current.Outcome
	c, model := r.Content(), outcome.Model()
	if c.Model.Kind != string(model.Kind) || c.Model.Algorithm != string(model.Algorithm) || c.Model.Code != model.Code || c.Model.Version != model.Version || c.Model.Title != model.Title {
		return nil, source.ErrInconsistent
	}
	if model.Kind != "typology" || model.Algorithm != "personality_typology" || model.Code != "MBTI_OEJTS" || model.Version != "v64-report-202608-v1" || outcome.Runtime().DecisionKind != "pole_composition" {
		return nil, source.ErrNotApplicable
	}
	if err := report.ValidateMBTIPoleFacts(c); err != nil {
		return nil, source.ErrInconsistent
	}
	if len(c.Dimensions) != 4 {
		return nil, source.ErrNotApplicable
	}
	dimensions := make([]map[string]any, 0, 4)
	for _, d := range c.Dimensions {
		p := d.PoleFacts()
		if p == nil {
			return nil, source.ErrNotApplicable
		}
		if d.Kind() != report.DimensionKindPole || p.MinScore != 8 || p.MaxScore != 40 || p.Threshold != 24 {
			return nil, source.ErrInconsistent
		}
		dimensions = append(dimensions, map[string]any{
			"code": d.Code().String(), "kind": d.Kind(), "name": d.Name(), "raw_score": d.RawScore(),
			"description": d.Description(), "suggestion": d.Suggestion(),
			"pole_facts": map[string]any{
				"schema_version": p.SchemaVersion, "left_pole": p.LeftPole, "right_pole": p.RightPole,
				"preference": p.Preference, "strength": p.Strength, "min_score": p.MinScore,
				"max_score": p.MaxScore, "threshold": p.Threshold, "composition_order": p.CompositionOrder,
			},
		})
	}
	extra := c.ModelExtra
	if extra == nil || extra.Kind != "personality_type" || extra.TypeName == "" || extra.IsSpecial || math.IsNaN(extra.MatchPercent) || math.IsInf(extra.MatchPercent, 0) || extra.MatchPercent < 0 || extra.MatchPercent > 100 || model.Title == "" {
		return nil, source.ErrInconsistent
	}
	suggestions := make([]map[string]any, 0, len(c.Suggestions))
	for i, s := range c.Suggestions {
		var code *string
		if s.FactorCode != nil {
			value := s.FactorCode.String()
			switch value {
			case "EI", "SN", "TF", "JP":
				code = &value
			default:
				return nil, source.ErrInconsistent
			}
		}
		suggestions = append(suggestions, map[string]any{"source_index": i, "category": s.Category, "content": s.Content, "dimension_code": code})
	}
	return json.Marshal(map[string]any{
		"schema_version": "qs-report-snapshot/v2",
		"source": map[string]any{
			"report_id": r.ID().String(), "outcome_id": r.OutcomeID().String(), "report_type": r.ReportType().String(),
			"report_template_version": r.TemplateVersion().String(), "content_schema_version": r.ContentSchemaVersion(),
			"builder_identity": r.BuilderIdentity(), "generated_at": r.GeneratedAt().UTC().Format(time.RFC3339Nano),
		},
		"model":      map[string]any{"kind": model.Kind, "algorithm": model.Algorithm, "code": model.Code, "version": model.Version, "title": model.Title},
		"runtime":    map[string]any{"decision_kind": outcome.Runtime().DecisionKind},
		"conclusion": c.Conclusion, "dimensions": dimensions, "suggestions": suggestions,
		"model_extra": map[string]any{"kind": extra.Kind, "type_code": extra.TypeCode, "type_name": extra.TypeName, "one_liner": extra.OneLiner, "match_percent": extra.MatchPercent, "commentary": extra.Commentary},
	})
}
