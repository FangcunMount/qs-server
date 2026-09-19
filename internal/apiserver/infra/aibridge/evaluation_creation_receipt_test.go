package aibridge

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
)

func creationFixture() app.EvaluationCreationReceipt {
	plan := planFixture()
	return app.EvaluationCreationReceipt{SchemaVersion: "qs-ai-evaluation-creation-receipt/v1", RunID: "6c346590-3a9e-478e-a270-18c8ef10e0ce", Release: plan.Release, ReleaseFingerprint: plan.ReleaseFingerprint, RequestedBy: "user:42", RequestReason: "创建评测", CreatedAt: "2026-09-13T00:00:00+00:00"}
}

func creationState(value app.EvaluationCreationReceipt) *pb.EvaluationState {
	raw, _ := json.Marshal(value)
	return &pb.EvaluationState{ReviewsJson: "[]", ReopeningsJson: "[]", RunId: creationFixture().RunID, Version: 7, Status: "collecting", ResolutionsJson: "[]", CreationJson: string(raw)}
}

func TestCreationRecoveryRetainsOriginalActorAndReferences(t *testing.T) {
	expected := creationFixture()
	response := creationState(expected)
	// A different auditor can recover the creation; the read must not replace its author.
	view, err := state(response, app.EvaluationScope{RunID: expected.RunID, OrganizationID: 1, OperatorUserID: 99})
	if err != nil || view.Creation == nil || *view.Creation != expected || view.Version != 7 {
		t.Fatal(view, err)
	}
	response.CreationJson = ""
	view, err = state(response, app.EvaluationScope{RunID: expected.RunID})
	if err != nil || view.Creation != nil {
		t.Fatal("old servers must not fabricate recovery evidence", view, err)
	}
}

func TestCreationRecoveryRejectsCorruptOrMismatchedReceipt(t *testing.T) {
	for name, change := range map[string]func(*app.EvaluationCreationReceipt){
		"schema":            func(r *app.EvaluationCreationReceipt) { r.SchemaVersion = "unknown" },
		"run":               func(r *app.EvaluationCreationReceipt) { r.RunID = "6c346590-3a9e-478e-a270-18c8ef10e0cf" },
		"reference":         func(r *app.EvaluationCreationReceipt) { r.Release.SemanticPrompt.Version = "v2" },
		"missing reference": func(r *app.EvaluationCreationReceipt) { r.Release.Suite = app.FrozenEvaluationRef{} },
		"fingerprint":       func(r *app.EvaluationCreationReceipt) { r.ReleaseFingerprint = "sha256:" + strings.Repeat("0", 64) },
		"actor":             func(r *app.EvaluationCreationReceipt) { r.RequestedBy = "" },
		"reason":            func(r *app.EvaluationCreationReceipt) { r.RequestReason = strings.Repeat("中", 334) },
		"time":              func(r *app.EvaluationCreationReceipt) { r.CreatedAt = "2026-09-13T00:00:00" },
	} {
		t.Run(name, func(t *testing.T) {
			value := creationFixture()
			change(&value)
			if _, err := evaluationCreation(creationState(value)); !errors.Is(err, app.ErrConflict) {
				t.Fatal(err)
			}
		})
	}
	for _, raw := range []string{"null", "{}", "[]", "\xff", strings.Repeat(" ", 16385), creationState(creationFixture()).CreationJson + "{}"} {
		response := creationState(creationFixture())
		response.CreationJson = raw
		if _, err := evaluationCreation(response); !errors.Is(err, app.ErrConflict) {
			t.Fatal(err)
		}
	}
}

type creationReceiptRPC struct {
	pb.EvaluationManagementClient
	response *pb.EvaluationState
	calls    int
}

func (s *creationReceiptRPC) Create(context.Context, *pb.EvaluationCreateCommand, ...grpc.CallOption) (*pb.EvaluationState, error) {
	s.calls++
	return s.response, nil
}

func TestCreateMatchesReceiptToOriginalCommandWithoutRetry(t *testing.T) {
	value := creationFixture()
	for _, kind := range []string{"valid", "actor", "reason", "release"} {
		t.Run(kind, func(t *testing.T) {
			command := app.EvaluationCreate{Release: value.Release, Reason: " 创建评测 ", Confirm: true}
			scope := app.EvaluationScope{RunID: value.RunID, OrganizationID: 1, OperatorUserID: 42}
			if kind == "actor" {
				scope.OperatorUserID = 43
			}
			if kind == "reason" {
				command.Reason = "另一目的"
			}
			if kind == "release" {
				command.Release.Prompt.Version = "v2"
			}
			rpc := &creationReceiptRPC{response: creationState(value)}
			_, err := (&EvaluationClient{RPC: rpc}).CreateEvaluation(context.Background(), scope, command)
			if rpc.calls != 1 || (kind == "valid" && err != nil) || (kind != "valid" && !errors.Is(err, app.ErrConflict)) {
				t.Fatal(err, rpc.calls)
			}
		})
	}
}
