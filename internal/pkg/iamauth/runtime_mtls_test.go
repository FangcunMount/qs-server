package iamauth_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"testing"
	"time"

	base "github.com/FangcunMount/component-base/pkg/grpc/interceptors"
	authnv3 "github.com/FangcunMount/iam/v5/api/grpc/iam/authn/v3"
	authzv4 "github.com/FangcunMount/iam/v5/api/grpc/iam/authz/v4"
	identityv2 "github.com/FangcunMount/iam/v5/api/grpc/iam/identity/v2"
	module "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/iam"
	collection "github.com/FangcunMount/qs-server/internal/collection-server/container"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/testutil/tlsfixture"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type startupIAM struct {
	authnv3.UnimplementedAuthServiceServer
	authzv4.UnimplementedAuthorizationServiceServer
	identityv2.UnimplementedProfileLinkQueryServer
}

func (*startupIAM) VerifyToken(context.Context, *authnv3.VerifyTokenRequest) (*authnv3.VerifyTokenResponse, error) {
	return nil, status.Error(codes.InvalidArgument, "empty token")
}
func (*startupIAM) GetAuthorizationSnapshot(context.Context, *authzv4.GetAuthorizationSnapshotRequest) (*authzv4.GetAuthorizationSnapshotResponse, error) {
	return &authzv4.GetAuthorizationSnapshotResponse{PolicyVersion: 1}, nil
}
func (*startupIAM) ListProfiles(context.Context, *identityv2.ListProfilesRequest) (*identityv2.ListProfilesResponse, error) {
	return &identityv2.ListProfilesResponse{}, nil
}

func TestRequiredRuntimesUseCertificateAndRealRPCWithoutBearer(t *testing.T) {
	ca := tlsfixture.New(t)
	pair := ca.Issue(t, "server.test", false)
	roots := x509.NewCertPool()
	roots.AddCert(ca.Cert)
	methods := []string{"/iam.authn.v3.AuthService/VerifyToken", "/iam.authz.v4.AuthorizationService/GetAuthorizationSnapshot", "/iam.identity.v2.ProfileLinkQuery/ListProfiles"}
	acl := base.NewServiceACL(&base.ACLConfig{DefaultPolicy: "deny", Services: []*base.ServicePermissions{{ServiceName: "qs-apiserver.svc", Enabled: true, AllowedMethods: methods}, {ServiceName: "qs-collection-server.svc", Enabled: true, AllowedMethods: methods}}})
	calls := make(chan string, 20)
	srv := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{pair.Certificate}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots})), grpc.ChainUnaryInterceptor(base.MTLSInterceptor(), base.ACLInterceptor(acl), func(ctx context.Context, r interface{}, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (interface{}, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		if len(md.Get("authorization")) != 0 {
			return nil, status.Error(codes.PermissionDenied, "unexpected bearer")
		}
		calls <- info.FullMethod
		return h(ctx, r)
	}))
	fixture := &startupIAM{}
	authnv3.RegisterAuthServiceServer(srv, fixture)
	authzv4.RegisterAuthorizationServiceServer(srv, fixture)
	identityv2.RegisterProfileLinkQueryServer(srv, fixture)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(srv.Stop)
	go func() { _ = srv.Serve(lis) }()
	for _, name := range []string{"qs-apiserver.svc", "qs-collection-server.svc", "unknown.svc"} {
		t.Run(name, func(t *testing.T) {
			clientPair := ca.Issue(t, name, false)
			opts := options.NewIAMOptions()
			opts.Enabled = true
			opts.GRPCEnabled = true
			opts.JWKSEnabled = false
			opts.GRPC.Address = lis.Addr().String()
			opts.GRPC.Timeout = time.Second
			// The server certificate needs the dial address as SAN because the IAM SDK derives ServerName from Endpoint.
			opts.GRPC.TLS = &options.IAMTLSOptions{Enabled: true, CAFile: ca.CAFile, CertFile: clientPair.CertFile, KeyFile: clientPair.KeyFile}
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			if name == "qs-collection-server.svc" {
				m, err := collection.NewIAMModule(ctx, opts)
				require.NoError(t, err)
				defer func() { _ = m.Close() }()
				require.NoError(t, m.ValidateRequiredRuntime(ctx))
			} else {
				m, err := module.NewWithRuntimeOptions(ctx, opts, module.RuntimeOptions{})
				require.NoError(t, err)
				defer func() { _ = m.Close() }()
				err = m.ValidateRequiredAuthzRuntime(ctx)
				if name == "unknown.svc" {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
			}
		})
	}
	seen := map[string]bool{}
	for len(calls) > 0 {
		seen[<-calls] = true
	}
	for _, method := range methods {
		require.True(t, seen[method], method)
	}
}
