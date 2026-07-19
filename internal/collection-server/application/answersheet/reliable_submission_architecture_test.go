package answersheet

import (
	"os"
	"strings"
	"testing"
)

func TestReliableSubmissionArchitectureHasNoInProcessQueueOrSynchronousAssessment(t *testing.T) {
	source, err := os.ReadFile("submission_service.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, forbidden := range []string{"SubmitQueued", "SubmitQueue", ".EnsureAssessment("} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("reliable submission source contains forbidden legacy path %q", forbidden)
		}
	}
	if !strings.Contains(text, "AcceptDurably") {
		t.Fatal("reliable submission source must expose AcceptDurably")
	}
}

func TestReliableSubmissionArchitectureKeepsReadinessOwnershipContract(t *testing.T) {
	grpcSource, err := os.ReadFile("../../../apiserver/transport/grpc/service/answersheet.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(grpcSource), "TesteeId:") {
		t.Fatal("gRPC AnswerSheet mapper must retain testee ownership for readiness")
	}

	acceptanceDoc, err := os.ReadFile("../../../../docs/02-业务模块/10-survey/31-关键链路-答卷校验与可靠受理.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"collection 同步 EnsureAssessment",
		"collection 当前同步等待 Assessment",
		"collection `SubmissionService.submitSync`",
		"Assessment ID 已同步返回",
		"同步调用返错",
	} {
		if strings.Contains(string(acceptanceDoc), forbidden) {
			t.Fatalf("active reliable-submit documentation contains stale statement %q", forbidden)
		}
	}
	for _, required := range []string{"202 accepted + request_id + answersheet_id"} {
		if !strings.Contains(string(acceptanceDoc), required) {
			t.Fatalf("active reliable-submit documentation is missing required statement %q", required)
		}
	}

	executionDoc, err := os.ReadFile("../../../../docs/02-业务模块/10-survey/32-关键链路-从作答事实到测评执行.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(executionDoc), "Worker 是正常创建与恢复 Assessment 的唯一入口") {
		t.Fatal("active execution documentation must retain Worker ownership contract")
	}
}
