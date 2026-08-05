package subsystem

import (
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	locksubsystem "github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease/subsystem"
	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
)

func TestSnapshotDoesNotInventWorkerRateOrBackpressureCapabilities(t *testing.T) {
	s, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := s.Snapshot(time.Now())
	if snapshot.InstanceID == "" || len(snapshot.RateLimits) != 0 || len(snapshot.Backpressure) != 0 {
		t.Fatalf("Snapshot() = %+v", snapshot)
	}
	if len(snapshot.DuplicateSuppression) != 1 || !snapshot.DuplicateSuppression[0].Degraded {
		t.Fatalf("duplicate suppression = %+v", snapshot.DuplicateSuppression)
	}
}

func TestSnapshotKeepsDuplicateSuppressionHealthyWithAttentionLeaderLock(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	locks := locksubsystem.New(locksubsystem.Options{
		Component: "worker",
		Handle: &redisruntime.Handle{
			Family:     redisruntime.FamilyLock,
			Client:     client,
			Builder:    keyspace.NewBuilderWithNamespace("cache:lock"),
			Configured: true,
			Available:  true,
		},
		RenewalEnabled: true,
	})
	s, err := New(Options{Locks: locks})
	if err != nil {
		t.Fatal(err)
	}

	snapshot := s.Snapshot(time.Now())
	if len(snapshot.Locks) != 2 {
		t.Fatalf("locks = %+v, want answersheet and attention capabilities", snapshot.Locks)
	}
	if len(snapshot.DuplicateSuppression) != 1 || !snapshot.DuplicateSuppression[0].Configured || snapshot.DuplicateSuppression[0].Degraded {
		t.Fatalf("duplicate suppression = %+v, want healthy capability with two worker locks", snapshot.DuplicateSuppression)
	}
	if !snapshot.Summary.Ready || snapshot.Summary.DegradedCount != 0 {
		t.Fatalf("summary = %+v, want ready worker resilience snapshot", snapshot.Summary)
	}
}
