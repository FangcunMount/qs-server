package observability_test

import (
	"context"
	"errors"
	"testing"

	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/redisruntime/keyspace"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease"
	"github.com/FangcunMount/qs-server/internal/pkg/resilience/locklease/redisadapter"
	"github.com/alicebob/miniredis/v2"
	"github.com/prometheus/client_golang/prometheus"
	redis "github.com/redis/go-redis/v9"
)

func TestAdapterCancellationEmitsCanonicalMetrics(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	const component = "locklease-canonical-cancellation-test"
	manager := redisadapter.NewManager(component, "lock_lease", &redisruntime.Handle{
		Family:  redisruntime.FamilyLock,
		Client:  client,
		Builder: keyspace.NewBuilderWithNamespace("cache:lock"),
	})
	capability, _ := locklease.Lookup(locklease.WorkloadAnswersheetProcessing)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	beforeAcquire := operationTotal(t, component, capability.Spec.Name, "acquire", "canceled")
	if _, acquired, err := manager.AcquireSpec(canceled, capability.Spec, "answersheet:processing:canceled-acquire"); !errors.Is(err, context.Canceled) || acquired {
		t.Fatalf("AcquireSpec() acquired=%v err=%v, want canceled", acquired, err)
	}
	assertOperationIncremented(t, component, capability.Spec.Name, "acquire", "canceled", beforeAcquire)

	lease, acquired, err := manager.AcquireSpec(context.Background(), capability.Spec, "answersheet:processing:canceled-renew-release")
	if err != nil || !acquired {
		t.Fatalf("AcquireSpec() acquired=%v err=%v", acquired, err)
	}
	beforeRenew := operationTotal(t, component, capability.Spec.Name, "renew", "canceled")
	if owned, renewErr := manager.RenewSpec(canceled, capability.Spec, "answersheet:processing:canceled-renew-release", lease); !errors.Is(renewErr, context.Canceled) || owned {
		t.Fatalf("RenewSpec() owned=%v err=%v, want canceled", owned, renewErr)
	}
	assertOperationIncremented(t, component, capability.Spec.Name, "renew", "canceled", beforeRenew)

	beforeRelease := operationTotal(t, component, capability.Spec.Name, "release", "canceled")
	if releaseErr := manager.ReleaseSpec(canceled, capability.Spec, "answersheet:processing:canceled-renew-release", lease); !errors.Is(releaseErr, context.Canceled) {
		t.Fatalf("ReleaseSpec() error=%v, want canceled", releaseErr)
	}
	assertOperationIncremented(t, component, capability.Spec.Name, "release", "canceled", beforeRelease)
	if releaseErr := manager.ReleaseSpec(context.Background(), capability.Spec, "answersheet:processing:canceled-renew-release", lease); releaseErr != nil {
		t.Fatalf("ReleaseSpec() cleanup error=%v", releaseErr)
	}
}

func assertOperationIncremented(t *testing.T, component, name, operation, result string, before float64) {
	t.Helper()
	if got := operationTotal(t, component, name, operation, result); got != before+1 {
		t.Fatalf("%s/%s operation total = %v, want %v", operation, result, got, before+1)
	}
}

func operationTotal(t *testing.T, component, name, operation, result string) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather Prometheus metrics: %v", err)
	}
	wanted := map[string]string{
		"component": component,
		"name":      name,
		"operation": operation,
		"result":    result,
	}
	for _, family := range families {
		if family.GetName() != "qs_locklease_operation_total" {
			continue
		}
		for _, metric := range family.Metric {
			matched := len(metric.Label) == len(wanted)
			for _, label := range metric.Label {
				if wanted[label.GetName()] != label.GetValue() {
					matched = false
					break
				}
			}
			if matched {
				return metric.GetCounter().GetValue()
			}
		}
	}
	return 0
}
