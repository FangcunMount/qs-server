package process

import (
	"errors"
	"testing"
	"time"

	apiserverconfig "github.com/FangcunMount/qs-server/internal/apiserver/config"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	grpcpkg "github.com/FangcunMount/qs-server/internal/pkg/grpc"
	genericapiserver "github.com/FangcunMount/qs-server/internal/pkg/server"
)

func TestApplyGRPCOptions(t *testing.T) {
	opts := apiserveroptions.NewOptions()
	opts.GRPCOptions.BindAddress = "0.0.0.0"
	opts.GRPCOptions.BindPort = 9443
	opts.GRPCOptions.Insecure = false
	opts.GRPCOptions.TLSCertFile = "/tmp/server.crt"
	opts.GRPCOptions.TLSKeyFile = "/tmp/server.key"
	opts.GRPCOptions.MaxMsgSize = 32 * 1024
	opts.GRPCOptions.MaxConnectionAge = 10 * time.Minute
	opts.GRPCOptions.MaxConnectionAgeGrace = 90 * time.Second
	opts.GRPCOptions.MTLS.Enabled = true
	opts.GRPCOptions.MTLS.CAFile = "/tmp/ca.crt"
	opts.GRPCOptions.MTLS.RequireClientCert = true
	opts.GRPCOptions.MTLS.AllowedCNs = []string{"svc-a"}
	opts.GRPCOptions.MTLS.AllowedOUs = []string{"ops"}
	opts.GRPCOptions.MTLS.MinTLSVersion = "1.3"
	opts.GRPCOptions.Auth.Enabled = true
	opts.GRPCOptions.ACL.Enabled = true
	opts.GRPCOptions.Audit.Enabled = true
	opts.GRPCOptions.EnableReflection = false
	opts.GRPCOptions.EnableHealthCheck = false
	opts.IAMOptions.JWT.ForceRemoteVerification = true

	cfg, err := apiserverconfig.CreateConfigFromOptions(opts)
	if err != nil {
		t.Fatalf("CreateConfigFromOptions() error = %v", err)
	}

	grpcConfig := grpcpkg.NewConfig()
	if err := applyGRPCOptions(cfg, grpcConfig); err != nil {
		t.Fatalf("applyGRPCOptions() error = %v", err)
	}

	if grpcConfig.BindAddress != "0.0.0.0" || grpcConfig.BindPort != 9443 {
		t.Fatalf("bind config mismatch: %s:%d", grpcConfig.BindAddress, grpcConfig.BindPort)
	}
	if grpcConfig.Insecure {
		t.Fatalf("Insecure = true, want false")
	}
	if grpcConfig.TLSCertFile != "/tmp/server.crt" || grpcConfig.TLSKeyFile != "/tmp/server.key" {
		t.Fatalf("TLS mapping mismatch: cert=%q key=%q", grpcConfig.TLSCertFile, grpcConfig.TLSKeyFile)
	}
	if grpcConfig.MaxMsgSize != 32*1024 || grpcConfig.MaxConnectionAge != 10*time.Minute || grpcConfig.MaxConnectionAgeGrace != 90*time.Second {
		t.Fatalf("connection settings mismatch: %+v", grpcConfig)
	}
	if !grpcConfig.MTLS.Enabled || grpcConfig.MTLS.CAFile != "/tmp/ca.crt" || !grpcConfig.MTLS.RequireClientCert {
		t.Fatalf("MTLS mapping mismatch: %+v", grpcConfig.MTLS)
	}
	if len(grpcConfig.MTLS.AllowedCNs) != 1 || grpcConfig.MTLS.AllowedCNs[0] != "svc-a" {
		t.Fatalf("MTLS.AllowedCNs = %+v", grpcConfig.MTLS.AllowedCNs)
	}
	if len(grpcConfig.MTLS.AllowedOUs) != 1 || grpcConfig.MTLS.AllowedOUs[0] != "ops" {
		t.Fatalf("MTLS.AllowedOUs = %+v", grpcConfig.MTLS.AllowedOUs)
	}
	if grpcConfig.MTLS.MinTLSVersion != "1.3" {
		t.Fatalf("MTLS.MinTLSVersion = %q, want 1.3", grpcConfig.MTLS.MinTLSVersion)
	}
	if !grpcConfig.Auth.Enabled || !grpcConfig.Auth.ForceRemoteVerification {
		t.Fatalf("Auth mapping mismatch: %+v", grpcConfig.Auth)
	}
	if !grpcConfig.ACL.Enabled {
		t.Fatalf("ACL.Enabled = false, want true")
	}
	if !grpcConfig.Audit.Enabled {
		t.Fatalf("Audit.Enabled = false, want true")
	}
	if grpcConfig.EnableReflection {
		t.Fatalf("EnableReflection = true, want false")
	}
	if grpcConfig.EnableHealthCheck {
		t.Fatalf("EnableHealthCheck = true, want false")
	}
}

func TestBootstrapTransportsBuildsAndRegistersServers(t *testing.T) {
	t.Parallel()

	httpServer := &genericapiserver.GenericAPIServer{}
	grpcServer := &grpcpkg.Server{}
	var restRegistered bool
	var grpcRegistered bool

	got, err := bootstrapTransports(transportStageDeps{
		buildHTTPServer: func() (*genericapiserver.GenericAPIServer, error) { return httpServer, nil },
		buildGRPCServer: func() (*grpcpkg.Server, error) { return grpcServer, nil },
		registerREST: func(gotHTTP *genericapiserver.GenericAPIServer) {
			if gotHTTP != httpServer {
				t.Fatalf("REST server = %#v, want %#v", gotHTTP, httpServer)
			}
			restRegistered = true
		},
		registerGRPC: func(gotGRPC *grpcpkg.Server) error {
			if gotGRPC != grpcServer {
				t.Fatalf("gRPC server = %#v, want %#v", gotGRPC, grpcServer)
			}
			grpcRegistered = true
			return nil
		},
	})
	if err != nil {
		t.Fatalf("bootstrapTransports() error = %v", err)
	}
	if got.httpServer != httpServer {
		t.Fatalf("httpServer = %#v, want %#v", got.httpServer, httpServer)
	}
	if got.grpcServer != grpcServer {
		t.Fatalf("grpcServer = %#v, want %#v", got.grpcServer, grpcServer)
	}
	if !restRegistered {
		t.Fatal("REST transport was not registered")
	}
	if !grpcRegistered {
		t.Fatal("gRPC transport was not registered")
	}
}

func TestBootstrapTransportsReturnsRegistrationError(t *testing.T) {
	t.Parallel()

	_, err := bootstrapTransports(transportStageDeps{
		buildGRPCServer: func() (*grpcpkg.Server, error) { return &grpcpkg.Server{}, nil },
		registerGRPC:    func(*grpcpkg.Server) error { return errors.New("grpc register boom") },
	})
	if err == nil || err.Error() != "grpc register boom" {
		t.Fatalf("bootstrapTransports() error = %v, want grpc register boom", err)
	}
}
