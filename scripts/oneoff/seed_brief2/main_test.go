package main

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/FangcunMount/qs-server/internal/apiserver/domain/modelcatalog/conclusion"
	surveyquestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestBuildDefinitionAssignsAllQuestionsAndCompositeIndexes(t *testing.T) {
	questionnaire := testQuestionnaire(t, 9)
	catalog := testCatalog()
	mapping := factorMapping{Factors: factorMap{}}
	for index, code := range catalog.order[:9] {
		mapping.Factors[code] = []string{fmt.Sprintf("q%d", index)}
	}

	definition, err := buildDefinition(questionnaire, mapping, catalog, "brief2-test", "parent")
	if err != nil {
		t.Fatalf("buildDefinition() error = %v", err)
	}
	if got := definition.Execution.Brief2.PrimaryFactorCode; got != "f12" {
		t.Fatalf("primary factor = %s, want f12", got)
	}
	if len(definition.Measure.Factors) != 13 || len(definition.Measure.Scoring) != 13 {
		t.Fatalf("factors=%d scoring=%d, want 13 each", len(definition.Measure.Factors), len(definition.Measure.Scoring))
	}
	if len(definition.Measure.FactorGraph.Edges) != 12 {
		t.Fatalf("factor graph edges = %d, want 12", len(definition.Measure.FactorGraph.Edges))
	}
	if len(definition.Conclusions) != 13 {
		t.Fatalf("conclusions = %d, want 13", len(definition.Conclusions))
	}
	for _, item := range definition.Conclusions {
		norm, ok := item.(conclusion.NormConclusion)
		if !ok {
			t.Fatalf("conclusion = %T, want NormConclusion", item)
		}
		if len(norm.Rules) != 4 {
			t.Fatalf("rules for %s = %d, want 4", norm.FactorCode, len(norm.Rules))
		}
		for _, rule := range norm.Rules {
			if rule.Summary == "" || rule.Description == "" {
				t.Fatalf("rule %s/%s must include summary and description: %#v", norm.FactorCode, rule.Level, rule)
			}
		}
	}
}

func TestBuildDefinitionRejectsDuplicateQuestionAssignment(t *testing.T) {
	questionnaire := testQuestionnaire(t, 9)
	catalog := testCatalog()
	mapping := factorMapping{Factors: factorMap{}}
	for index, code := range catalog.order[:9] {
		mapping.Factors[code] = []string{fmt.Sprintf("q%d", index)}
	}
	mapping.Factors[catalog.order[1]] = []string{"q0", "q1"}

	if _, err := buildDefinition(questionnaire, mapping, catalog, "brief2-test", "parent"); err == nil {
		t.Fatal("buildDefinition() error = nil, want duplicate question error")
	}
}

func TestBuildDefinitionAllowsExplicitlyExcludedQuestions(t *testing.T) {
	questionnaire := testQuestionnaire(t, 10)
	catalog := testCatalog()
	mapping := factorMapping{Factors: factorMap{}, ExcludedQuestions: map[string][]string{"infrequency_validity": {"q9"}}}
	for index, code := range catalog.order[:9] {
		mapping.Factors[code] = []string{fmt.Sprintf("q%d", index)}
	}

	if _, err := buildDefinition(questionnaire, mapping, catalog, "brief2-test", "parent"); err != nil {
		t.Fatalf("buildDefinition() error = %v", err)
	}
}

func TestVersionedFactorMapCoversBRIEF2ClinicalAndExcludedQuestions(t *testing.T) {
	mapping, err := loadFactorMap(filepath.Join("data", "gXkk9W_4.0.1_factor_map.json"))
	if err != nil {
		t.Fatalf("loadFactorMap() error = %v", err)
	}
	if err := mapping.validateTarget("gXkk9W", "4.0.1"); err != nil {
		t.Fatalf("validateTarget() error = %v", err)
	}
	if got := mapping.mappedQuestionCount(); got != 60 {
		t.Fatalf("mapped questions = %d, want 60", got)
	}
	if got := mapping.excludedQuestionCount(); got != 10 {
		t.Fatalf("excluded questions = %d, want 10", got)
	}
}

func TestEmbeddedNormSourceIsUsableWithoutExternalFiles(t *testing.T) {
	source, err := loadNormSource("")
	if err != nil {
		t.Fatalf("loadNormSource() error = %v", err)
	}
	if len(source.Factors) != 13 || len(source.Scores) != 6 {
		t.Fatalf("embedded source factors=%d strata=%d, want 13 and 6", len(source.Factors), len(source.Scores))
	}
	if _, _, err := buildNormTable(source, defaultNormVersion, defaultFormVariant); err != nil {
		t.Fatalf("buildNormTable() error = %v", err)
	}
}

func testCatalog() normCatalog {
	catalog := normCatalog{byNormName: map[string]string{}, titles: map[string]string{}, order: make([]string, 0, 13)}
	for index := 0; index < 13; index++ {
		code := fmt.Sprintf("f%d", index)
		catalog.order = append(catalog.order, code)
		catalog.titles[code] = code
	}
	return catalog
}

func testQuestionnaire(t *testing.T, count int) *surveyquestionnaire.Questionnaire {
	t.Helper()
	questionnaire, err := surveyquestionnaire.NewQuestionnaire(meta.NewCode("brief2-test"), "BRIEF-2", surveyquestionnaire.WithVersion(surveyquestionnaire.NewVersion("1.0.0")))
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
