package iamauth_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"strings"
	"sync/atomic"
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
	committedVersion atomic.Int64
	runtimeVersion   atomic.Int64
}

func (*startupIAM) VerifyToken(context.Context, *authnv3.VerifyTokenRequest) (*authnv3.VerifyTokenResponse, error) {
	return nil, status.Error(codes.InvalidArgument, "empty token")
}
func (s *startupIAM) GetAuthorizationSnapshot(context.Context, *authzv4.GetAuthorizationSnapshotRequest) (*authzv4.GetAuthorizationSnapshotResponse, error) {
	version := s.runtimeVersion.Load()
	if version == 0 {
		version = 1
	}
	return &authzv4.GetAuthorizationSnapshotResponse{PolicyVersion: version}, nil
}
func (s *startupIAM) GetCommittedPolicyVersion(context.Context, *authzv4.GetCommittedPolicyVersionRequest) (*authzv4.GetCommittedPolicyVersionResponse, error) {
	version := s.committedVersion.Load()
	if version == 0 {
		return nil, status.Error(codes.Unimplemented, "older IAM server")
	}
	return &authzv4.GetCommittedPolicyVersionResponse{PolicyVersion: version}, nil
}
func (*startupIAM) ListProfiles(context.Context, *identityv2.ListProfilesRequest) (*identityv2.ListProfilesResponse, error) {
	return &identityv2.ListProfilesResponse{}, nil
}

func TestRequiredRuntimesUseCertificateAndRealRPCWithoutBearer(t *testing.T) {
	ca := tlsfixture.New(t)
	pair := ca.Issue(t, "server.test", false)
	roots := x509.NewCertPool()
	roots.AddCert(ca.Cert)
	baseMethods := []string{"/iam.authn.v3.AuthService/VerifyToken", "/iam.authz.v4.AuthorizationService/GetAuthorizationSnapshot", "/iam.identity.v2.ProfileLinkQuery/ListProfiles"}
	methods := append(append([]string(nil), baseMethods...), "/iam.authz.v4.AuthorizationService/GetCommittedPolicyVersion")
	acl := base.NewServiceACL(&base.ACLConfig{DefaultPolicy: "deny", Services: []*base.ServicePermissions{{ServiceName: "qs-apiserver.svc", Enabled: true, AllowedMethods: methods}, {ServiceName: "qs-collection-server.svc", Enabled: true, AllowedMethods: baseMethods}}})
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
	t.Run("version guard rejects old IAM and catches committed revocation", func(t *testing.T) {
		clientPair := ca.Issue(t, "qs-apiserver.svc", false)
		opts := options.NewIAMOptions()
		opts.Enabled = true
		opts.JWKSEnabled = false
		opts.GRPC.Address = lis.Addr().String()
		opts.GRPC.Timeout = time.Second
		opts.GRPC.TLS = &options.IAMTLSOptions{Enabled: true, CAFile: ca.CAFile, CertFile: clientPair.CertFile, KeyFile: clientPair.KeyFile}
		opts.AuthzVersionGuard.Enabled = true
		opts.AuthzVersionGuard.MaxAge = 2 * time.Second
		opts.AuthzVersionGuard.PollInterval = 100 * time.Millisecond
		opts.AuthzVersionGuard.ReadTimeout = 500 * time.Millisecond

		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		oldModule, err := module.NewWithRuntimeOptions(ctx, opts, module.RuntimeOptions{})
		require.NoError(t, err)
		require.Error(t, oldModule.ValidateRequiredAuthzRuntime(ctx))
		require.NoError(t, oldModule.Close())

		fixture.committedVersion.Store(1)
		protected, err := module.NewWithRuntimeOptions(ctx, opts, module.RuntimeOptions{})
		require.NoError(t, err)
		defer func() { require.NoError(t, protected.Close()) }()
		require.NoError(t, protected.ValidateRequiredAuthzRuntime(ctx))
		protected.StartAuthzVersionGuard()

		fixture.committedVersion.Store(2) // Committed revocation precedes IAM runtime reconciliation.
		deadline := time.After(2 * time.Second)
		for {
			_, err = protected.AuthzSnapshotLoader().Load(ctx, "1")
			if err != nil && strings.Contains(err.Error(), "older") {
				break
			}
			select {
			case <-deadline:
				t.Fatalf("stale IAM runtime remained usable after committed version advanced: %v", err)
			case <-time.After(10 * time.Millisecond):
			}
		}

		fixture.runtimeVersion.Store(2)
		snap, err := protected.AuthzSnapshotLoader().Load(ctx, "1")
		require.NoError(t, err)
		require.Equal(t, int64(2), snap.AuthzVersion)
	})
	seen := map[string]bool{}
	for len(calls) > 0 {
		seen[<-calls] = true
	}
	for _, method := range methods {
		require.True(t, seen[method], method)
	}
}
