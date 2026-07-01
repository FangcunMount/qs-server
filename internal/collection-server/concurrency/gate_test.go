package concurrency

import (
	"testing"
)

func TestGateTryAcquireAndRelease(t *testing.T) {
	gate := NewGate(1)
	if !gate.TryAcquire() {
		t.Fatal("expected first acquire to succeed")
	}
	if gate.TryAcquire() {
		t.Fatal("expected second acquire to fail")
	}
	gate.Release()
	if !gate.TryAcquire() {
		t.Fatal("expected acquire after release")
	}
}

func TestNilGateAlwaysAcquires(t *testing.T) {
	var gate *Gate
	if !gate.TryAcquire() {
		t.Fatal("nil gate should allow acquire")
	}
	gate.Release()
}
