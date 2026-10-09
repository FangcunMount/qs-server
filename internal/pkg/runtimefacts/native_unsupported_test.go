//go:build !linux

package runtimefacts

import (
	"strings"
	"testing"
)

func TestUnsupportedNativeChannelDoesNotBlockDevelopmentIdentity(t *testing.T) {
	o, err := New("collection-server", strings.Repeat("a", 40))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = o.Close() }()
	if o.NoLocalMQ() != nil || o.ClientID("development") == "" {
		t.Fatal("development owner unavailable")
	}
	if o.Start() == nil {
		t.Fatal("unsupported native channel qualified")
	}
	if o.Start() == nil {
		t.Fatal("unsupported native channel qualified or retried")
	}
	if got := o.Snapshot(); got.ObservationComplete || got.BrokerConnectionsVerified || !contains(got.IncompleteReasons, "native_identity_unavailable") || !contains(got.IncompleteReasons, "private_channel_unavailable") {
		t.Fatal("unsupported platform observation qualified")
	}
}
