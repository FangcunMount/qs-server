package worker

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/FangcunMount/component-base/pkg/database"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	workerconfig "github.com/FangcunMount/qs-server/internal/worker/config"
	workeroptions "github.com/FangcunMount/qs-server/internal/worker/options"
	"github.com/alicebob/miniredis/v2"
)

func TestWorkerDatabaseManagerGetRedisClientByProfile(t *testing.T) {
	mr := miniredis.RunT(t)
	host, port := splitTestMiniredisAddr(t, mr.Addr())

	opts := workeroptions.NewOptions()
	opts.Redis.Host = host
	opts.Redis.Port = port
	opts.Redis.Database = 1
	lockOpts := &genericoptions.RedisOptions{
		Database: 5,
	}
	opts.RedisProfiles["lock_cache"] = lockOpts

	badOpts := workeroptions.NewOptions().Redis
	badOpts.Host = host
	badOpts.Port = 63999
	opts.RedisProfiles["sdk_cache"] = badOpts

	cfg, err := workerconfig.CreateConfigFromOptions(opts)
	if err != nil {
		t.Fatalf("CreateConfigFromOptions() error = %v", err)
	}

	dm := NewDatabaseManager(cfg)
	if err := dm.Initialize(); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	t.Cleanup(func() {
		_ = dm.Close()
	})

	ctx := context.Background()

	defaultClient, err := dm.GetRedisClient()
	if err != nil {
		t.Fatalf("GetRedisClient() error = %v", err)
	}
	if err := defaultClient.Set(ctx, "shared:key", "default", 0).Err(); err != nil {
		t.Fatalf("set default key failed: %v", err)
	}

	fallbackClient, err := dm.GetRedisClientByProfile("query_cache")
	if err != nil {
		t.Fatalf("GetRedisClientByProfile(query_cache) error = %v", err)
	}
	if got, err := fallbackClient.Get(ctx, "shared:key").Result(); err != nil || got != "default" {
		t.Fatalf("fallback client should read default db key, got value=%q err=%v", got, err)
	}

	lockClient, err := dm.GetRedisClientByProfile("lock_cache")
	if err != nil {
		t.Fatalf("GetRedisClientByProfile(lock_cache) error = %v", err)
	}
	if err := lockClient.Set(ctx, "shared:key", "lock", 0).Err(); err != nil {
		t.Fatalf("set lock key failed: %v", err)
	}
	if got, _ := lockClient.Get(ctx, "shared:key").Result(); got != "lock" {
		t.Fatalf("lock db value = %q, want lock", got)
	}

	if status := dm.GetRedisProfileStatus("query_cache"); status.State != database.RedisProfileStateMissing {
		t.Fatalf("query_cache profile state = %q, want missing", status.State)
	}
	if status := dm.GetRedisProfileStatus("lock_cache"); status.State != database.RedisProfileStateAvailable {
		t.Fatalf("lock_cache profile state = %q, want available", status.State)
	}
	if status := dm.GetRedisProfileStatus("sdk_cache"); status.State != database.RedisProfileStateUnavailable {
		t.Fatalf("sdk_cache profile state = %q, want unavailable", status.State)
	}
	if _, err := dm.GetRedisClientByProfile("sdk_cache"); err == nil {
		t.Fatalf("GetRedisClientByProfile(sdk_cache) unexpectedly succeeded")
	}
}

func splitTestMiniredisAddr(t *testing.T, addr string) (string, int) {
	t.Helper()

	host, portStr, ok := strings.Cut(addr, ":")
	if !ok {
		t.Fatalf("unexpected miniredis addr %q", addr)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse miniredis port failed: %v", err)
	}
	return host, port
}
