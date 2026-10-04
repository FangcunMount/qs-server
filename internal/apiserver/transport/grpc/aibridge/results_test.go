package aibridge

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"sync/atomic"
	"testing"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type retiredResultStore struct {
	app.Store
	writes atomic.Int32
}

func (s *retiredResultStore) Accept(context.Context, app.Event) error {
	s.writes.Add(1)
	return nil
}

func TestMQRetiredResultsRPCNeverPersistsOrAcknowledges(t *testing.T) {
	store := &retiredResultStore{}
	r := &Receiver{Service: &app.Service{Store: store}}
	cert := &x509.Certificate{Subject: pkix.Name{CommonName: "qs-ai.svc"}}
	ctx := peer.NewContext(t.Context(), &peer.Peer{AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}, VerifiedChains: [][]*x509.Certificate{{cert}}}}})
	e := &pb.StateEvent{
		EventId: "11111111-1111-4111-8111-111111111111", RequestId: "22222222-2222-4222-8222-222222222222",
		SessionId: "33333333-3333-4333-8333-333333333333", Version: 1, Status: "queued",
		Actor: &pb.Actor{OrgId: "1", SubjectId: "42"}, TesteeId: "7",
	}
	for _, event := range []*pb.StateEvent{e, {}, nil} {
		ack, err := r.Accept(ctx, event)
		if status.Code(err) != codes.FailedPrecondition || ack != nil || store.writes.Load() != 0 {
			t.Fatalf("retired RPC persisted/acknowledged: code=%v ack=%v writes=%d", status.Code(err), ack, store.writes.Load())
		}
	}
	// No configured business service may turn the retired route back on.
	r.Service = nil
	ack, err := r.Accept(ctx, e)
	if status.Code(err) != codes.FailedPrecondition || ack != nil {
		t.Fatal("nil service changed permanent rejection", ack, err)
	}
}

func TestResultReceiverRegistersCanonicalServiceAndChecksCallerBeforeStore(t *testing.T) {
	r := &Receiver{Service: &app.Service{}}
	server := grpc.NewServer()
	t.Cleanup(server.Stop)
	r.RegisterService(server)
	info, ok := server.GetServiceInfo()["qsai.workflow.v1.Results"]
	if !ok || len(info.Methods) != 1 || info.Methods[0].Name != "Accept" {
		t.Fatal("result service missing")
	}
	for _, cn := range []string{"", "qs-apiserver.svc", "qs-ai.svc"} {
		t.Run(cn, func(t *testing.T) {
			ctx := context.Background()
			if cn != "" {
				cert := &x509.Certificate{Subject: pkix.Name{CommonName: cn}}
				ctx = peer.NewContext(ctx, &peer.Peer{AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}, VerifiedChains: [][]*x509.Certificate{{cert}}}}})
			}
			_, err := r.Accept(ctx, &pb.StateEvent{})
			want := codes.PermissionDenied
			if cn == "qs-ai.svc" {
				want = codes.FailedPrecondition
			}
			if status.Code(err) != want {
				t.Fatalf("status = %v, want %v", status.Code(err), want)
			}
		})
	}
}
