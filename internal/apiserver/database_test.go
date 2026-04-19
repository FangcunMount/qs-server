package apiserver

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/FangcunMount/component-base/pkg/database"
	apiserverconfig "github.com/FangcunMount/qs-server/internal/apiserver/config"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/alicebob/miniredis/v2"
)

func TestDatabaseManagerGetRedisClientByProfile(t *testing.T) {
	mr := miniredis.RunT(t)
	host, port := splitTestMiniredisAddr(t, mr.Addr())

	dm := &DatabaseManager{
		registry: database.NewRegistry(),
		redisProfiles: database.NewNamedRedisRegistry(&database.RedisConfig{
			Host: host,
			Port: port,
		}, map[string]*database.RedisConfig{
			"object_cache": {
				Host:     host,
				Port:     port,
				Database: 2,
			},
			"sdk_cache": {
				Host: host,
				Port: 63999,
			},
		}),
	}
	if err := dm.redisProfiles.Connect(); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}
	t.Cleanup(func() {
		_ = dm.redisProfiles.Close()
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

	objectClient, err := dm.GetRedisClientByProfile("object_cache")
	if err != nil {
		t.Fatalf("GetRedisClientByProfile(object_cache) error = %v", err)
	}
	if err := objectClient.Set(ctx, "shared:key", "object", 0).Err(); err != nil {
		t.Fatalf("set object key failed: %v", err)
	}
	if got, _ := objectClient.Get(ctx, "shared:key").Result(); got != "object" {
		t.Fatalf("object db value = %q, want object", got)
	}

	if status := dm.GetRedisProfileStatus("query_cache"); status.State != database.RedisProfileStateMissing {
		t.Fatalf("query_cache profile state = %q, want missing", status.State)
	}
	if status := dm.GetRedisProfileStatus("object_cache"); status.State != database.RedisProfileStateAvailable {
		t.Fatalf("object_cache profile state = %q, want available", status.State)
	}
	if status := dm.GetRedisProfileStatus("sdk_cache"); status.State != database.RedisProfileStateUnavailable {
		t.Fatalf("sdk_cache profile state = %q, want unavailable", status.State)
	}
	if _, err := dm.GetRedisClientByProfile("sdk_cache"); err == nil {
		t.Fatalf("GetRedisClientByProfile(sdk_cache) unexpectedly succeeded")
	}
}

func TestDatabaseManagerInitRedisMergesDefaultIntoNamedProfiles(t *testing.T) {
	mr := miniredis.RunT(t)
	host, port := splitTestMiniredisAddr(t, mr.Addr())

	opts := apiserveroptions.NewOptions()
	opts.RedisOptions.Host = host
	opts.RedisOptions.Port = port
	opts.RedisOptions.Database = 1

	objectProfile := &genericoptions.RedisOptions{
		Database: 3,
	}
	opts.RedisProfiles["object_cache"] = objectProfile

	cfg, err := apiserverconfig.CreateConfigFromOptions(opts)
	if err != nil {
		t.Fatalf("CreateConfigFromOptions() error = %v", err)
	}

	dm := NewDatabaseManager(cfg)
	if err := dm.initRedis(context.Background()); err != nil {
		t.Fatalf("initRedis() error = %v", err)
	}
	if err := dm.redisProfiles.Connect(); err != nil {
		t.Fatalf("redisProfiles.Connect() error = %v", err)
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

	objectClient, err := dm.GetRedisClientByProfile("object_cache")
	if err != nil {
		t.Fatalf("GetRedisClientByProfile(object_cache) error = %v", err)
	}
	if err := objectClient.Set(ctx, "shared:key", "object", 0).Err(); err != nil {
		t.Fatalf("set object key failed: %v", err)
	}
	if got, _ := objectClient.Get(ctx, "shared:key").Result(); got != "object" {
		t.Fatalf("object db value = %q, want object", got)
	}
	if got, _ := defaultClient.Get(ctx, "shared:key").Result(); got != "default" {
		t.Fatalf("default db value = %q, want default", got)
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
