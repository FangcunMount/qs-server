package aibridge

import (
	"context"
	"errors"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"testing"
	"time"
)

type eligibilityRPC struct {
	pb.CommandsClient
	query    *pb.EligibilityQuery
	result   *pb.EligibilityStatus
	err      error
	deadline time.Time
}

func (r *eligibilityRPC) CheckEligibility(ctx context.Context, q *pb.EligibilityQuery, _ ...grpc.CallOption) (*pb.EligibilityStatus, error) {
	r.query = q
	r.deadline, _ = ctx.Deadline()
	return r.result, r.err
}
func TestEligibilityClientPreservesTrustedSourceAndDoesNotSendCommand(t *testing.T) {
	rpc := &eligibilityRPC{result: &pb.EligibilityStatus{Status: "unavailable", ReasonCode: "publication_missing"}}
	client := &Client{RPC: rpc}
	evidence := []app.EvidenceItem{{AssessmentID: "42", TesteeID: "7", ReportID: "99", SourceVersion: "v2:101", Facts: []app.Fact{{Ref: "standard_report", Value: `{"schema_version":"qs-report-snapshot/v2"}`}}}}
	got, err := client.CheckEligibility(context.Background(), app.Actor{OrgID: "1", SubjectID: "parent"}, "7", []string{"42"}, evidence)
	if err != nil || got.ReasonCode != "publication_missing" || rpc.query.Actor.OrgId != "1" || rpc.query.Actor.SubjectId != "parent" || rpc.query.Evidence[0].Facts[0].Value != evidence[0].Facts[0].Value || rpc.deadline.IsZero() || time.Until(rpc.deadline) > 5*time.Second {
		t.Fatalf("capability=%#v err=%v query=%v", got, err, rpc.query)
	}
	for _, result := range []*pb.EligibilityStatus{nil, {Status: "available", ReasonCode: "private"}, {Status: "unavailable", ReasonCode: "private"}, {Status: "other"}} {
		rpc.result = result
		if _, err := client.CheckEligibility(context.Background(), app.Actor{}, "", nil, nil); !errors.Is(err, app.ErrAccessUnavailable) {
			t.Fatal("unsafe response accepted", err)
		}
	}
	rpc.err = status.Error(codes.Unavailable, "private database details")
	if _, err := client.CheckEligibility(context.Background(), app.Actor{}, "", nil, nil); err != app.ErrAccessUnavailable {
		t.Fatal("dependency details leaked", err)
	}
	rpc.err = status.Error(codes.PermissionDenied, "private identity")
	if _, err := client.CheckEligibility(context.Background(), app.Actor{}, "", nil, nil); err != app.ErrAccessDenied {
		t.Fatal("permission error lost", err)
	}
}
