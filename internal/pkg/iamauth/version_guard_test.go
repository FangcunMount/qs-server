package iamauth

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	authzv4 "github.com/FangcunMount/iam/v5/api/grpc/iam/authz/v4"
	"github.com/FangcunMount/iam/v5/pkg/sdk"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type versionReaderFunc func(context.Context) (int64, error)

func (f versionReaderFunc) GetCommittedPolicyVersion(ctx context.Context) (int64, error) {
	return f(ctx)
}

type guardClock struct {
	mu sync.Mutex
	at time.Time
}

func (c *guardClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

func (c *guardClock) advance(d time.Duration) {
	c.mu.Lock()
	c.at = c.at.Add(d)
	c.mu.Unlock()
}

func TestVersionGuardBoundsProofFromReadStart(t *testing.T) {
	clock := &guardClock{at: time.Now()}
	version := int64(10)
	reads := 0
	guard, err := NewVersionGuard(versionReaderFunc(func(context.Context) (int64, error) {
		reads++
		clock.advance(2 * time.Second)
		return version, nil
	}), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	guard.now = clock.now
	if got, err := guard.Verify(context.Background()); err != nil || got != 10 {
		t.Fatalf("initial proof = %d, %v", got, err)
	}
	clock.advance(7 * time.Second) // Nine seconds from the call's start.
	if got, err := guard.Verify(context.Background()); err != nil || got != 10 || reads != 1 {
		t.Fatalf("proof before hard boundary = %d, %v, reads %d", got, err, reads)
	}
	version = 11 // Committed revocation while the original proof was still valid.
	clock.advance(time.Second)
	if got, err := guard.Verify(context.Background()); err != nil || got != 11 || reads != 2 {
		t.Fatalf("proof at hard boundary = %d, %v, reads %d", got, err, reads)
	}
}

func TestVersionGuardFailsClosedForOutageAndSlowResponse(t *testing.T) {
	clock := &guardClock{at: time.Now()}
	fail := false
	slow := false
	reads := 0
	guard, err := NewVersionGuard(versionReaderFunc(func(context.Context) (int64, error) {
		reads++
		if fail {
			return 0, errors.New("iam unavailable")
		}
		if slow {
			clock.advance(10 * time.Second)
		}
		return 20, nil
	}), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	guard.now = clock.now
	if _, err := guard.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	fail = true
	if err := guard.Refresh(context.Background()); err == nil {
		t.Fatal("outage was accepted as a refreshed proof")
	}
	clock.advance(10 * time.Second)
	if _, err := guard.Verify(context.Background()); err == nil {
		t.Fatal("expired proof passed during IAM outage")
	}
	fail = false
	slow = true
	clock.advance(5 * time.Second)
	if _, err := guard.Verify(context.Background()); err == nil {
		t.Fatal("slow read renewed a proof after its 10-second deadline")
	}
	if reads != 4 {
		t.Fatalf("slow read was not exercised: reads = %d", reads)
	}
}

func TestVersionGuardNotificationAndRegressionNeverRenewProof(t *testing.T) {
	clock := &guardClock{at: time.Now()}
	version := int64(30)
	guard, err := NewVersionGuard(versionReaderFunc(func(context.Context) (int64, error) {
		return version, nil
	}), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	guard.now = clock.now
	if _, err := guard.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	clock.advance(9 * time.Second)
	guard.ObserveVersion(31)
	if _, err := guard.Verify(context.Background()); err == nil {
		t.Fatal("notification ahead of committed version was treated as proof")
	}
	version = 31
	if err := guard.Refresh(context.Background()); err != nil {
		t.Fatalf("periodic refresh must recover even during request cooldown: %v", err)
	}
	if got, err := guard.Verify(context.Background()); err != nil || got != 31 {
		t.Fatalf("committed notification version = %d, %v", got, err)
	}
	version = 30
	if err := guard.Refresh(context.Background()); err == nil {
		t.Fatal("version regression renewed proof")
	}
	clock.advance(10 * time.Second)
	if _, err := guard.Verify(context.Background()); err == nil {
		t.Fatal("regressed reader allowed old permission after expiry")
	}
}

func TestVersionGuardBoundsRetryTrafficDuringOutage(t *testing.T) {
	clock := &guardClock{at: time.Now()}
	var reads atomic.Int32
	unavailable := false
	guard, err := NewVersionGuard(versionReaderFunc(func(context.Context) (int64, error) {
		reads.Add(1)
		if unavailable {
			return 0, errors.New("IAM unavailable")
		}
		return 42, nil
	}), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	guard.now = clock.now
	if _, err := guard.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	unavailable = true
	clock.advance(10 * time.Second)
	var callers sync.WaitGroup
	denials := make(chan error, 100)
	for range 100 {
		callers.Add(1)
		go func() {
			defer callers.Done()
			_, err := guard.Verify(context.Background())
			denials <- err
		}()
	}
	callers.Wait()
	close(denials)
	for err := range denials {
		if err == nil {
			t.Fatal("old permission passed during IAM outage")
		}
	}
	if got := reads.Load(); got != 2 {
		t.Fatalf("100 denied requests caused %d IAM reads, want 1 retry after initial proof", got)
	}
	clock.advance(5 * time.Second)
	if _, err := guard.Verify(context.Background()); err == nil || reads.Load() != 3 {
		t.Fatalf("retry after cooldown = %v, reads %d", err, reads.Load())
	}
}

func TestVersionGuardRefreshDiscoversRevocationBeforeExpiry(t *testing.T) {
	clock := &guardClock{at: time.Now()}
	version := int64(40)
	guard, err := NewVersionGuard(versionReaderFunc(func(context.Context) (int64, error) {
		return version, nil
	}), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	guard.now = clock.now
	if _, err := guard.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	clock.advance(5 * time.Second)
	version = 41
	if err := guard.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, err := guard.Verify(context.Background()); err != nil || got != 41 {
		t.Fatalf("periodic proof = %d, %v", got, err)
	}
}

type laggingSnapshotIAM struct {
	authzv4.UnimplementedAuthorizationServiceServer
	mu      sync.Mutex
	version int64
	entered chan struct{}
	release chan struct{}
}

func (s *laggingSnapshotIAM) GetAuthorizationSnapshot(context.Context, *authzv4.GetAuthorizationSnapshotRequest) (*authzv4.GetAuthorizationSnapshotResponse, error) {
	s.mu.Lock()
	version, entered, release := s.version, s.entered, s.release
	s.mu.Unlock()
	if entered != nil {
		entered <- struct{}{}
		<-release
	}
	return &authzv4.GetAuthorizationSnapshotResponse{PolicyVersion: version, DirectRoles: []string{"qs:assessment_operator"}}, nil
}

func TestSnapshotLoaderRejectsInFlightSnapshotAfterVersionAdvances(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	iam := &laggingSnapshotIAM{version: 70, entered: make(chan struct{}, 1), release: make(chan struct{})}
	authzv4.RegisterAuthorizationServiceServer(server, iam)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })
	client, err := sdk.NewClient(context.Background(), &sdk.Config{
		Endpoint: "passthrough:///bufnet", Timeout: time.Second, DialTimeout: time.Second,
		TLS: &sdk.TLSConfig{Enabled: false}, Retry: &sdk.RetryConfig{Enabled: false},
	}, sdk.WithDisableDefaultInterceptors(), sdk.WithDialOptions(
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	committed := int64(70)
	guard, err := NewVersionGuard(versionReaderFunc(func(context.Context) (int64, error) {
		return committed, nil
	}), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	loader := NewSnapshotLoader(&contractGRPCClient{client: client}, SnapshotLoaderOptions{VersionGuard: guard})
	finished := make(chan error, 1)
	go func() { _, err := loader.Load(context.Background(), "42"); finished <- err }()
	select {
	case <-iam.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("IAM snapshot call did not begin")
	}
	committed = 71
	if err := guard.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	close(iam.release)
	select {
	case err := <-finished:
		if err == nil {
			t.Fatal("in-flight pre-revocation snapshot passed after committed version advanced")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("IAM snapshot call did not finish")
	}
	if loader.getCached(cacheKey("42", "qs")) != nil {
		t.Fatal("in-flight stale snapshot remained usable in the cache")
	}
}

func TestSnapshotLoaderRejectsCachedAndRuntimeStalePermissions(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	iam := &laggingSnapshotIAM{version: 50}
	authzv4.RegisterAuthorizationServiceServer(server, iam)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })
	client, err := sdk.NewClient(context.Background(), &sdk.Config{
		Endpoint: "passthrough:///bufnet", Timeout: time.Second, DialTimeout: time.Second,
		TLS: &sdk.TLSConfig{Enabled: false}, Retry: &sdk.RetryConfig{Enabled: false},
	}, sdk.WithDisableDefaultInterceptors(), sdk.WithDialOptions(
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
	))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	clock := &guardClock{at: time.Now()}
	version := int64(50)
	outage := false
	guard, err := NewVersionGuard(versionReaderFunc(func(context.Context) (int64, error) {
		if outage {
			return 0, errors.New("IAM policy database unavailable")
		}
		return version, nil
	}), 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	guard.now = clock.now
	loader := NewSnapshotLoader(&contractGRPCClient{client: client}, SnapshotLoaderOptions{
		CacheTTL: time.Minute, VersionGuard: guard,
	})
	if snap, err := loader.Load(context.Background(), "42"); err != nil || snap.AuthzVersion != 50 {
		t.Fatalf("initial IAM snapshot = %#v, %v", snap, err)
	}
	clock.advance(5 * time.Second)
	version = 51 // Policy committed, but IAM's in-memory runtime still has v50.
	if err := guard.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := loader.Load(context.Background(), "42"); err == nil {
		t.Fatal("cached or freshly fetched stale permission passed after committed revocation")
	}
	iam.mu.Lock()
	iam.version = 51
	iam.mu.Unlock()
	if snap, err := loader.Load(context.Background(), "42"); err != nil || snap.AuthzVersion != 51 {
		t.Fatalf("reconciled IAM snapshot = %#v, %v", snap, err)
	}
	outage = true
	clock.advance(10 * time.Second)
	if _, err := loader.Load(context.Background(), "42"); err == nil {
		t.Fatal("cached permission passed after policy database proof expired")
	}
}
