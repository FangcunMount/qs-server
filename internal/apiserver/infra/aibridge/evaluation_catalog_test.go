package aibridge

import (
	"context"
	"encoding/base64"
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

func (s *evaluationRPCStub) List(context.Context, *pb.EvaluationCatalogQuery, ...grpc.CallOption) (*pb.EvaluationCatalogPage, error) {
	s.t.Fatal("unexpected catalog call in another operation")
	return nil, app.ErrConflict
}

type catalogRPC struct {
	pb.EvaluationManagementClient
	t        *testing.T
	calls    int
	request  *pb.EvaluationCatalogQuery
	response *pb.EvaluationCatalogPage
	err      error
}

func (s *catalogRPC) List(ctx context.Context, query *pb.EvaluationCatalogQuery, _ ...grpc.CallOption) (*pb.EvaluationCatalogPage, error) {
	s.calls++
	s.request = query
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 5*time.Second {
		s.t.Fatal("missing bounded deadline")
	}
	return s.response, s.err
}
func catalogSummary() *pb.EvaluationSummary {
	return &pb.EvaluationSummary{RunId: "00000000-0000-4000-8000-000000000003", OrganizationId: 7, Version: 1, Status: "requested", CreatedAt: "2026-09-13T08:00:00.123456+00:00", RequestedBy: "user:42", ProfileId: "profile", ProfileVersion: "v6", PromptId: "prompt", PromptVersion: "v6", ReleaseFingerprint: "sha256:" + strings.Repeat("a", 64), RequiredCandidates: 5, AcceptedCandidates: 3, ReviewReadyCandidates: 2}
}
func runCursor(org int64, status string, row *pb.EvaluationSummary) string {
	raw, _ := json.Marshal([]any{1, "evaluation", org, status, row.CreatedAt, row.RunId})
	return base64.URLEncoding.EncodeToString(raw)
}
func TestEvaluationCatalogClientForwardsScopeAndChecksContinuation(t *testing.T) {
	row := catalogSummary()
	rpc := &catalogRPC{t: t, response: &pb.EvaluationCatalogPage{Items: []*pb.EvaluationSummary{row}, NextCursor: runCursor(7, "requested", row)}}
	client := &EvaluationClient{RPC: rpc}
	scope := app.DraftScope{OrganizationID: 7, OperatorUserID: 42}
	query := app.EvaluationCatalogQuery{Status: "requested", Limit: 1}
	page, err := client.ListEvaluations(context.Background(), scope, query)
	if err != nil || len(page.Items) != 1 || rpc.calls != 1 || rpc.request.Scope.OrganizationId != 7 || rpc.request.Scope.OperatorUserId != 42 || page.Items[0].RequiredCandidates != 5 {
		t.Fatal(page, err, rpc)
	}
	query.Cursor = page.NextCursor
	next := proto.Clone(row).(*pb.EvaluationSummary)
	next.RunId = "00000000-0000-4000-8000-000000000002"
	rpc.response = &pb.EvaluationCatalogPage{Items: []*pb.EvaluationSummary{next}}
	if _, err = client.ListEvaluations(context.Background(), scope, query); err != nil {
		t.Fatal(err)
	}
	rpc.err = context.DeadlineExceeded
	if _, err = client.ListEvaluations(context.Background(), scope, query); !errors.Is(err, context.DeadlineExceeded) || rpc.calls != 3 {
		t.Fatal("unexpected retry", err, rpc.calls)
	}
}
func TestEvaluationCatalogRejectsScopeOrderAndMalformedSummary(t *testing.T) {
	for _, name := range []string{"scope", "filter", "duplicate", "order", "version", "time", "counts", "cursor", "cursor_filter", "oversize", "nil"} {
		t.Run(name, func(t *testing.T) {
			row := catalogSummary()
			older := proto.Clone(row).(*pb.EvaluationSummary)
			older.RunId = "00000000-0000-4000-8000-000000000002"
			raw := &pb.EvaluationCatalogPage{Items: []*pb.EvaluationSummary{row, older}}
			query := app.EvaluationCatalogQuery{Status: "requested", Limit: 2}
			switch name {
			case "scope":
				row.OrganizationId = 8
			case "filter":
				row.Status = "approved"
			case "duplicate":
				raw.Items[1] = row
			case "order":
				raw.Items = []*pb.EvaluationSummary{older, row}
			case "version":
				row.Version = 0
			case "time":
				row.CreatedAt = "bad"
			case "counts":
				row.ReviewReadyCandidates = 6
			case "cursor":
				raw.NextCursor = runCursor(7, "requested", row)
			case "cursor_filter":
				raw.NextCursor = runCursor(7, "approved", older)
			case "oversize":
				row.LastReason = strings.Repeat("x", 256*1024)
			case "nil":
				raw.Items[0] = nil
			}
			if _, err := evaluationCatalogPage(raw, app.DraftScope{OrganizationID: 7, OperatorUserID: 42}, query); !errors.Is(err, app.ErrConflict) {
				t.Fatal(name, err)
			}
		})
	}
	empty, err := evaluationCatalogPage(&pb.EvaluationCatalogPage{}, app.DraftScope{OrganizationID: 7}, app.EvaluationCatalogQuery{Limit: 20})
	if err != nil || empty.Items == nil || len(empty.Items) != 0 {
		t.Fatal(empty, err)
	}
}
