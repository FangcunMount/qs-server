package service

import (
	"os"
	"strings"
	"testing"
)

func TestEvaluationWorkerServiceUsesWorkerActorPort(t *testing.T) {
	data, err := os.ReadFile("evaluation_worker.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	if !strings.Contains(source, "service evaluationworker.Service") {
		t.Fatal("EvaluationWorkerService must depend on the Worker actor port")
	}
	if strings.Contains(source, "AssessmentManagementService") {
		t.Fatal("InternalService must not depend on the legacy Assessment management facade")
	}
}

func TestWorkerEvaluationFlowDoesNotUseOperatorBatchExecution(t *testing.T) {
	data, err := os.ReadFile("evaluation_worker.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	if !strings.Contains(source, "s.service.Execute(") {
		t.Fatal("ExecuteEvaluation must call the Worker actor service")
	}
	if strings.Contains(source, "EvaluateBatch(") {
		t.Fatal("Worker evaluation flow must not call the operator batch execution entrance")
	}
}
