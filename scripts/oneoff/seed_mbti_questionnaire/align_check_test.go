package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestQuestionnaireSeedAlignsWithMBTIModel(t *testing.T) {
	seed := loadQuestionnaireSeed(t)
	model := loadMBTIModelSeed(t)

	if seed.Code != "MBTI_OEJTS" {
		t.Fatalf("seed code = %s, want MBTI_OEJTS", seed.Code)
	}
	if seed.Version != model.QuestionnaireVersion {
		t.Fatalf("seed version = %s, want %s", seed.Version, model.QuestionnaireVersion)
	}

	byCode := make(map[string]questionSeed, len(seed.Questions))
	for _, q := range seed.Questions {
		byCode[q.Code] = q
	}

	if len(model.QuestionMappings) != 32 {
		t.Fatalf("model question_mappings = %d, want 32", len(model.QuestionMappings))
	}
	for _, mapping := range model.QuestionMappings {
		q, ok := byCode[mapping.QuestionCode]
		if !ok {
			t.Fatalf("missing questionnaire question %s", mapping.QuestionCode)
		}
		if len(q.Options) != 5 {
			t.Fatalf("%s options = %d, want 5", mapping.QuestionCode, len(q.Options))
		}
		for i, opt := range q.Options {
			want := float64(i + 1)
			if opt.Score != want {
				t.Fatalf("%s option %s score = %v, want %v", mapping.QuestionCode, opt.Code, opt.Score, want)
			}
		}
	}
}

func loadQuestionnaireSeed(t *testing.T) questionnaireSeedFile {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "mbti_questionnaire.json")
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

func loadMBTIModelSeed(t *testing.T) mbtiModelSeedFile {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
	path := filepath.Join(root, "internal", "apiserver", "infra", "evaluationinput", "seed", "mbti_oejts.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read mbti model seed: %v", err)
	}
	var model mbtiModelSeedFile
	if err := json.Unmarshal(raw, &model); err != nil {
		t.Fatalf("unmarshal mbti model seed: %v", err)
	}
	return model
}
