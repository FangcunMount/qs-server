package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestQuestionnaireSeedAlignsWithSBTIModel(t *testing.T) {
	seed := loadQuestionnaireSeed(t)
	model := loadSBTIModelSeed(t)

	if seed.Code != "SBTI_FUN" {
		t.Fatalf("seed code = %s, want SBTI_FUN", seed.Code)
	}
	if seed.Version != model.QuestionnaireVersion {
		t.Fatalf("seed version = %s, want %s", seed.Version, model.QuestionnaireVersion)
	}

	byCode := make(map[string]questionSeed, len(seed.Questions))
	for _, q := range seed.Questions {
		byCode[q.Code] = q
	}

	if len(model.QuestionMappings) != 30 {
		t.Fatalf("model question_mappings = %d, want 30", len(model.QuestionMappings))
	}
	for i, mapping := range model.QuestionMappings {
		wantCode := "SBTI_Q" + strings.TrimLeft(strconv.Itoa(i+1), "0")
		if i < 9 {
			wantCode = "SBTI_Q0" + strconv.Itoa(i+1)
		}
		q, ok := byCode[mapping.QuestionCode]
		if !ok {
			t.Fatalf("missing questionnaire question %s", mapping.QuestionCode)
		}
		if len(q.Options) != 3 {
			t.Fatalf("%s options = %d, want 3", mapping.QuestionCode, len(q.Options))
		}
		for _, opt := range q.Options {
			if _, ok := mapping.OptionScores[opt.Code]; !ok {
				t.Fatalf("%s option code %s not in option_scores", mapping.QuestionCode, opt.Code)
			}
		}
		_ = wantCode
	}

	drink, ok := byCode["drink_gate_q2"]
	if !ok {
		t.Fatal("missing drink_gate_q2")
	}
	if len(drink.Options) != 2 {
		t.Fatalf("drink_gate_q2 options = %d, want 2", len(drink.Options))
	}
	triggerValues := make(map[string]struct{})
	for _, v := range model.DrinkTrigger.OptionValues {
		triggerValues[v] = struct{}{}
	}
	foundTrigger := false
	for _, opt := range drink.Options {
		if _, ok := triggerValues[opt.Code]; ok {
			foundTrigger = true
		}
	}
	if !foundTrigger {
		t.Fatalf("drink_gate_q2 options %#v do not match drink_trigger %#v", drink.Options, model.DrinkTrigger.OptionValues)
	}
}

func loadQuestionnaireSeed(t *testing.T) questionnaireSeedFile {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "sbti_questionnaire.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read questionnaire seed: %v", err)
	}
	var seed questionnaireSeedFile
	if err := json.Unmarshal(raw, &seed); err != nil {
		t.Fatalf("unmarshal questionnaire seed: %v", err)
	}
	return seed
}

func loadSBTIModelSeed(t *testing.T) sbtiModelSeedFile {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	path := filepath.Join(root, "internal", "apiserver", "infra", "evaluationinput", "seed", "sbti_fun.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read sbti model seed: %v", err)
	}
	var model sbtiModelSeedFile
	if err := json.Unmarshal(raw, &model); err != nil {
		t.Fatalf("unmarshal sbti model seed: %v", err)
	}
	return model
}
