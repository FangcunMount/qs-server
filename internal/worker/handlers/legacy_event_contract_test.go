package handlers

import (
	"os"
	"strings"
	"testing"
)

func TestEvaluationAndInterpretationHandlersDoNotUseRetiredPayloadTypes(t *testing.T) {
	t.Parallel()

	forbidden := []string{
		"AssessmentInterpretedPayload",
		"AssessmentSubmittedData",
		"AssessmentEvaluatedData",
		"AssessmentFailedData",
	}
	for _, path := range []string{"assessment_handler.go", "report_handler.go"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(data)
		for _, token := range forbidden {
			if strings.Contains(text, token) {
				t.Fatalf("%s contains %q; interpreted/report handlers must use eventoutcome payloads only", path, token)
			}
		}
	}
}
