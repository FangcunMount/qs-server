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

	activeDoc, err := os.ReadFile("../../../../docs/02-业务模块/10-survey/31-关键链路-答卷提交校验与测评驱动.md")
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
		if strings.Contains(string(activeDoc), forbidden) {
			t.Fatalf("active reliable-submit documentation contains stale statement %q", forbidden)
		}
	}
	for _, required := range []string{"202 accepted + answersheet_id", "Worker 是正常创建与恢复 Assessment 的唯一入口"} {
		if !strings.Contains(string(activeDoc), required) {
			t.Fatalf("active reliable-submit documentation is missing required statement %q", required)
		}
	}
}
