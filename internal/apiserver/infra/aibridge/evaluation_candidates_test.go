package aibridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

func (s *evaluationRPCStub) ListCandidates(ctx context.Context, r *pb.EvaluationQuery, _ ...grpc.CallOption) (*pb.EvaluationCandidateIndex, error) {
	s.calls++
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 5*time.Second {
		s.t.Fatal("read needs deadline")
	}
	s.candidateQuery = &pb.EvaluationCandidateQuery{Scope: r}
	return s.candidateIndex, s.fail
}
func (s *evaluationRPCStub) GetCandidate(ctx context.Context, r *pb.EvaluationCandidateQuery, _ ...grpc.CallOption) (*pb.EvaluationCandidateEvidence, error) {
	_, err := s.ListCandidates(ctx, r.Scope)
	s.candidateQuery = r
	return s.candidateEvidence, err
}
func candidateResponse() *pb.EvaluationCandidateEvidence {
	output := []byte("{\n  \"正文\": \"原文\"\n}")
	semantic := []byte(`{"scores": {}}`)
	fp := func(v []byte) string { sum := sha256.Sum256(v); return "sha256:" + hex.EncodeToString(sum[:]) }
	evidence, _ := json.Marshal(map[string]any{"candidate": map[string]string{"id": "candidate:1", "normalized_output_fingerprint": fp(output)}, "semantic": map[string]string{"output_fingerprint": fp(semantic)}})
	return &pb.EvaluationCandidateEvidence{RunId: "run:1", Version: 7, CandidateId: "candidate:1", NormalizedOutput: output, SemanticOutput: semantic, EvidenceJson: string(evidence)}
}
func TestCandidateClientPreservesExactTextAndRejectsWrongEvidence(t *testing.T) {
	rpc := &evaluationRPCStub{t: t, candidateEvidence: candidateResponse()}
	client := &EvaluationClient{RPC: rpc}
	scope := app.EvaluationScope{RunID: "run:1", OrganizationID: 7, OperatorUserID: 42}
	query := app.CandidateQuery{CandidateID: "candidate:1", ExpectedVersion: 7}
	result, err := client.GetEvaluationCandidate(context.Background(), scope, query)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(result)
	var decoded app.EvaluationCandidateEvidence
	if json.Unmarshal(raw, &decoded) != nil || decoded.NormalizedOutput != string(rpc.candidateEvidence.NormalizedOutput) {
		t.Fatal("original bytes changed in HTTP JSON")
	}
	if rpc.candidateQuery.Scope.OperatorUserId != 42 || rpc.candidateQuery.ExpectedVersion != 7 {
		t.Fatal("query scope drift")
	}
	for _, mutate := range []func(*pb.EvaluationCandidateEvidence){
		func(r *pb.EvaluationCandidateEvidence) { r.RunId = "other" },
		func(r *pb.EvaluationCandidateEvidence) { r.Version++ },
		func(r *pb.EvaluationCandidateEvidence) { r.CandidateId = "other" },
		func(r *pb.EvaluationCandidateEvidence) { r.NormalizedOutput = []byte(`{}`) },
		func(r *pb.EvaluationCandidateEvidence) { r.SemanticOutput = []byte(`{}`) },
		func(r *pb.EvaluationCandidateEvidence) { r.EvidenceJson = "null" },
		func(r *pb.EvaluationCandidateEvidence) { r.EvidenceJson = strings.Repeat(" ", 2*1024*1024) },
	} {
		rpc.candidateEvidence = proto.Clone(candidateResponse()).(*pb.EvaluationCandidateEvidence)
		mutate(rpc.candidateEvidence)
		if _, err := client.GetEvaluationCandidate(context.Background(), scope, query); !errors.Is(err, app.ErrConflict) {
			t.Fatal(err)
		}
	}
	rpc.fail = context.DeadlineExceeded
	before := rpc.calls
	if _, err := client.GetEvaluationCandidate(context.Background(), scope, query); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if rpc.calls != before+1 {
		t.Fatal("unexpected retry")
	}
}
func TestCandidateIndexRejectsDuplicatesAndOversizedLists(t *testing.T) {
	rpc := &evaluationRPCStub{t: t, candidateIndex: &pb.EvaluationCandidateIndex{RunId: "run:1", Version: 7}}
	client := &EvaluationClient{RPC: rpc}
	scope := app.EvaluationScope{RunID: "run:1", OrganizationID: 7, OperatorUserID: 42}
	result, err := client.ListEvaluationCandidates(context.Background(), scope)
	if err != nil || result.Candidates == nil {
		t.Fatal("empty list must be array")
	}
	item := &pb.EvaluationCandidateSummary{CandidateId: "candidate:1", CaseId: "case:1", SlotOrdinal: 1}
	rpc.candidateIndex.Candidates = []*pb.EvaluationCandidateSummary{item}
	if _, err := client.ListEvaluationCandidates(context.Background(), scope); err != nil {
		t.Fatal(err)
	}
	rpc.candidateIndex.Candidates = append(rpc.candidateIndex.Candidates, item)
	if _, err := client.ListEvaluationCandidates(context.Background(), scope); !errors.Is(err, app.ErrConflict) {
		t.Fatal(err)
	}
	rpc.candidateIndex.Candidates = make([]*pb.EvaluationCandidateSummary, 36)
	if _, err := client.ListEvaluationCandidates(context.Background(), scope); !errors.Is(err, app.ErrConflict) {
		t.Fatal(err)
	}
}
