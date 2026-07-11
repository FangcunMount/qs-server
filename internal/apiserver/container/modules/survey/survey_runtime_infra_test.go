package survey

import (
	"os"
	"strings"
	"testing"
)

func TestSurveyRuntimeInfraDoesNotOwnModelCatalogRepository(t *testing.T) {
	content, err := os.ReadFile("survey_runtime_infra.go")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(content), "Model"+"Repository") {
		t.Fatal("SurveyRuntimeInfra must not own a draft assessment-model repository")
	}
}
