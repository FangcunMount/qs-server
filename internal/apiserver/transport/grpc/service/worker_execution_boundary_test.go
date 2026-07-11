package service

import (
	"os"
	"strings"
	"testing"
)

func TestInternalServiceUsesWorkerExecutionPortWithoutManagementFacade(t *testing.T) {
	data, err := os.ReadFile("internal.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	if !strings.Contains(source, "workerExecutionService     execute.WorkerExecutionService") {
		t.Fatal("InternalService must depend on the Worker execution port")
	}
	if strings.Contains(source, "AssessmentManagementService") {
		t.Fatal("InternalService must not depend on the legacy Assessment management facade")
	}
}

func TestWorkerEvaluationFlowDoesNotUseOperatorBatchExecution(t *testing.T) {
	data, err := os.ReadFile("internal_assessment_flow.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	if !strings.Contains(source, "s.workerExecutionService.Evaluate(") {
		t.Fatal("EvaluateAssessment must call the Worker execution port")
	}
	if strings.Contains(source, "EvaluateBatch(") {
		t.Fatal("Worker evaluation flow must not call the operator batch execution entrance")
	}
}
