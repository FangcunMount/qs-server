package main

import (
	"fmt"
	"path/filepath"
	"testing"

	surveyquestionnaire "github.com/FangcunMount/qs-server/internal/apiserver/domain/survey/questionnaire"
	"github.com/FangcunMount/qs-server/internal/pkg/meta"
)

func TestBuildDefinitionUsesFactorNormRuntime(t *testing.T) {
	questionnaire := testQuestionnaire(t, 75)
	catalog := testCatalog()
	mapping := factorMap{}
	offset := 0
	for _, normName := range normOrder[:7] {
		code := catalog.byNormName[normName]
		count := expectedQuestionCounts[normName]
		for index := 0; index < count; index++ {
			mapping[code] = append(mapping[code], fmt.Sprintf("q%d", offset+index))
		}
		offset += count
	}
	for index := 0; index < 5; index++ {
		mapping[tasteSmellFactorCode] = append(mapping[tasteSmellFactorCode], fmt.Sprintf("q%d", offset+index))
	}

	definition, err := buildDefinition(questionnaire, mapping, catalog, "spm-sensory-test")
	if err != nil {
		t.Fatalf("buildDefinition() error = %v", err)
	}
	if definition.Execution.SPM != nil || definition.Execution.Brief2 != nil {
		t.Fatalf("SPM sensory must not use algorithm-specific execution: %#v", definition.Execution)
	}
	if got := definition.Measure.Factors[7].Role; got != "total" {
		t.Fatalf("total role = %q, want total", got)
	}
	if len(definition.Measure.Factors) != 9 || len(definition.Measure.Scoring) != 9 || len(definition.Calibration.NormRefs) != 8 {
		t.Fatalf("factors=%d scoring=%d normRefs=%d, want 9, 9, 8", len(definition.Measure.Factors), len(definition.Measure.Scoring), len(definition.Calibration.NormRefs))
	}
	totalScoring := definition.Measure.Scoring[len(definition.Measure.Scoring)-1]
	if len(totalScoring.Sources) != 6 {
		t.Fatalf("SPM TOT sources=%d, want 5 sensory-system factors plus taste/smell", len(totalScoring.Sources))
	}
	if len(definition.Measure.FactorGraph.Roots) != 3 {
		t.Fatalf("factor graph roots=%v, want SOC, TOT and PLA", definition.Measure.FactorGraph.Roots)
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

func TestVersionedFactorMapCoversAllSPMQuestions(t *testing.T) {
	mapping, err := loadFactorMap(filepath.Join("data", "bJFKi3_4.0.1_factor_map.json"))
	if err != nil {
		t.Fatalf("loadFactorMap() error = %v", err)
	}
	if err := mapping.validateTarget("bJFKi3", "4.0.1"); err != nil {
		t.Fatalf("validateTarget() error = %v", err)
	}
	if got := mapping.mappedQuestionCount(); got != 75 {
		t.Fatalf("mapped questions = %d, want 75", got)
	}
	actualCodes := map[string]string{
		"SOC": "hwYAqCSd", "VIS": "TPRrr0hh", "HEA": "JxzqkoP3", "TOU": "Hs5rKy8b",
		"BOD": "2OEnsR1F", "BAL": "wZTNmKJk", "PLA": "oaj20O9N",
	}
	for normName, want := range expectedQuestionCounts {
		if got := len(mapping.Factors[actualCodes[normName]]); got != want {
			t.Fatalf("%s questions = %d, want %d", normName, got, want)
		}
	}
	if got := len(mapping.Factors[tasteSmellFactorCode]); got != 5 {
		t.Fatalf("taste/smell questions = %d, want 5", got)
	}
}

func TestEmbeddedNormSourceIsUsableWithoutExternalFiles(t *testing.T) {
	source, err := loadNormSource("")
	if err != nil {
		t.Fatalf("loadNormSource() error = %v", err)
	}
	if len(source.Factors) != 8 || source.Scores == "" {
		t.Fatalf("embedded source factors=%d scores=%t, want 8 and non-empty", len(source.Factors), source.Scores != "")
	}
	table, _, fallbacks, err := buildNormTable(source, defaultNormVersion, defaultFormVariant)
	if err != nil {
		t.Fatalf("buildNormTable() error = %v", err)
	}
	if len(table.Factors) != 8 || fallbacks != 9 {
		t.Fatalf("embedded norm factors=%d fallbacks=%d, want 8 and 9", len(table.Factors), fallbacks)
	}
}

func testCatalog() normCatalog {
	catalog := normCatalog{byNormName: map[string]string{}, titles: map[string]string{}, order: make([]string, 0, 8)}
	for index := 0; index < 8; index++ {
		code := fmt.Sprintf("f%d", index)
		catalog.order = append(catalog.order, code)
		catalog.titles[code] = code
		catalog.byNormName[normOrder[index]] = code
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
		reverse := index < expectedQuestionCounts["SOC"]
		contents := []string{"从不", "偶尔", "经常", "总是"}
		options := make([]surveyquestionnaire.Option, 0, len(contents))
		for optionIndex, content := range contents {
			score := float64(optionIndex + 1)
			if reverse {
				score = 5 - score
			}
			option, err := surveyquestionnaire.NewOptionWithStringCode(fmt.Sprintf("o%d", optionIndex+1), content, score)
			if err != nil {
				t.Fatalf("NewOptionWithStringCode() error = %v", err)
			}
			options = append(options, option)
		}
		question, err := surveyquestionnaire.NewQuestion(
			surveyquestionnaire.WithCode(meta.NewCode(fmt.Sprintf("q%d", index))),
			surveyquestionnaire.WithStem(fmt.Sprintf("题目 %d", index)),
			surveyquestionnaire.WithQuestionType(surveyquestionnaire.QuestionType("Radio")),
			surveyquestionnaire.WithOptions(options),
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
