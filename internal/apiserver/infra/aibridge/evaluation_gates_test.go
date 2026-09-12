package aibridge

import (
	"context"
	"encoding/json"
	"errors"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"strings"
	"testing"
)

func (s *evaluationRPCStub) PreviewGates(ctx context.Context, r *pb.EvaluationGateQuery, _ ...grpc.CallOption) (*pb.EvaluationGatePreview, error) {
	_, err := s.ListCandidates(ctx, r.Scope)
	s.gateQuery = r
	return s.gatePreview, err
}
func gateResponse() *pb.EvaluationGatePreview {
	return &pb.EvaluationGatePreview{RunId: "run:1", Version: 7, ReleaseFingerprint: "sha256:" + strings.Repeat("a", 64), GateResultJson: `{"schema_version":"qs-ai-evaluation-gate-preview/v1","evaluated_at":"2026-09-13T00:00:00Z","gate_passes":{"G1":true,"G2":true,"G3":true,"G4":false,"G5":false},"metrics":[],"reasons":[],"semantic_adjudications":[]}`}
}
func TestGateClientPreservesAIResultAndValidatesEnvelopeOnce(t *testing.T) {
	rpc := &evaluationRPCStub{t: t, gatePreview: gateResponse()}
	client := &EvaluationClient{RPC: rpc}
	scope := app.EvaluationScope{RunID: "run:1", OrganizationID: 7, OperatorUserID: 42}
	result, err := client.PreviewEvaluationGates(context.Background(), scope, 7)
	if err != nil || string(result.GateResult) != rpc.gatePreview.GateResultJson || result.Version != 7 {
		t.Fatal(result, err)
	}
	if rpc.gateQuery.Scope.OperatorUserId != 42 || rpc.gateQuery.Scope.OrganizationId != 7 || rpc.gateQuery.ExpectedVersion != 7 {
		t.Fatal("scope or version drift")
	}
	for _, mutate := range []func(*pb.EvaluationGatePreview){
		func(r *pb.EvaluationGatePreview) { r.RunId = "other" },
		func(r *pb.EvaluationGatePreview) { r.Version++ },
		func(r *pb.EvaluationGatePreview) { r.ReleaseFingerprint = "bad" },
		func(r *pb.EvaluationGatePreview) { r.GateResultJson = "null" },
		func(r *pb.EvaluationGatePreview) { r.GateResultJson = strings.Repeat(" ", 256*1024+1) },
	} {
		rpc.gatePreview = gateResponse()
		mutate(rpc.gatePreview)
		if _, err := client.PreviewEvaluationGates(context.Background(), scope, 7); !errors.Is(err, app.ErrConflict) {
			t.Fatal(err)
		}
	}
	for _, change := range []func(map[string]any){
		func(r map[string]any) { r["schema_version"] = "unknown" },
		func(r map[string]any) { r["evaluated_at"] = "2026-09-13" },
		func(r map[string]any) { delete(r["gate_passes"].(map[string]any), "G1") },
		func(r map[string]any) { r["gate_passes"].(map[string]any)["G1"] = nil },
		func(r map[string]any) { r["gate_passes"].(map[string]any)["G6"] = true },
		func(r map[string]any) { r["metrics"] = nil },
		func(r map[string]any) { r["reasons"] = nil },
		func(r map[string]any) { r["semantic_adjudications"] = nil },
	} {
		rpc.gatePreview = gateResponse()
		var raw map[string]any
		if err := json.Unmarshal([]byte(rpc.gatePreview.GateResultJson), &raw); err != nil {
			t.Fatal(err)
		}
		change(raw)
		body, _ := json.Marshal(raw)
		rpc.gatePreview.GateResultJson = string(body)
		if _, err := client.PreviewEvaluationGates(context.Background(), scope, 7); !errors.Is(err, app.ErrConflict) {
			t.Fatal(err)
		}
	}
	rpc.fail = context.DeadlineExceeded
	before := rpc.calls
	if _, err := client.PreviewEvaluationGates(context.Background(), scope, 7); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if rpc.calls != before+1 {
		t.Fatal("unexpected retry")
	}
}
