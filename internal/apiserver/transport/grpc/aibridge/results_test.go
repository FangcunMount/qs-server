package aibridge

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"

	pb "github.com/FangcunMount/qs-server/api/grpc/gen/aiworkflow"
	app "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

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
				want = codes.InvalidArgument
			}
			if status.Code(err) != want {
				t.Fatalf("status = %v, want %v", status.Code(err), want)
			}
		})
	}
}
