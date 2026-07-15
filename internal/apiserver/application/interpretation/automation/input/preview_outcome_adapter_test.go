package input

import (
	"testing"

	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	"github.com/FangcunMount/qs-server/internal/apiserver/domain/interpretation/report"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationfact"
	"github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	behavioralsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/behavioral"
)

func TestPreviewFactorScoresPreferCanonicalDimensions(t *testing.T) {
	execution := &evaluationfact.Execution{Dimensions: []evaluationfact.DimensionResult{
		{Code: "gec", Name: "GEC", Role: "index", HierarchyLevel: 1, Score: &evaluationfact.ScoreValue{Value: 10}, DerivedScores: []evaluationfact.ScoreValue{{Kind: evaluationfact.ScoreKindTScore, Value: 65}}, Level: &evaluationfact.ResultLevel{Code: "medium", Label: "中等", Severity: "medium"}, NormReference: &evaluationfact.NormReference{ScoreKind: evaluationfact.ScoreKindTScore, Benchmark: 50, TableVersion: "2026", MinAgeMonths: 60, MaxAgeMonths: 95}},
		{Code: "bri", Name: "BRI", Role: "index", ParentCode: "gec", HierarchyLevel: 2, Score: &evaluationfact.ScoreValue{Value: 8}},
	}}
	items := factorScores(execution, nil)
	if len(items) != 2 {
		t.Fatalf("factor scores = %d, want 2", len(items))
	}
	if items[1].ParentCode != "gec" || items[1].HierarchyLevel != 2 {
		t.Fatalf("child score = %#v, want hierarchy metadata", items[1])
	}
	if items[0].RiskLevel != report.RiskLevelMedium {
		t.Fatalf("risk level = %s, want medium", items[0].RiskLevel)
	}
	if len(items[0].DerivedScores) != 1 || items[0].DerivedScores[0].Value != 65 || items[0].Level == nil || items[0].Level.Label != "中等" {
		t.Fatalf("derived score context = %#v", items[0])
	}
	if items[0].NormReference == nil || items[0].NormReference.TableVersion != "2026" || items[0].NormReference.Benchmark != 50 {
		t.Fatalf("norm reference = %#v", items[0].NormReference)
	}
}

func TestApplyFrozenNormInterpretationRestoresDimensionLabelAndSuggestion(t *testing.T) {
	factors := factorScores(&evaluationfact.Execution{Dimensions: []evaluationfact.DimensionResult{{
		Code: "gec", Score: &evaluationfact.ScoreValue{Value: 10},
		DerivedScores: []evaluationfact.ScoreValue{{Kind: evaluationfact.ScoreKindTScore, Value: 65}},
		Level:         &evaluationfact.ResultLevel{Code: "elevated"},
	}}}, nil)
	assets := &evaluationinput.InputSnapshot{ModelPayload: evaluationinput.BehavioralRatingModelPayload{Snapshot: &behavioralsnapshot.Snapshot{Norming: &behavioralsnapshot.NormingProfile{NormTables: &calcnorm.NormTables{TScoreRules: []calcnorm.TScoreInterpretRule{{FactorCode: "gec", Ranges: []calcnorm.TScoreRange{{MinT: 60, MaxT: 100, Level: "elevated", Conclusion: "偏高", Suggestion: "建议关注"}}}}}}}}}

	applyFrozenNormInterpretation(factors, assets)
	if factors[0].Level == nil || factors[0].Level.Label != "偏高" || factors[0].Conclusion != "偏高" || factors[0].Suggestion != "建议关注" {
		t.Fatalf("restored norm interpretation = %#v", factors[0])
	}
}
