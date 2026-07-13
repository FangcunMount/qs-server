package cache

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestRegistryResolveSortCopyAndPublishCAS(t *testing.T) {
	registry := NewRegistry(
		EffectiveCapability{Capability: "z", Policy: Policy{TTL: time.Minute}},
		EffectiveCapability{Capability: "a", Policy: Policy{TTL: time.Second}},
	)
	if registry.Version() != 1 {
		t.Fatalf("Version() = %d, want 1", registry.Version())
	}
	all := registry.All()
	if len(all) != 2 || all[0].Capability != "a" || all[1].Capability != "z" {
		t.Fatalf("All() = %#v", all)
	}
	all[0].Policy.TTL = time.Hour
	got, _ := registry.Resolve("a")
	if got.Policy.TTL != time.Second {
		t.Fatalf("Resolve() TTL = %s, registry leaked caller mutation", got.Policy.TTL)
	}

	noChange, err := registry.Publish(1, registry.All(), time.Now())
	if err != nil || noChange.Changed || noChange.CurrentVersion != 1 {
		t.Fatalf("no-change Publish() = %+v, %v", noChange, err)
	}
	next := registry.All()
	next[0].Policy.TTL = 2 * time.Second
	changed, err := registry.Publish(1, next, time.Unix(2, 0))
	if err != nil || !changed.Changed || changed.CurrentVersion != 2 {
		t.Fatalf("changed Publish() = %+v, %v", changed, err)
	}
	if _, err := registry.Publish(1, next, time.Now()); !errors.Is(err, ErrRegistryVersionConflict) {
		t.Fatalf("stale Publish() error = %v", err)
	}
}

func TestRegistryConcurrentReadersSeeWholeSnapshots(t *testing.T) {
	registry := NewRegistry(
		EffectiveCapability{Capability: "a", Policy: Policy{TTL: time.Second}},
		EffectiveCapability{Capability: "b", Policy: Policy{TTL: time.Second}},
	)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				a, _ := registry.Resolve("a")
				b, _ := registry.Resolve("b")
				if a.Policy.TTL != time.Second && a.Policy.TTL != 2*time.Second {
					t.Errorf("a TTL = %s", a.Policy.TTL)
				}
				if b.Policy.TTL != time.Second && b.Policy.TTL != 2*time.Second {
					t.Errorf("b TTL = %s", b.Policy.TTL)
				}
			}
		}()
	}
	next := registry.All()
	for i := range next {
		next[i].Policy.TTL = 2 * time.Second
	}
	if _, err := registry.Publish(1, next, time.Now()); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
}
