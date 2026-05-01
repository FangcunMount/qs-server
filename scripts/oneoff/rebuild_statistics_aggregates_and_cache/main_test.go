package main

import "testing"

func TestNewRedisClientUsesACLUsername(t *testing.T) {
	client := newRedisClient("127.0.0.1:6379", "stats-user", "secret", 3)
	defer client.Close()

	opts := client.Options()
	if opts.Username != "stats-user" {
		t.Fatalf("expected redis username to be propagated, got %q", opts.Username)
	}
	if opts.Password != "secret" {
		t.Fatalf("expected redis password to be propagated, got %q", opts.Password)
	}
	if opts.DB != 3 {
		t.Fatalf("expected redis db to be propagated, got %d", opts.DB)
	}
}
