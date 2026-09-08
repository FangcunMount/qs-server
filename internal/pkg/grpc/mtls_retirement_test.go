package grpc_test

import (
	"context"
	"net"
	"testing"
	"time"

	basegrpc "github.com/FangcunMount/component-base/pkg/grpc/interceptors"
	pb "github.com/FangcunMount/qs-server/api/grpc/gen/questionnaire"
	collection "github.com/FangcunMount/qs-server/internal/collection-server/infra/grpcclient"
	servergrpc "github.com/FangcunMount/qs-server/internal/pkg/grpc"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/admission"
	"github.com/FangcunMount/qs-server/internal/testutil/tlsfixture"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type certificateQuestionnaireServer struct {
	pb.UnimplementedQuestionnaireServiceServer
}

func (*certificateQuestionnaireServer) ListQuestionnaires(ctx context.Context, _ *pb.ListQuestionnairesRequest) (*pb.ListQuestionnairesResponse, error) {
	identity, ok := basegrpc.ServiceIdentityFromContext(ctx)
	if !ok || identity.ServiceName != "qs-collection-server.svc" {
		return nil, status.Error(codes.PermissionDenied, "unexpected identity")
	}
	return &pb.ListQuestionnairesResponse{}, nil
}
func TestCollectionMTLSWithoutServiceToken(t *testing.T) {
	ca := tlsfixture.New(t)
	serverPair := ca.Issue(t, "server.test", false)
	cfg := &servergrpc.Config{TLSCertFile: serverPair.CertFile, TLSKeyFile: serverPair.KeyFile, MTLS: servergrpc.MTLSConfig{Enabled: true, CAFile: ca.CAFile, RequireClientCert: true}, ACL: servergrpc.ACLConfig{Enabled: true, ConfigFile: "../../../configs/grpc-acl.prod.yaml", DefaultPolicy: "deny"}}
	srv, err := servergrpc.NewServer(cfg, nil)
	require.NoError(t, err)
	t.Cleanup(srv.Server.Stop)
	pb.RegisterQuestionnaireServiceServer(srv.Server, &certificateQuestionnaireServer{})
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = lis.Close() })
	go func() { _ = srv.Server.Serve(lis) }()
	valid := ca.Issue(t, "qs-collection-server.svc", false)
	manager, err := collection.NewManager(&collection.ManagerConfig{Endpoint: lis.Addr().String(), Timeout: time.Second, InflightSemaphore: admission.NewChannelSemaphore(2), TLSCertFile: valid.CertFile, TLSKeyFile: valid.KeyFile, TLSCAFile: ca.CAFile, TLSServerName: "server.test"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = manager.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = pb.NewQuestionnaireServiceClient(manager.Conn()).ListQuestionnaires(ctx, &pb.ListQuestionnairesRequest{})
	require.NoError(t, err)
	unknown := ca.Issue(t, "unknown.svc", false)
	expired := ca.Issue(t, "qs-collection-server.svc", true)
	rogue := tlsfixture.New(t).Issue(t, "qs-collection-server.svc", false)
	for _, tt := range []struct {
		name string
		pair *tlsfixture.Pair
		want codes.Code
	}{
		{"forged bearer does not change certificate identity", &valid, codes.OK}, {"unknown identity", &unknown, codes.PermissionDenied}, {"missing certificate", nil, codes.Unavailable}, {"expired certificate", &expired, codes.Unavailable}, {"untrusted certificate", &rogue, codes.Unavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(credentials.NewTLS(ca.Client(tt.pair))))
			require.NoError(t, err)
			defer conn.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer forged-admin-token")
			_, err = pb.NewQuestionnaireServiceClient(conn).ListQuestionnaires(ctx, &pb.ListQuestionnairesRequest{})
			require.Equal(t, tt.want, status.Code(err))
		})
	}
}
