package iam

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/iamauth"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
)

type committedVersionReaderFunc func(context.Context) (int64, error)

func (f committedVersionReaderFunc) GetCommittedPolicyVersion(ctx context.Context) (int64, error) {
	return f(ctx)
}

func TestModuleVersionGuardPollsOnceAndStopsOnClose(t *testing.T) {
	t.Parallel()
	var reads atomic.Int32
	guard, err := iamauth.NewVersionGuard(committedVersionReaderFunc(func(context.Context) (int64, error) {
		reads.Add(1)
		return 1, nil
	}), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	module := &Module{authzVersionGuard: guard, guardPollInterval: 10 * time.Millisecond}
	module.StartAuthzVersionGuard()
	module.StartAuthzVersionGuard()
	deadline := time.After(time.Second)
	for reads.Load() < 2 {
		select {
		case <-deadline:
			t.Fatalf("version guard made %d reads, want initial read and poll", reads.Load())
		case <-time.After(time.Millisecond):
		}
	}
	if err := module.Close(); err != nil {
		t.Fatal(err)
	}
	count := reads.Load()
	time.Sleep(25 * time.Millisecond)
	if got := reads.Load(); got != count {
		t.Fatalf("version guard kept polling after close: %d -> %d", count, got)
	}
}

func TestConvertIAMOptionsMapsKeepalive(t *testing.T) {
	t.Parallel()

	opts := options.NewIAMOptions()
	converted := convertIAMOptions(opts)
	if converted.GRPC == nil {
		t.Fatal("GRPC = nil")
	}
	if converted.GRPC.KeepaliveTime != 5*time.Minute || converted.GRPC.KeepaliveTimeout != 20*time.Second || converted.GRPC.KeepalivePermitWithoutStream {
		t.Fatalf("converted keepalive = %#v", converted.GRPC)
	}
}
