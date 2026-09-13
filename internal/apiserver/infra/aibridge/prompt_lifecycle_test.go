package aibridge

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type lifecycleRPCStub struct {
	pb.PromptDraftManagementClient
	response *pb.PromptDraftLifecycle
	err      error
	calls    int
	query    *pb.PromptDraftQuery
	deadline bool
}

func (s *lifecycleRPCStub) GetLifecycle(ctx context.Context, q *pb.PromptDraftQuery, _ ...grpc.CallOption) (*pb.PromptDraftLifecycle, error) {
	s.calls++
	s.query = q
	end, ok := ctx.Deadline()
	s.deadline = ok && time.Until(end) <= 5*time.Second
	return s.response, s.err
}
func lifecycleTestResponse() *pb.PromptDraftLifecycle {
	d := draftTestState()
	return &pb.PromptDraftLifecycle{SchemaVersion: "qs-ai-prompt-lifecycle/v1", Draft: draftTestResponse(d), Status: "frozen", Frozen: &pb.PromptDraftFrozenVersion{Asset: &pb.PromptDraftSource{Identity: d.TemplateID, Version: d.TargetVersion, Fingerprint: d.Source.Fingerprint, ContentSha256: d.Source.ContentSHA256}, Revision: d.Revision, FrozenAt: d.SavedAt}}
}
func TestPromptLifecycleBindsCurrentHeadAndFrozenIdentity(t *testing.T) {
	for _, bad := range []string{"", "editable", "unknown", "schema", "missing", "unexpected", "revision", "identity", "version", "hash", "time", "oldtime", "draft", "org", "oversize", "nil", "timeout"} {
		t.Run(bad, func(t *testing.T) {
			r := lifecycleTestResponse()
			s := &lifecycleRPCStub{response: r}
			switch bad {
			case "editable":
				r.Status = "editable"
				r.Frozen = nil
			case "unknown":
				r.Status = "future"
			case "schema":
				r.SchemaVersion = "other"
			case "missing":
				r.Frozen = nil
			case "unexpected":
				r.Status = "editable"
			case "revision":
				r.Frozen.Revision++
			case "identity":
				r.Frozen.Asset.Identity = "other"
			case "version":
				r.Frozen.Asset.Version = "v3"
			case "hash":
				r.Frozen.Asset.ContentSha256 = "bad"
			case "time":
				r.Frozen.FrozenAt = "bad"
			case "oldtime":
				r.Frozen.FrozenAt = "2020-01-01T00:00:00Z"
			case "draft":
				r.Draft.DraftId = "different"
			case "org":
				d := draftTestState()
				d.OrganizationID = 8
				r.Draft = draftTestResponse(d)
			case "oversize":
				r.Draft.SnapshotJson = strings.Repeat(" ", 260*1024)
			case "nil":
				s.response = nil
			case "timeout":
				s.err = status.Error(codes.DeadlineExceeded, "private")
			}
			v, err := (&PromptDraftClient{RPC: s}).GetPromptDraftLifecycle(context.Background(), app.DraftScope{OrganizationID: 7, OperatorUserID: 43}, publicationTestID)
			if s.calls != 1 || !s.deadline || s.query.Revision != nil || s.query.Scope.OperatorUserId != 43 {
				t.Fatal("query scope/deadline/retry", s)
			}
			if bad == "" || bad == "editable" {
				if err != nil || v.Draft.Revision != 1 || v.Status != r.Status {
					t.Fatal(v, err)
				}
				return
			}
			if bad == "timeout" {
				if status.Code(err) != codes.DeadlineExceeded {
					t.Fatal(err)
				}
				return
			}
			if !errors.Is(err, app.ErrConflict) {
				t.Fatal("invalid lifecycle accepted", v, err)
			}
		})
	}
}
