package control

import (
	"errors"
	"testing"
)

func TestResolveInstanceIdentityReturnsRandomSourceFailure(t *testing.T) {
	original := readRandom
	readRandom = func([]byte) (int, error) { return 0, errors.New("random source failed") }
	t.Cleanup(func() { readRandom = original })

	identity, err := ResolveInstanceIdentity("collection-server", "collection-0")
	if err == nil || identity.Generation == "unknown" || identity.Generation != "" {
		t.Fatalf("identity=%+v err=%v, want startup error without fallback generation", identity, err)
	}
}

func TestResolveInstanceIdentityUsesFreshGeneration(t *testing.T) {
	first, err := ResolveInstanceIdentity("worker", "worker-0")
	if err != nil {
		t.Fatal(err)
	}
	second, err := ResolveInstanceIdentity("worker", "worker-0")
	if err != nil {
		t.Fatal(err)
	}
	if first.Generation == "" || first.Generation == second.Generation {
		t.Fatalf("generations=(%q,%q), want fresh non-empty values", first.Generation, second.Generation)
	}
}
