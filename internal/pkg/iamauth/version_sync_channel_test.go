package iamauth

import (
	"strings"
	"testing"

	gonq "github.com/nsqio/go-nsq"
)

func TestEphemeralVersionSyncChannelIsPerProcessAndWithinNSQLimit(t *testing.T) {
	for _, service := range []string{"qs-authz-sync-apiserver", strings.Repeat("long-prefix-", 20)} {
		channel := EphemeralVersionSyncChannel(service)
		if !strings.HasSuffix(channel, "#ephemeral") || !gonq.IsValidChannelName(channel) {
			t.Fatalf("invalid ephemeral NSQ channel %q", channel)
		}
		if channel != EphemeralVersionSyncChannel(service) {
			t.Fatalf("per-process channel changed within one process: %q", channel)
		}
	}
	first := EphemeralVersionSyncChannel(strings.Repeat("long-prefix-", 20) + "a")
	second := EphemeralVersionSyncChannel(strings.Repeat("long-prefix-", 20) + "b")
	if first == second {
		t.Fatal("long channel names lost distinct suffixes")
	}
}
