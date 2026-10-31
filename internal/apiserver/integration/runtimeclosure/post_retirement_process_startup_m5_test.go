//go:build integration && m3_host_runtime

package runtimeclosure

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	authnv3 "github.com/FangcunMount/iam/v5/api/grpc/iam/authn/v3"
	authzv4 "github.com/FangcunMount/iam/v5/api/grpc/iam/authz/v4"
	identityv2 "github.com/FangcunMount/iam/v5/api/grpc/iam/identity/v2"
	apiconfig "github.com/FangcunMount/qs-server/internal/apiserver/config"
	apioptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	apiprocess "github.com/FangcunMount/qs-server/internal/apiserver/process"
	collectionconfig "github.com/FangcunMount/qs-server/internal/collection-server/config"
	collectionoptions "github.com/FangcunMount/qs-server/internal/collection-server/options"
	collectionprocess "github.com/FangcunMount/qs-server/internal/collection-server/process"
	"github.com/FangcunMount/qs-server/internal/pkg/delegatedsubject"
	generic "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/testutil/tlsfixture"
	"github.com/FangcunMount/qs-server/pkg/version"
	driver "github.com/go-sql-driver/mysql"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

// Real process roots after the owned business fixture; not an end-user login.
type m5RootConfig struct {
	Source string
	// Private file only: public option JSON deliberately omits database passwords.
	MySQLDSN   string `json:"mysql_dsn,omitempty"`
	API        *apioptions.Options
	Collection *collectionoptions.Options
	Worker     *m3cChildConfig
}

func restoreM5PrivateAPIConnection(cfg *m5RootConfig) error {
	if cfg.API == nil || cfg.MySQLDSN == "" {
		return errors.New("private owned MySQL connection required")
	}
	dsn, err := driver.ParseDSN(cfg.MySQLDSN)
	if err != nil {
		return errors.New("private owned MySQL connection invalid")
	}
	host, _, err := net.SplitHostPort(dsn.Addr)
	if err != nil || !net.ParseIP(host).IsLoopback() || dsn.Net != "tcp" || dsn.DBName != cfg.API.MySQLOptions.Database || dsn.Addr != cfg.API.MySQLOptions.Host || dsn.User != cfg.API.MySQLOptions.Username || dsn.Passwd == "" {
		return errors.New("private owned MySQL binding changed")
	}
	cfg.API.MySQLOptions.Password = dsn.Passwd
	return nil
}

func TestM5PrivateRootConnectionSurvivesPublicOptionRedaction(t *testing.T) {
	api := apioptions.NewOptions()
	api.MySQLOptions.Host = "127.0.0.1:3306"
	api.MySQLOptions.Username = "owned"
	api.MySQLOptions.Database = "qs_runtime_closure_123"
	api.MySQLOptions.Password = "owned-private:@/mysql"
	public, err := json.Marshal(api)
	require.NoError(t, err)
	require.False(t, bytes.Contains(public, []byte(api.MySQLOptions.Password)))
	dsn := driver.NewConfig()
	dsn.Net = "tcp"
	dsn.Addr = api.MySQLOptions.Host
	dsn.User = api.MySQLOptions.Username
	dsn.Passwd = api.MySQLOptions.Password
	dsn.DBName = api.MySQLOptions.Database
	raw, err := json.Marshal(m5RootConfig{API: api, MySQLDSN: dsn.FormatDSN()})
	require.NoError(t, err)
	var child m5RootConfig
	require.NoError(t, json.Unmarshal(raw, &child))
	require.Empty(t, child.API.MySQLOptions.Password)
	require.NoError(t, restoreM5PrivateAPIConnection(&child))
	require.Equal(t, api.MySQLOptions.Password, child.API.MySQLOptions.Password)
	child.MySQLDSN = ""
	require.Error(t, restoreM5PrivateAPIConnection(&child))
	child.MySQLDSN = dsn.FormatDSN()
	child.API.MySQLOptions.Database = "foreign"
	require.Error(t, restoreM5PrivateAPIConnection(&child))
}

func TestM5PostRetirementRootChild(t *testing.T) {
	path := os.Getenv("QS_M5_ROOT_CHILD")
	if path == "" {
		return
	}
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var cfg m5RootConfig
	require.NoError(t, json.Unmarshal(raw, &cfg))
	require.Equal(t, cfg.Source, version.Get().GitCommit, "must use compiled exact source")
	switch {
	case cfg.API != nil && cfg.Collection == nil && cfg.Worker == nil:
		require.True(t, regexp.MustCompile(`^qs_runtime_closure_[0-9]+$`).MatchString(cfg.API.MySQLOptions.Database), "owned_runtime_namespace_required")
		require.False(t, cfg.API.MigrationOptions.Enabled)
		require.NoError(t, restoreM5PrivateAPIConnection(&cfg))
		viper.Set("mongodb.database", cfg.API.MongoDBOptions.Database)
		require.NoError(t, apiprocess.Run(&apiconfig.Config{Options: cfg.API}))
	case cfg.Collection != nil && cfg.API == nil && cfg.Worker == nil:
		require.NoError(t, collectionprocess.Run(&collectionconfig.Config{Options: cfg.Collection}))
	case cfg.Worker != nil && cfg.API == nil && cfg.Collection == nil:
		raw, err := json.Marshal(cfg.Worker)
		require.NoError(t, err)
		path := filepath.Join(t.TempDir(), "worker.json")
		require.NoError(t, os.WriteFile(path, raw, 0600))
		t.Setenv("QS_M3C_WORKER_CHILD", path)
		TestM3RecoveryWorkerProcess(t)
	default:
		t.Fatal("exactly one owned root required")
	}
}

type m5StartupIAM struct {
	authnv3.UnimplementedAuthServiceServer
	authzv4.UnimplementedAuthorizationServiceServer
	identityv2.UnimplementedProfileLinkQueryServer
}

func (*m5StartupIAM) VerifyToken(context.Context, *authnv3.VerifyTokenRequest) (*authnv3.VerifyTokenResponse, error) {
	return nil, status.Error(codes.InvalidArgument, "empty token")
}
func (*m5StartupIAM) GetAuthorizationSnapshot(context.Context, *authzv4.GetAuthorizationSnapshotRequest) (*authzv4.GetAuthorizationSnapshotResponse, error) {
	return &authzv4.GetAuthorizationSnapshotResponse{PolicyVersion: 1}, nil
}
func (*m5StartupIAM) GetCommittedPolicyVersion(context.Context, *authzv4.GetCommittedPolicyVersionRequest) (*authzv4.GetCommittedPolicyVersionResponse, error) {
	return &authzv4.GetCommittedPolicyVersionResponse{PolicyVersion: 1}, nil
}
func (*m5StartupIAM) ListProfiles(context.Context, *identityv2.ListProfilesRequest) (*identityv2.ListProfilesResponse, error) {
	return &identityv2.ListProfilesResponse{}, nil
}

// Only fixed technical labels leave the private child log.
func m5StartupDiagnostic(role, phase string, err error, raw []byte) {
	if captureErr := encryptM5StartupDiagnostic(role, raw); captureErr != nil {
		fmt.Println("M5 encrypted fixture diagnostic unavailable")
	}
	code := -1
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	}
	stage := "unknown"
	for _, entry := range [][2]string{{"prepare resources", "resources"}, {"initialize container", "container"}, {"initialize integrations", "integrations"}, {"initialize transports", "transports"}, {"start background runtimes", "runtime"}, {"register shutdown callback", "shutdown"}} {
		if strings.Contains(string(raw), entry[0]) {
			stage = entry[1]
		}
	}
	reason := "process_exit"
	for _, entry := range [][2]string{{"owned_runtime_namespace_required", "owned_namespace"}, {"must use compiled exact source", "compiled_source"}, {"paired migration preflight rejected", "schema_pair"}, {"IAM health check failed", "iam"}, {"IAM AuthZ v3 startup probe failed", "iam"}, {"failed to initialize MySQL", "database"}, {"failed to initialize MongoDB", "database"}} {
		if strings.Contains(string(raw), entry[0]) {
			reason = entry[1]
			break
		}
	}
	diagnostic, _ := json.Marshal(map[string]any{"role": role, "phase": phase, "stage": stage, "reason": reason, "exit_code": code})
	fmt.Println("M5_PROCESS_STARTUP_DIAGNOSTIC " + string(diagnostic))
}

func m5StartProcessRoots(t *testing.T, cfg m3cChildConfig) time.Duration {
	t.Helper()
	began := time.Now()
	dsn, err := driver.ParseDSN(cfg.DSN)
	require.NoError(t, err)
	host, _, err := net.SplitHostPort(dsn.Addr)
	require.NoError(t, err)
	require.True(t, net.ParseIP(host).IsLoopback())
	rh, rp, err := net.SplitHostPort(cfg.Redis)
	require.NoError(t, err)
	require.True(t, net.ParseIP(rh).IsLoopback())
	port, err := strconv.Atoi(rp)
	require.NoError(t, err)
	source, err := exec.CommandContext(t.Context(), "git", "rev-parse", "HEAD").Output()
	require.NoError(t, err)
	sha := strings.TrimSpace(string(source))
	require.Len(t, sha, 40)
	private := t.TempDir()
	// The existing native harness already links this race binary with the exact
	// source SHA. Reuse it rather than starting a nested build in the proof.
	require.Equal(t, sha, version.Get().GitCommit)
	binary, err := os.Executable()
	require.NoError(t, err)
	ca := tlsfixture.New(t)
	serverPair := ca.Issue(t, "server.test", false)
	roots := x509.NewCertPool()
	roots.AddCert(ca.Cert)
	iam := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{serverPair.Certificate}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots})))
	fixture := &m5StartupIAM{}
	authnv3.RegisterAuthServiceServer(iam, fixture)
	authzv4.RegisterAuthorizationServiceServer(iam, fixture)
	identityv2.RegisterProfileLinkQueryServer(iam, fixture)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(iam.Stop)
	go func() { _ = iam.Serve(listener) }()
	iamOptions := func(name string) *generic.IAMOptions {
		p := ca.Issue(t, name, false)
		o := generic.NewIAMOptions()
		o.Enabled = true
		o.JWKSEnabled = false
		o.GRPC.Address = listener.Addr().String()
		o.GRPC.Timeout = 2 * time.Second
		o.GRPC.TLS = &generic.IAMTLSOptions{Enabled: true, CAFile: ca.CAFile, CertFile: p.CertFile, KeyFile: p.KeyFile}
		o.AuthzSync.Enabled = false
		return o
	}
	allocatedPorts := map[int]bool{}
	freePort := func() int {
		for range 10 {
			l, e := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, e)
			p := l.Addr().(*net.TCPAddr).Port
			require.NoError(t, l.Close())
			if !allocatedPorts[p] {
				allocatedPorts[p] = true
				return p
			}
		}
		t.Fatal("could not allocate distinct owned ports")
		return 0
	}
	redisOptions := generic.NewRedisOptions()
	redisOptions.Host = rh
	redisOptions.Port = port
	redisOptions.Database = 8
	profiles := func(runtime *generic.RedisRuntimeOptions) map[string]*generic.RedisOptions {
		out := map[string]*generic.RedisOptions{}
		for _, r := range runtime.Families {
			c := *redisOptions
			out[r.RedisProfile] = &c
		}
		return out
	}
	api := apioptions.NewOptions()
	api.GenericServerRunOptions.Mode = "release"
	api.InsecureServing.BindPort = freePort()
	api.SecureServing.BindPort = 0
	api.MySQLOptions.MaxOpenConnections = 8
	api.MySQLOptions.MaxIdleConnections = 2
	api.MySQLOptions.Host = dsn.Addr
	api.MySQLOptions.Username = dsn.User
	api.MySQLOptions.Password = dsn.Passwd
	api.MySQLOptions.Database = dsn.DBName
	api.MySQLOptions.Location = "Asia/Shanghai"
	api.MySQLOptions.SessionTimeZone = "+08:00"
	api.MongoDBOptions.URL = cfg.MongoURI
	api.MongoDBOptions.Database = cfg.MongoDB
	api.MongoDBOptions.MaxPoolSize = 4
	api.MigrationOptions.Enabled = false
	api.MigrationOptions.AutoSeed = false
	api.MigrationOptions.Database = ""
	api.RedisOptions = redisOptions
	api.RedisRuntime.Namespace = cfg.Namespace + "-root"
	api.RedisProfiles = profiles(api.RedisRuntime)
	mq := m3cLoadMQ(t, os.Getenv("QS_M3_HOST_OPTIONS"))
	api.MessagingOptions = &mq
	api.IAMOptions = iamOptions("qs-apiserver.svc")
	api.WeChatOptions = nil
	api.OSSOptions = nil
	api.Eventing.TaskOpenedReminder = false // Provider behavior is proven in the preceding fourteen cases.
	api.DelegatedSubject = &delegatedsubject.Options{Enabled: true, CurrentKey: strings.Repeat("m5-owned-", 8), TTL: time.Minute}
	api.GRPCOptions.BindPort = freePort()
	api.GRPCOptions.HealthzPort = freePort()
	api.GRPCOptions.Insecure = false
	api.GRPCOptions.TLSCertFile = serverPair.CertFile
	api.GRPCOptions.TLSKeyFile = serverPair.KeyFile
	api.GRPCOptions.MTLS.CAFile = ca.CAFile
	api.GRPCOptions.MTLS.Enabled = true
	api.GRPCOptions.MTLS.RequireClientCert = true
	api.GRPCOptions.Auth.Enabled = true
	api.GRPCOptions.ACL.Enabled = true
	api.GRPCOptions.ACL.ConfigFile = "configs/grpc-acl.prod.yaml"
	collection := collectionoptions.NewOptions()
	collection.GenericServerRunOptions.Mode = "release"
	collection.InsecureServing.BindPort = freePort()
	collection.SecureServing.BindPort = 0
	collection.IAMOptions = iamOptions("qs-collection-server.svc")
	collection.RedisOptions = redisOptions
	collection.RedisRuntime.Namespace = api.RedisRuntime.Namespace
	collection.RedisProfiles = profiles(collection.RedisRuntime)
	collection.DelegatedSubject = api.DelegatedSubject
	cp := ca.Issue(t, "qs-collection-server.svc", false)
	collection.GRPCClient.Endpoint = fmt.Sprintf("127.0.0.1:%d", api.GRPCOptions.BindPort)
	collection.GRPCClient.Insecure = false
	collection.GRPCClient.TLSCAFile = ca.CAFile
	collection.GRPCClient.TLSCertFile = cp.CertFile
	collection.GRPCClient.TLSKeyFile = cp.KeyFile
	collection.GRPCClient.TLSServerName = "127.0.0.1"
	type process struct {
		role   string
		cmd    *exec.Cmd
		done   chan error
		log    *os.File
		exited atomic.Bool
	}
	start := func(name string, root m5RootConfig, httpPort int) *process {
		healthPath := "/healthz"
		if root.Worker != nil {
			healthPath = "/readyz"
		}
		b, e := json.Marshal(root)
		require.NoError(t, e)
		p := filepath.Join(private, name+".json")
		require.NoError(t, os.WriteFile(p, b, 0600))
		log, e := os.OpenFile(filepath.Join(private, name+".log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		require.NoError(t, e)
		cmd := exec.Command(binary, "-test.run=^TestM5PostRetirementRootChild$", "-test.v")
		cmd.Dir, e = filepath.Abs("../../../..")
		require.NoError(t, e)
		// The existing Worker child resolves its event catalog from the package
		// directory, while API/Collection roots resolve repository configs.
		if root.Worker != nil {
			cmd.Dir = ""
		}
		cmd.Env = append(os.Environ(), "QS_M5_ROOT_CHILD="+p)
		cmd.Stdout = log
		cmd.Stderr = log
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		require.NoError(t, cmd.Start())
		child := &process{role: name, cmd: cmd, done: make(chan error, 1), log: log}
		go func() { err := cmd.Wait(); child.exited.Store(true); child.done <- err }()
		t.Cleanup(func() {
			if !child.exited.Load() {
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				select {
				case <-child.done:
				case <-time.After(10 * time.Second):
					t.Error("owned root cleanup timed out")
				}
			}
			_ = log.Close()
		})
		deadline := time.Now().Add(30 * time.Second)
		client := http.Client{Timeout: time.Second}
		for time.Now().Before(deadline) {
			select {
			case err := <-child.done:
				raw, _ := os.ReadFile(log.Name())
				m5StartupDiagnostic(name, "startup_exit", err, raw)
				t.Fatalf("owned %s exited before health: %v\n%s", name, err, raw)
			default:
			}
			response, e := client.Get(fmt.Sprintf("http://127.0.0.1:%d%s", httpPort, healthPath))
			if e == nil {
				ready := response.StatusCode == http.StatusOK
				if root.Worker != nil {
					var state struct {
						Redis struct {
							Summary struct {
								Ready bool `json:"ready"`
							} `json:"summary"`
						} `json:"redis"`
					}
					ready = ready && json.NewDecoder(response.Body).Decode(&state) == nil && state.Redis.Summary.Ready
					// Redis governance becomes ready before subscriptions and the
					// signal manager. Wait for the real Run boundary before SIGTERM.
					startupLog, _ := os.ReadFile(log.Name())
					ready = ready && bytes.Contains(startupLog, []byte("Worker started, waiting for events"))
				}
				_ = response.Body.Close()
				if ready {
					return child
				}
			}
			time.Sleep(200 * time.Millisecond)
		}
		raw, _ := os.ReadFile(log.Name())
		m5StartupDiagnostic(name, "health_deadline", nil, raw)
		t.Fatalf("owned %s health unavailable\n%s", name, raw)
		return nil
	}
	ap := start("api", m5RootConfig{Source: sha, API: api, MySQLDSN: cfg.DSN}, api.InsecureServing.BindPort)
	co := start("collection", m5RootConfig{Source: sha, Collection: collection}, collection.InsecureServing.BindPort)
	workerConfig := cfg
	wp := ca.Issue(t, "qs-worker.svc", false)
	workerConfig.CA = ca.CAFile
	workerConfig.Cert = wp.CertFile
	workerConfig.Key = wp.KeyFile
	workerConfig.Endpoint = collection.GRPCClient.Endpoint
	workerConfig.MetricsPort = freePort()
	workerConfig.Hold = false
	workerConfig.Marker = filepath.Join(private, "post-startup-worker-marker.json")
	wo := start("worker", m5RootConfig{Source: sha, Worker: &workerConfig}, workerConfig.MetricsPort)
	// Require all roots to remain alive concurrently before stopping any of them.
	for _, p := range []*process{wo, co, ap} {
		require.False(t, p.exited.Load(), "owned root exited before complete process-set observation")
	}
	for _, p := range []*process{wo, co, ap} {
		require.NoError(t, p.cmd.Process.Signal(syscall.SIGTERM))
		select {
		case e := <-p.done:
			if e != nil {
				raw, _ := os.ReadFile(p.log.Name())
				m5StartupDiagnostic(p.role, "drain_exit", e, raw)
			}
			require.NoError(t, e, "owned %s drain failed", p.role)
		case <-time.After(30 * time.Second):
			raw, _ := os.ReadFile(p.log.Name())
			m5StartupDiagnostic(p.role, "drain_deadline", nil, raw)
			t.Fatalf("owned %s did not drain", p.role)
		}
		require.NoError(t, p.log.Close())
	}
	return time.Since(began)
}
