//go:build integration

package iamauth_test

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	base "github.com/FangcunMount/component-base/pkg/grpc/interceptors"
	authnv3 "github.com/FangcunMount/iam/v5/api/grpc/iam/authn/v3"
	authzv4 "github.com/FangcunMount/iam/v5/api/grpc/iam/authz/v4"
	identityv2 "github.com/FangcunMount/iam/v5/api/grpc/iam/identity/v2"
	module "github.com/FangcunMount/qs-server/internal/apiserver/container/modules/iam"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/qs-server/internal/testutil/tlsfixture"
	mysqldsn "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

const samplePrefix = "AUTHZ_GUARD_SAMPLE "

type guardProcessSample struct {
	Process string `json:"process"`
	AtNanos int64  `json:"at_nanos"`
	Outcome string `json:"outcome"`
}

type mysqlBackedIAM struct {
	*startupIAM
	db            *sql.DB
	reads         atomic.Int64
	latestVersion atomic.Int64
	latencyMu     sync.Mutex
	latencies     []time.Duration
}

func (s *mysqlBackedIAM) GetCommittedPolicyVersion(
	ctx context.Context, _ *authzv4.GetCommittedPolicyVersionRequest,
) (*authzv4.GetCommittedPolicyVersionResponse, error) {
	s.reads.Add(1)
	started := time.Now()
	var version int64
	err := s.db.QueryRowContext(ctx, "SELECT policy_version FROM authz_policy_versions WHERE id = 1").Scan(&version)
	s.latencyMu.Lock()
	s.latencies = append(s.latencies, time.Since(started))
	s.latencyMu.Unlock()
	if err != nil {
		return nil, status.Error(codes.Unavailable, "test IAM committed version row unavailable")
	}
	s.latestVersion.Store(version)
	return &authzv4.GetCommittedPolicyVersionResponse{PolicyVersion: version}, nil
}

func (s *mysqlBackedIAM) latencyPercentiles() (time.Duration, time.Duration) {
	s.latencyMu.Lock()
	values := slices.Clone(s.latencies)
	s.latencyMu.Unlock()
	if len(values) == 0 {
		return 0, 0
	}
	slices.Sort(values)
	index := func(percent int) int { return (len(values)*percent+99)/100 - 1 }
	return values[index(95)], values[index(99)]
}

func (s *mysqlBackedIAM) GetAuthorizationSnapshot(
	_ context.Context, _ *authzv4.GetAuthorizationSnapshotRequest,
) (*authzv4.GetAuthorizationSnapshotResponse, error) {
	version := s.runtimeVersion.Load()
	roles := []string(nil)
	if version == 1 {
		roles = []string{"qs:assessment_operator"}
	}
	return &authzv4.GetAuthorizationSnapshotResponse{PolicyVersion: version, DirectRoles: roles}, nil
}

// TestTwoProcessCommittedRevocationWithoutNotification is a process-boundary
// contract. The IAM gRPC server is a TLS/ACL fixture backed by a real MySQL
// committed version row; it is not a running IAM application or full QS API.
func TestTwoProcessCommittedRevocationWithoutNotification(t *testing.T) {
	rawDSN := os.Getenv("QS_SERVER_TEST_IAM_VERSION_DSN")
	if rawDSN == "" {
		t.Skip("set QS_SERVER_TEST_IAM_VERSION_DSN to a disposable localhost qs_authz_guard_test database")
	}
	cfg, err := mysqldsn.ParseDSN(rawDSN)
	require.NoError(t, err)
	host, _, err := net.SplitHostPort(cfg.Addr)
	require.NoError(t, err)
	require.Contains(t, []string{"127.0.0.1", "localhost"}, host)
	require.Equal(t, "qs_authz_guard_test", cfg.DBName)
	db, err := sql.Open("mysql", rawDSN)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	db.SetMaxOpenConns(4)
	require.NoError(t, db.PingContext(t.Context()))
	var existing int
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_name = 'authz_policy_versions'`).Scan(&existing))
	require.Zero(t, existing, "the fixture may only use an empty disposable database")
	_, err = db.ExecContext(t.Context(), `CREATE TABLE authz_policy_versions (
		id BIGINT PRIMARY KEY, policy_version BIGINT NOT NULL
	) ENGINE=InnoDB`)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.Exec("DROP TABLE authz_policy_versions") })
	_, err = db.ExecContext(t.Context(), "INSERT INTO authz_policy_versions(id, policy_version) VALUES(1, 1)")
	require.NoError(t, err)

	ca := tlsfixture.New(t)
	serverPair := ca.Issue(t, "server.test", false)
	clientPair := ca.Issue(t, "qs-apiserver.svc", false)
	roots := x509.NewCertPool()
	roots.AddCert(ca.Cert)
	methods := []string{
		"/iam.authn.v3.AuthService/VerifyToken",
		"/iam.authz.v4.AuthorizationService/GetAuthorizationSnapshot",
		"/iam.authz.v4.AuthorizationService/GetCommittedPolicyVersion",
		"/iam.identity.v2.ProfileLinkQuery/ListProfiles",
	}
	acl := base.NewServiceACL(&base.ACLConfig{DefaultPolicy: "deny", Services: []*base.ServicePermissions{
		{ServiceName: "qs-apiserver.svc", Enabled: true, AllowedMethods: methods},
	}})
	fixture := &mysqlBackedIAM{startupIAM: &startupIAM{}, db: db}
	fixture.runtimeVersion.Store(1) // IAM runtime intentionally lags after commit.
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{
		MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{serverPair.Certificate},
		ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: roots,
	})), grpc.ChainUnaryInterceptor(base.MTLSInterceptor(), base.ACLInterceptor(acl)))
	authzv4.RegisterAuthorizationServiceServer(server, fixture)
	authnv3.RegisterAuthServiceServer(server, fixture)
	identityv2.RegisterProfileLinkQueryServer(server, fixture)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(server.Stop)
	go func() { _ = server.Serve(listener) }()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	samples := make(chan guardProcessSample, 2048)
	children := make([]*exec.Cmd, 0, 2)
	for _, id := range []string{"a", "b"} {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAuthzGuardChildProcess$", "-test.count=1")
		cmd.Env = append(os.Environ(),
			"QS_AUTHZ_GUARD_CHILD="+id,
			"QS_AUTHZ_GUARD_ADDR="+listener.Addr().String(),
			"QS_AUTHZ_GUARD_CA="+ca.CAFile,
			"QS_AUTHZ_GUARD_CERT="+clientPair.CertFile,
			"QS_AUTHZ_GUARD_KEY="+clientPair.KeyFile,
		)
		stdout, pipeErr := cmd.StdoutPipe()
		require.NoError(t, pipeErr)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		require.NoError(t, cmd.Start(), "start child %s", id)
		children = append(children, cmd)
		go func() {
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				line := scanner.Text()
				if !strings.HasPrefix(line, samplePrefix) {
					continue
				}
				var sample guardProcessSample
				if json.Unmarshal([]byte(strings.TrimPrefix(line, samplePrefix)), &sample) == nil {
					samples <- sample
				}
			}
		}()
		t.Cleanup(func() {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			if t.Failed() {
				t.Logf("child %s stderr: %s", id, stderr.String())
			}
		})
	}

	ready := map[string]bool{}
	allowed := map[string]bool{}
	startupDeadline := time.NewTimer(8 * time.Second)
	defer startupDeadline.Stop()
	for len(ready) < 2 || len(allowed) < 2 {
		select {
		case sample := <-samples:
			if sample.Outcome == "ready" {
				ready[sample.Process] = true
			}
			if sample.Outcome == "old_allowed" {
				allowed[sample.Process] = true
			}
		case <-startupDeadline.C:
			t.Fatalf("both child processes did not admit the original role: ready=%v allowed=%v", ready, allowed)
		}
	}
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	_, err = tx.ExecContext(t.Context(), "UPDATE authz_policy_versions SET policy_version = 2 WHERE id = 1")
	require.NoError(t, err)
	var outsideVersion int64
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT policy_version FROM authz_policy_versions WHERE id = 1").Scan(&outsideVersion))
	require.Equal(t, int64(1), outsideVersion, "uncommitted revocation must be invisible")
	require.NoError(t, tx.Commit())
	committedAt := time.Now()
	firstBlocked := map[string]time.Time{}
	seenAfterDeadline := map[string]bool{}
	hardDeadline := committedAt.Add(10 * time.Second)
	observationEnd := time.NewTimer(time.Until(committedAt.Add(11 * time.Second)))
	defer observationEnd.Stop()
	for observing := true; observing; {
		select {
		case sample := <-samples:
			at := time.Unix(0, sample.AtNanos)
			if sample.Outcome == "ready" || at.Before(committedAt) {
				continue
			}
			if sample.Outcome != "old_allowed" && firstBlocked[sample.Process].IsZero() {
				firstBlocked[sample.Process] = at
			}
			if !at.Before(hardDeadline) {
				if sample.Outcome == "old_allowed" {
					t.Fatalf("process %s admitted revoked role after 10s", sample.Process)
				}
				seenAfterDeadline[sample.Process] = true
			}
		case <-observationEnd.C:
			observing = false
		}
	}
	require.Len(t, seenAfterDeadline, 2, "both processes must remain observable after the ten-second boundary")
	for _, id := range []string{"a", "b"} {
		require.False(t, firstBlocked[id].IsZero(), "process %s never rejected", id)
		require.True(t, firstBlocked[id].Before(hardDeadline), "process %s first rejected after deadline", id)
		t.Logf("process %s first rejected after %s", id, firstBlocked[id].Sub(committedAt))
	}
	require.Equal(t, int64(2), fixture.latestVersion.Load())
	require.LessOrEqual(t, fixture.reads.Load(), int64(16), "authorization requests amplified IAM version reads")
	p95, p99 := fixture.latencyPercentiles()
	t.Logf("IAM committed-version RPCs=%d MySQL row-read p95=%s p99=%s", fixture.reads.Load(), p95, p99)
}

func TestAuthzGuardChildProcess(t *testing.T) {
	id := os.Getenv("QS_AUTHZ_GUARD_CHILD")
	if id == "" {
		t.Skip("helper process")
	}
	opts := options.NewIAMOptions()
	opts.Enabled = true
	opts.GRPCEnabled = true
	opts.JWKSEnabled = false
	opts.GRPC.Address = os.Getenv("QS_AUTHZ_GUARD_ADDR")
	opts.GRPC.Timeout = time.Second
	opts.GRPC.TLS = &options.IAMTLSOptions{
		Enabled: true, CAFile: os.Getenv("QS_AUTHZ_GUARD_CA"),
		CertFile: os.Getenv("QS_AUTHZ_GUARD_CERT"), KeyFile: os.Getenv("QS_AUTHZ_GUARD_KEY"),
	}
	opts.AuthzVersionGuard.Enabled = true
	opts.AuthzVersionGuard.MaxAge = 10 * time.Second
	opts.AuthzVersionGuard.PollInterval = 5 * time.Second
	opts.AuthzVersionGuard.ReadTimeout = 2 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	iamModule, err := module.NewWithRuntimeOptions(ctx, opts, module.RuntimeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = iamModule.Close() }()
	if err := iamModule.ValidateRequiredAuthzRuntime(ctx); err != nil {
		t.Fatal(err)
	}
	iamModule.StartAuthzVersionGuard()
	emitGuardSample(id, "ready")
	for {
		readCtx, readCancel := context.WithTimeout(context.Background(), time.Second)
		snap, readErr := iamModule.AuthzSnapshotLoader().Load(readCtx, "1")
		outcome := "unavailable"
		if readErr == nil {
			if verifyErr := iamModule.AuthzSnapshotLoader().VerifySnapshot(readCtx, snap); verifyErr == nil {
				outcome = "new_denied"
				for _, role := range snap.DirectRoles {
					if role == "qs:assessment_operator" {
						outcome = "old_allowed"
					}
				}
			}
		}
		readCancel()
		emitGuardSample(id, outcome)
		time.Sleep(50 * time.Millisecond)
	}
}

func emitGuardSample(process, outcome string) {
	encoded, _ := json.Marshal(guardProcessSample{Process: process, AtNanos: time.Now().UnixNano(), Outcome: outcome})
	fmt.Printf("%s%s\n", samplePrefix, encoded)
}
