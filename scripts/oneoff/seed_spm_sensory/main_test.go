package main

import (
	"fmt"
	"testing"

	surveyquestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestBuildDefinitionUsesFactorNormRuntime(t *testing.T) {
	questionnaire := testQuestionnaire(t, 7)
	catalog := testCatalog()
	mapping := factorMap{}
	for index, code := range catalog.order[:7] {
		mapping[code] = []string{fmt.Sprintf("q%d", index)}
	}

	definition, err := buildDefinition(questionnaire, mapping, catalog, "spm-sensory-test")
	if err != nil {
		t.Fatalf("buildDefinition() error = %v", err)
	}
	if definition.Execution.SPM != nil || definition.Execution.Brief2 != nil {
		t.Fatalf("SPM sensory must not use algorithm-specific execution: %#v", definition.Execution)
	}
	if got := definition.Measure.Factors[len(definition.Measure.Factors)-1].Role; got != "total" {
		t.Fatalf("total role = %q, want total", got)
	}
	if len(definition.Measure.Scoring) != 8 || len(definition.Calibration.NormRefs) != 8 {
		t.Fatalf("scoring=%d normRefs=%d, want 8 each", len(definition.Measure.Scoring), len(definition.Calibration.NormRefs))
	}
}

func TestParseRangeAndTopCodedPercentile(t *testing.T) {
	min, max, err := parseRange("81 -83")
	if err != nil || min != 81 || max != 83 {
		t.Fatalf("parseRange = %v, %v, %v", min, max, err)
	}
	percentile, fallback, err := parsePercentile("")
	if err != nil || !fallback || percentile != 99 {
		t.Fatalf("parsePercentile = %v, %v, %v", percentile, fallback, err)
	}
}

func testCatalog() normCatalog {
	catalog := normCatalog{byNormName: map[string]string{}, titles: map[string]string{}, order: make([]string, 0, 8)}
	for index := 0; index < 8; index++ {
		code := fmt.Sprintf("f%d", index)
		catalog.order = append(catalog.order, code)
		catalog.titles[code] = code
	}
	return catalog
}

func testQuestionnaire(t *testing.T, count int) *surveyquestionnaire.Questionnaire {
	t.Helper()
	questionnaire, err := surveyquestionnaire.NewQuestionnaire(meta.NewCode("spm-sensory-test"), "SPM", surveyquestionnaire.WithVersion(surveyquestionnaire.NewVersion("1.0.0")))
	if err != nil {
		t.Fatalf("NewQuestionnaire() error = %v", err)
	}
	questions := make([]surveyquestionnaire.Question, 0, count)
	for index := 0; index < count; index++ {
		option, err := surveyquestionnaire.NewOptionWithStringCode("1", "从不", 1)
		if err != nil {
			t.Fatalf("NewOptionWithStringCode() error = %v", err)
		}
		question, err := surveyquestionnaire.NewQuestion(
			surveyquestionnaire.WithCode(meta.NewCode(fmt.Sprintf("q%d", index))),
			surveyquestionnaire.WithStem(fmt.Sprintf("题目 %d", index)),
			surveyquestionnaire.WithQuestionType(surveyquestionnaire.QuestionType("Radio")),
			surveyquestionnaire.WithOptions([]surveyquestionnaire.Option{option}),
		)
		if err != nil {
			t.Fatalf("NewQuestion() error = %v", err)
		}
		questions = append(questions, question)
	}
	if err := questionnaire.ReplaceQuestions(questions); err != nil {
		t.Fatalf("ReplaceQuestions() error = %v", err)
	}
	return questionnaire
}
