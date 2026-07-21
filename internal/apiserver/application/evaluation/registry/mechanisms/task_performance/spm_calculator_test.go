package task_performance

import (
	"errors"
	"testing"

	calcnorm "github.com/FangcunMount/qs-server/internal/apiserver/domain/calculation/norm"
	domainoutcome "github.com/FangcunMount/qs-server/internal/apiserver/domain/evaluation/outcome"
	portevaluationinput "github.com/FangcunMount/qs-server/internal/apiserver/port/evaluationinput"
	taskperfsnapshot "github.com/FangcunMount/qs-server/internal/apiserver/port/modelcatalog/payload/cognitive"
)

func spmAgePtr(value int) *int { return &value }

func TestCalculateSPMUsesFrozenAnswerKeys(t *testing.T) {
	t.Parallel()
	snapshot := &taskperfsnapshot.Snapshot{Code: "SPM", Version: "1", Title: "SPM", SPM: &taskperfsnapshot.SPMSpec{
		TimeLimitSeconds: 2400, TotalFactorCode: "total",
		ItemSets: []taskperfsnapshot.SPMItemSet{{Code: "A", Items: []taskperfsnapshot.SPMItem{{QuestionCode: "A1", CorrectOptionCode: "1"}, {QuestionCode: "A2", CorrectOptionCode: "3"}}}},
	}}
	input := &portevaluationinput.InputSnapshot{AnswerSheet: &portevaluationinput.AnswerSheetSnapshot{Answers: []portevaluationinput.AnswerSnapshot{
		{QuestionCode: "A1", Value: "1"}, {QuestionCode: "A2", Value: "2"}, {QuestionCode: "A3", Value: "1"},
	}}}
	got, err := CalculateSPM(input, snapshot)
	if err != nil {
		t.Fatalf("CalculateSPM: %v", err)
	}
	if got.Primary == nil || got.Primary.Value != 1 || got.Primary.Max == nil || *got.Primary.Max != 2 {
		t.Fatalf("primary = %#v, want 1/2", got.Primary)
	}
	if len(got.Dimensions) != 2 || got.Dimensions[0].Score == nil || got.Dimensions[0].Score.Value != 1 || got.Dimensions[1].Code != "total" {
		t.Fatalf("dimensions = %#v", got.Dimensions)
	}
}

func TestCalculateSPMRequiredNormFailureReturnsNoOutcome(t *testing.T) {
	t.Parallel()

	snapshot := &taskperfsnapshot.Snapshot{Code: "SPM", Version: "1", Title: "SPM", SPM: &taskperfsnapshot.SPMSpec{
		TotalFactorCode: "total", NormRequired: true,
		ItemSets: []taskperfsnapshot.SPMItemSet{{Code: "A", Items: []taskperfsnapshot.SPMItem{{QuestionCode: "A1", CorrectOptionCode: "1"}}}},
		NormTables: &calcnorm.NormTables{Factors: []calcnorm.FactorNormTable{{
			FactorCode: "total",
			Lookup:     []calcnorm.NormLookupEntry{{RawMin: 1, RawMax: 1, MinAgeMonths: 60, MaxAgeMonths: 95, Gender: "female", TScore: 60, Percentile: 84}},
		}}},
	}}
	input := &portevaluationinput.InputSnapshot{AnswerSheet: &portevaluationinput.AnswerSheetSnapshot{Answers: []portevaluationinput.AnswerSnapshot{{QuestionCode: "A1", Value: "1"}}}}

	got, err := CalculateSPM(input, snapshot)
	if got != nil {
		t.Fatalf("outcome = %#v, want nil", got)
	}
	var resolutionErr *calcnorm.ResolutionError
	if !errors.As(err, &resolutionErr) || resolutionErr.Kind != calcnorm.ErrorKindSubjectMissing {
		t.Fatalf("error = %T %v", err, err)
	}
}

func TestCalculateSPMRetainsNormReference(t *testing.T) {
	t.Parallel()
	standard := 110.0
	snapshot := &taskperfsnapshot.Snapshot{Code: "SPM", Version: "1", Title: "SPM", SPM: &taskperfsnapshot.SPMSpec{
		TimeLimitSeconds: 2400, TotalFactorCode: "total",
		ItemSets: []taskperfsnapshot.SPMItemSet{{Code: "A", Items: []taskperfsnapshot.SPMItem{{QuestionCode: "A1", CorrectOptionCode: "1"}}}},
		NormTables: &calcnorm.NormTables{
			NormTableVersion: "spm-cn-2024", FormVariant: "standard",
			Factors: []calcnorm.FactorNormTable{{
				FactorCode: "total",
				Lookup: []calcnorm.NormLookupEntry{{
					RawMin: 1, RawMax: 1, Percentile: 75, StandardScore: &standard,
					MinAgeMonths: 60, MaxAgeMonths: 95, Gender: "female",
				}},
			}},
		},
	}}
	input := &portevaluationinput.InputSnapshot{
		AnswerSheet: &portevaluationinput.AnswerSheetSnapshot{Answers: []portevaluationinput.AnswerSnapshot{{QuestionCode: "A1", Value: "1"}}},
		NormSubject: &portevaluationinput.NormSubjectSnapshot{AgeMonths: spmAgePtr(72), Gender: "female"},
	}
	got, err := CalculateSPM(input, snapshot)
	if err != nil {
		t.Fatalf("CalculateSPM: %v", err)
	}
	total := got.Dimensions[len(got.Dimensions)-1]
	if total.NormReference == nil {
		t.Fatal("NormReference = nil")
	}
	ref := total.NormReference
	if ref.TableVersion != "spm-cn-2024" || ref.FormVariant != "standard" || ref.ScoreKind != domainoutcome.ScoreKindStandardScore {
		t.Fatalf("NormReference = %#v", ref)
	}
	if ref.MinAgeMonths != 60 || ref.MaxAgeMonths != 95 || ref.Gender != "female" {
		t.Fatalf("NormReference cohort = %#v", ref)
	}
}
