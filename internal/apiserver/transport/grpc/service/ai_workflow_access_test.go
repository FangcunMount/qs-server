package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/interpretation"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"testing"
)

type aiAccessStub struct {
	calls int
	err   error
}

func (a *aiAccessStub) Authorize(_ context.Context, actor app.Actor, id string, ids []string) error {
	a.calls++
	if actor.SubjectID != "parent" || id != "7" || len(ids) != 1 || ids[0] != "42" {
		panic("request mapping")
	}
	return a.err
}
func TestAIWorkflowAccessIdentityAndErrors(t *testing.T) {
	for _, tc := range []struct {
		name, cn string
		verified bool
		err      error
		want     codes.Code
	}{
		{"allowed", "qs-ai.svc", true, nil, codes.OK}, {"wrong workload", "qs-apiserver.svc", true, nil, codes.PermissionDenied}, {"unverified", "qs-ai.svc", false, nil, codes.PermissionDenied}, {"invalid", "qs-ai.svc", true, app.ErrInvalid, codes.InvalidArgument}, {"revoked", "qs-ai.svc", true, app.ErrAccessDenied, codes.PermissionDenied}, {"offline", "qs-ai.svc", true, app.ErrAccessUnavailable, codes.Unavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cert := &x509.Certificate{Subject: pkix.Name{CommonName: tc.cn}}
			state := tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}
			if tc.verified {
				state.VerifiedChains = [][]*x509.Certificate{{cert}}
			}
			ctx := peer.NewContext(context.Background(), &peer.Peer{AuthInfo: credentials.TLSInfo{State: state}})
			a := &aiAccessStub{err: tc.err}
			s := &AIWorkflowAccessService{Access: a}
			_, err := s.Authorize(ctx, &pb.AIWorkflowAccessRequest{OrgId: "1", SubjectId: "parent", TesteeId: "7", AssessmentIds: []string{"42"}})
			if status.Code(err) != tc.want {
				t.Fatalf("got %v want %v", err, tc.want)
			}
			if (!tc.verified || tc.cn != "qs-ai.svc") && a.calls != 0 {
				t.Fatal("untrusted caller reached access service")
			}
		})
	}
	s := &AIWorkflowAccessService{Access: &aiAccessStub{}}
	if _, err := s.Authorize(context.Background(), nil); status.Code(err) != codes.PermissionDenied {
		t.Fatal(err)
	}
}
