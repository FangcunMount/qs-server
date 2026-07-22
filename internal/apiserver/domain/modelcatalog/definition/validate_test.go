package definition_test

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/binding"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/definition"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/factor"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/norm"
)

func TestValidateAcceptsCompleteDefinition(t *testing.T) {
	t.Parallel()

	def := definition.Definition{
		Measure: definition.MeasureSpec{
			Factors: []factor.Factor{
				{Code: "total", Role: factor.FactorRoleTotal},
				{Code: "trait", Role: factor.FactorRoleDimension},
			},
			Scoring: []factor.Scoring{{
				FactorCode: "trait",
				Sources: []factor.ScoringSource{{
					Kind:         factor.ScoringSourceQuestion,
					Code:         "q1",
					Sign:         -1,
					OptionScores: map[string]float64{"A": 1, "B": 5},
				}},
				Strategy: factor.ScoringStrategySum,
			}},
		},
		Calibration: definition.Calibration{NormRefs: []norm.Ref{{FactorCode: "total", NormTableVersion: "2026"}}},
		Outcomes: []conclusion.Outcome{
			{Code: "type_a", Title: "Type A"},
			{Code: "average", Title: "Average"},
		},
		Conclusions: []conclusion.Conclusion{
			conclusion.NormConclusion{
				FactorCode: "total", ScoreBasis: conclusion.ScoreBasisTScore, Primary: true,
				Rules: []conclusion.ScoreRangeOutcome{{MinScore: 40, MaxScore: 60, Level: "average", OutcomeCode: "average", MaxInclusive: true}},
			},
			conclusion.AbilityConclusion{
				FactorCode: "total", ScoreBasis: conclusion.ScoreBasisRaw, Primary: true,
				Rules: []conclusion.ScoreRangeOutcome{{MinScore: 0, MaxScore: 10, OutcomeCode: "type_a", MaxInclusive: true}},
			},
			conclusion.TypeConclusion{
				FactorCodes: []string{"trait"},
				Decision:    conclusion.TypeDecision{Kind: binding.DecisionKindTraitProfile},
				Profiles:    []conclusion.TypeOutcomeProfile{{OutcomeCode: "type_a"}},
			},
		},
		ReportMap: definition.ReportMap{Sections: []definition.ReportSection{{
			Code: "personality", Kind: "template", TemplateID: "bigfive", AdapterKey: "trait_profile",
		}}},
	}

	if issues := definition.Validate(def); len(issues) != 0 {
		t.Fatalf("Validate() issues = %#v", issues)
	}
}

func TestValidateReportsCrossLayerReferenceErrors(t *testing.T) {
	t.Parallel()

	issues := definition.Validate(definition.Definition{
		Measure: definition.MeasureSpec{Factors: []factor.Factor{{Code: "total", Role: factor.FactorRoleTotal}}},
		Calibration: definition.Calibration{NormRefs: []norm.Ref{
			{FactorCode: "missing", NormTableVersion: "2026"},
			{FactorCode: "missing", NormTableVersion: "2026"},
		}},
		Outcomes: []conclusion.Outcome{{Code: "same"}, {Code: "same"}},
		Conclusions: []conclusion.Conclusion{
			conclusion.NormConclusion{FactorCode: "missing", ScoreBasis: conclusion.ScoreBasis("unknown")},
			conclusion.TypeConclusion{Decision: conclusion.TypeDecision{Kind: binding.DecisionKindNormLookup}, Profiles: []conclusion.TypeOutcomeProfile{{OutcomeCode: "missing"}}},
		},
		ReportMap: definition.ReportMap{Sections: []definition.ReportSection{{Code: "report", TemplateID: "x"}, {Code: "report"}}},
	})

	if len(issues) < 7 {
		t.Fatalf("Validate() issue count = %d, want cross-layer violations: %#v", len(issues), issues)
	}
}

func TestValidateRequiresOutcomeCodeAndRejectsOverlapOrGap(t *testing.T) {
	t.Parallel()

	base := definition.Definition{
		Measure:  definition.MeasureSpec{Factors: []factor.Factor{{Code: "total", Role: factor.FactorRoleTotal}}},
		Outcomes: []conclusion.Outcome{{Code: "low"}, {Code: "high"}},
	}

	t.Run("missing outcome code", func(t *testing.T) {
		def := base
		def.Conclusions = []conclusion.Conclusion{conclusion.RiskConclusion{
			FactorCode: "total",
			Rules:      []conclusion.ScoreRangeOutcome{{MinScore: 0, MaxScore: 10, Level: "low"}},
		}}
		issues := definition.Validate(def)
		if !hasValidationCode(issues, "conclusion.outcome_code.required") {
			t.Fatalf("issues = %#v", issues)
		}
	})

	t.Run("overlap", func(t *testing.T) {
		def := base
		def.Conclusions = []conclusion.Conclusion{conclusion.RiskConclusion{
			FactorCode: "total",
			Rules: []conclusion.ScoreRangeOutcome{
				{MinScore: 0, MaxScore: 60, OutcomeCode: "low"},
				{MinScore: 50, MaxScore: 100, OutcomeCode: "high", MaxInclusive: true},
			},
		}}
		issues := definition.Validate(def)
		if !hasValidationCode(issues, "conclusion.range.overlap") {
			t.Fatalf("issues = %#v", issues)
		}
	})

	t.Run("gap", func(t *testing.T) {
		def := base
		def.Conclusions = []conclusion.Conclusion{conclusion.RiskConclusion{
			FactorCode: "total",
			Rules: []conclusion.ScoreRangeOutcome{
				{MinScore: 0, MaxScore: 40, OutcomeCode: "low"},
				{MinScore: 50, MaxScore: 100, OutcomeCode: "high", MaxInclusive: true},
			},
		}}
		issues := definition.Validate(def)
		if !hasValidationCode(issues, "conclusion.range.gap") {
			t.Fatalf("issues = %#v", issues)
		}
	})

	t.Run("adjacent half-open accepted", func(t *testing.T) {
		def := base
		def.Conclusions = []conclusion.Conclusion{conclusion.RiskConclusion{
			FactorCode: "total",
			Rules: []conclusion.ScoreRangeOutcome{
				{MinScore: 0, MaxScore: 60, OutcomeCode: "low"},
				{MinScore: 60, MaxScore: 100, OutcomeCode: "high", MaxInclusive: true},
			},
		}}
		if issues := definition.Validate(def); len(issues) != 0 {
			t.Fatalf("issues = %#v", issues)
		}
	})

	t.Run("unbounded max accepted", func(t *testing.T) {
		def := base
		def.Conclusions = []conclusion.Conclusion{conclusion.RiskConclusion{
			FactorCode: "total",
			Rules: []conclusion.ScoreRangeOutcome{
				{MinScore: 0, MaxScore: 60, OutcomeCode: "low"},
				{MinScore: 60, OutcomeCode: "high", UnboundedMax: true},
			},
		}}
		if issues := definition.Validate(def); len(issues) != 0 {
			t.Fatalf("issues = %#v", issues)
		}
	})

	t.Run("last endpoint required", func(t *testing.T) {
		def := base
		def.Conclusions = []conclusion.Conclusion{conclusion.RiskConclusion{
			FactorCode: "total",
			Rules: []conclusion.ScoreRangeOutcome{
				{MinScore: 0, MaxScore: 60, OutcomeCode: "low"},
				{MinScore: 60, MaxScore: 100, OutcomeCode: "high"},
			},
		}}
		issues := definition.Validate(def)
		if !hasValidationCode(issues, "conclusion.range.endpoint.required") {
			t.Fatalf("issues = %#v", issues)
		}
	})

	t.Run("non-last max inclusive rejected", func(t *testing.T) {
		def := base
		def.Conclusions = []conclusion.Conclusion{conclusion.RiskConclusion{
			FactorCode: "total",
			Rules: []conclusion.ScoreRangeOutcome{
				{MinScore: 0, MaxScore: 60, OutcomeCode: "low", MaxInclusive: true},
				{MinScore: 61, MaxScore: 100, OutcomeCode: "high", MaxInclusive: true},
			},
		}}
		issues := definition.Validate(def)
		if !hasValidationCode(issues, "conclusion.range.endpoint.non_last") {
			t.Fatalf("issues = %#v", issues)
		}
	})
}

func TestValidateScoreRangeCoverageForEveryRangeFamily(t *testing.T) {
	t.Parallel()

	type conclusionFactory func([]conclusion.ScoreRangeOutcome) conclusion.Conclusion
	families := map[string]conclusionFactory{
		"scale": func(rules []conclusion.ScoreRangeOutcome) conclusion.Conclusion {
			return conclusion.RiskConclusion{FactorCode: "total", Rules: rules}
		},
		"behavioral": func(rules []conclusion.ScoreRangeOutcome) conclusion.Conclusion {
			return conclusion.NormConclusion{FactorCode: "total", ScoreBasis: conclusion.ScoreBasisTScore, Primary: true, Rules: rules}
		},
		"cognitive": func(rules []conclusion.ScoreRangeOutcome) conclusion.Conclusion {
			return conclusion.AbilityConclusion{FactorCode: "total", ScoreBasis: conclusion.ScoreBasisRaw, Primary: true, Rules: rules}
		},
	}
	base := definition.Definition{
		Measure:  definition.MeasureSpec{Factors: []factor.Factor{{Code: "total", Role: factor.FactorRoleTotal}}},
		Outcomes: []conclusion.Outcome{{Code: "low"}, {Code: "high"}},
	}
	for family, factory := range families {
		family, factory := family, factory
		t.Run(family, func(t *testing.T) {
			t.Run("adjacent accepted", func(t *testing.T) {
				def := base
				def.Conclusions = []conclusion.Conclusion{factory([]conclusion.ScoreRangeOutcome{
					{MinScore: 0, MaxScore: 40, OutcomeCode: "low"},
					{MinScore: 40, MaxScore: 100, OutcomeCode: "high", MaxInclusive: true},
				})}
				if issues := definition.Validate(def); len(issues) != 0 {
					t.Fatalf("issues = %#v", issues)
				}
			})
			t.Run("gap rejected", func(t *testing.T) {
				def := base
				def.Conclusions = []conclusion.Conclusion{factory([]conclusion.ScoreRangeOutcome{
					{MinScore: 0, MaxScore: 30, OutcomeCode: "low"},
					{MinScore: 40, MaxScore: 100, OutcomeCode: "high", MaxInclusive: true},
				})}
				if issues := definition.Validate(def); !hasValidationCode(issues, "conclusion.range.gap") {
					t.Fatalf("issues = %#v, want conclusion.range.gap", issues)
				}
			})
			t.Run("overlap rejected", func(t *testing.T) {
				def := base
				def.Conclusions = []conclusion.Conclusion{factory([]conclusion.ScoreRangeOutcome{
					{MinScore: 0, MaxScore: 50, OutcomeCode: "low"},
					{MinScore: 40, MaxScore: 100, OutcomeCode: "high", MaxInclusive: true},
				})}
				if issues := definition.Validate(def); !hasValidationCode(issues, "conclusion.range.overlap") {
					t.Fatalf("issues = %#v, want conclusion.range.overlap", issues)
				}
			})
		})
	}
}

func TestValidateRejectsReportAdapterIncompatibleWithDecisionKind(t *testing.T) {
	t.Parallel()

	def := definition.Definition{
		Measure:  definition.MeasureSpec{Factors: []factor.Factor{{Code: "E", Role: factor.FactorRoleDimension}}},
		Outcomes: []conclusion.Outcome{{Code: "ENTJ"}},
		Conclusions: []conclusion.Conclusion{conclusion.TypeConclusion{
			FactorCodes: []string{"E"},
			Decision:    conclusion.TypeDecision{Kind: binding.DecisionKindPoleComposition},
			Profiles:    []conclusion.TypeOutcomeProfile{{OutcomeCode: "ENTJ"}},
		}},
		ReportMap: definition.ReportMap{Sections: []definition.ReportSection{{
			Code: "main", AdapterKey: "trait_profile",
		}}},
	}
	issues := definition.Validate(def)
	if !hasValidationCode(issues, "report_section.adapter.decision_mismatch") {
		t.Fatalf("issues = %#v, want report_section.adapter.decision_mismatch", issues)
	}

	def.ReportMap.Sections[0].AdapterKey = "mbti"
	issues = definition.Validate(def)
	if !hasValidationCode(issues, "report_section.adapter.decision_mismatch") {
		t.Fatalf("issues = %#v, want report_section.adapter.decision_mismatch", issues)
	}

	def.ReportMap.Sections[0].AdapterKey = "personality_type"
	issues = definition.Validate(def)
	if hasValidationCode(issues, "report_section.adapter.decision_mismatch") {
		t.Fatalf("issues = %#v, want no adapter issues for compatible adapter", issues)
	}

	def.ReportMap.Sections[0].TemplateID = "not-registered"
	issues = definition.Validate(def)
	if !hasValidationCode(issues, "report_section.template_id.unknown") {
		t.Fatalf("issues = %#v, want report_section.template_id.unknown", issues)
	}

	def.ReportMap.Sections[0].TemplateID = "mbti"
	issues = definition.Validate(def)
	if hasValidationCode(issues, "report_section.template_id.unknown") {
		t.Fatalf("issues = %#v, want registered template_id accepted", issues)
	}
}

func TestValidateReportMapFactorScoreSources(t *testing.T) {
	t.Parallel()

	base := definition.Definition{Measure: definition.MeasureSpec{Factors: []factor.Factor{
		{Code: "total", Role: factor.FactorRoleTotal},
		{Code: "detail", Role: factor.FactorRoleDimension},
	}}}

	t.Run("absent mapping accepted", func(t *testing.T) {
		if issues := definition.Validate(base); len(issues) != 0 {
			t.Fatalf("issues = %#v", issues)
		}
	})

	t.Run("explicit empty mapping accepted", func(t *testing.T) {
		def := base
		def.ReportMap.Sections = []definition.ReportSection{{Code: "factors", Kind: definition.ReportSectionKindFactorScores}}
		if issues := definition.Validate(def); len(issues) != 0 {
			t.Fatalf("issues = %#v", issues)
		}
	})

	t.Run("known sources accepted", func(t *testing.T) {
		def := base
		def.ReportMap.Sections = []definition.ReportSection{{Code: "factors", Kind: definition.ReportSectionKindFactorScores, SourceRefs: []string{"total", "detail"}}}
		if issues := definition.Validate(def); len(issues) != 0 {
			t.Fatalf("issues = %#v", issues)
		}
	})

	cases := []struct {
		name     string
		sections []definition.ReportSection
		code     string
	}{
		{name: "blank source", sections: []definition.ReportSection{{Code: "factors", Kind: definition.ReportSectionKindFactorScores, SourceRefs: []string{""}}}, code: "report_section.source_ref.required"},
		{name: "duplicate source", sections: []definition.ReportSection{{Code: "factors", Kind: definition.ReportSectionKindFactorScores, SourceRefs: []string{"total", "total"}}}, code: "report_section.source_ref.duplicate"},
		{name: "unknown source", sections: []definition.ReportSection{{Code: "factors", Kind: definition.ReportSectionKindFactorScores, SourceRefs: []string{"missing"}}}, code: "report_section.source_ref.not_found"},
		{name: "multiple factor sections", sections: []definition.ReportSection{{Code: "factors-a", Kind: definition.ReportSectionKindFactorScores}, {Code: "factors-b", Kind: definition.ReportSectionKindFactorScores}}, code: "report_section.factor_scores.multiple"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			def := base
			def.ReportMap.Sections = tc.sections
			if issues := definition.Validate(def); !hasValidationCode(issues, tc.code) {
				t.Fatalf("issues = %#v, want %s", issues, tc.code)
			}
		})
	}
}

func hasValidationCode(issues []definition.ValidationIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
