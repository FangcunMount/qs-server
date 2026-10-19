//go:build integration && rocketmq_m3

package iamauth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	appauthz "github.com/FangcunMount/qs-server/internal/apiserver/application/authz"
	hosttransport "github.com/FangcunMount/qs-server/internal/pkg/eventing/transport"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
	rmtransport "github.com/FangcunMount/reliable-messaging/transport"
	"github.com/FangcunMount/reliable-messaging/wire/legacy"
	mysqldsn "github.com/go-sql-driver/mysql"
)

// This child is invoked only by the isolated infra/IAM parent. The reader port
// uses the parent's real committed primary row, not an IAM gRPC server. It
// proves the actual QS subscriber/cache/proof boundaries, not system login.
func TestM3RocketMQIAMFanoutChild(t *testing.T) {
	if !strings.HasPrefix(os.Getenv("IAM_M3_FIXTURE_PROJECT"), "rocketmq-m2-") {
		t.Fatal("owned parent fixture required")
	}
	var input struct {
		DSN     string `json:"dsn"`
		Config  string `json:"config"`
		Ready   string `json:"ready"`
		Target  string `json:"target"`
		Receipt string `json:"receipt"`
	}
	inputPath := os.Getenv("QS_M3_FANOUT_INPUT")
	raw, err := os.ReadFile(inputPath)
	if err != nil || json.Unmarshal(raw, &input) != nil {
		t.Fatal("private child input required")
	}
	root := filepath.Dir(inputPath)
	info, err := os.Stat(root)
	if err != nil || info.Mode().Perm()&0077 != 0 {
		t.Fatal("owned private input directory required")
	}
	for _, path := range []string{input.Ready, input.Target, input.Receipt} {
		if filepath.Dir(path) != root {
			t.Fatal("child output escaped owned fixture")
		}
	}
	cfg, err := mysqldsn.ParseDSN(input.DSN)
	if err != nil {
		t.Fatal("owned primary reader required")
	}
	host, _, err := net.SplitHostPort(cfg.Addr)
	if err != nil || cfg.Net != "tcp" || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() || !strings.HasPrefix(cfg.DBName, "iam_m3_broker_test_") {
		t.Fatal("owned loopback parent schema required")
	}
	db, err := sql.Open("mysql", input.DSN)
	if err != nil {
		t.Fatal("primary reader construction failed")
	}
	db.SetMaxOpenConns(2)
	t.Cleanup(func() {
		if db.Close() != nil {
			t.Error("primary reader did not close")
		}
	})
	reader := versionReaderFunc(func(ctx context.Context) (int64, error) {
		var version int64
		err := db.QueryRowContext(ctx, "SELECT policy_version FROM authz_policy_versions WHERE id=1").Scan(&version)
		return version, err
	})
	guard, err := NewVersionGuard(reader, 10*time.Second)
	if err != nil {
		t.Fatal("proof construction failed")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 70*time.Second)
	defer cancel()
	initial, err := guard.Verify(ctx)
	if err != nil {
		t.Fatal("initial primary proof failed")
	}
	loader := NewSnapshotLoader(nil, SnapshotLoaderOptions{AppName: "qs", CacheTTL: time.Minute, VersionGuard: guard})
	keys := []string{cacheKey("m3-fixture-a", "qs"), cacheKey("m3-fixture-b", "qs")}
	for _, key := range keys {
		if loader.setCached(key, &appauthz.Snapshot{AuthzVersion: initial}) != nil {
			t.Fatal("seeded stale cache invalid")
		}
	}
	var config options.RocketMQOptions
	raw, err = os.ReadFile(input.Config)
	if err != nil || json.Unmarshal(raw, &config) != nil {
		t.Fatal("owned subscriber config missing")
	}
	var unexpected atomic.Bool
	sub, err := hosttransport.NewSDKDeliverySubscriber(hosttransport.SubscriberConfig{Provider: "rocketmq", RocketMQ: &config, NSQMessageTimeout: 5 * time.Second}, 2, 3,
		func(context.Context, legacy.FailedHandoff) error {
			unexpected.Store(true)
			return errors.New("unexpected NSQ handoff")
		},
		func(context.Context, rmtransport.Failure) error {
			unexpected.Store(true)
			return errors.New("unexpected RocketMQ handoff")
		})
	if err != nil {
		t.Fatal("QS actual subscriber construction failed")
	}
	t.Cleanup(func() {
		if sub.Close() != nil {
			t.Error("QS subscriber did not drain")
		}
	})
	if SubscribeVersionChangesSDK(ctx, sub, DefaultVersionTopic, "qs-m3-native-fanout", loader) != nil {
		t.Fatal("QS actual subscription failed")
	}
	write := func(path string, v any) {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal("child proof encoding failed")
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal("new private child proof required")
		}
		_, writeErr := f.Write(raw)
		closeErr := f.Close()
		if writeErr != nil || closeErr != nil {
			t.Fatal("child proof write failed")
		}
	}
	write(input.Ready, map[string]any{"initial_version": initial})
	var target int64
	for target == 0 {
		raw, err = os.ReadFile(input.Target)
		if err == nil {
			if json.Unmarshal(raw, &target) != nil || target <= initial {
				t.Fatal("invalid target version")
			}
			break
		}
		if !os.IsNotExist(err) {
			t.Fatal("target read failed")
		}
		select {
		case <-ctx.Done():
			t.Fatal("target not supplied")
		case <-time.After(20 * time.Millisecond):
		}
	}
	// Do not call the independent reader while awaiting the hint: a periodic
	// primary read must not masquerade as proof of actual Broker fanout.
	for {
		loader.mu.Lock()
		observed := loader.globalVersion
		loader.mu.Unlock()
		if observed >= target {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("QS notification was not delivered")
		case <-time.After(20 * time.Millisecond):
		}
	}
	for _, key := range keys {
		if loader.getCached(key) != nil {
			t.Fatal("QS retained an old subject snapshot")
		}
	}
	if loader.VerifySnapshot(ctx, &appauthz.Snapshot{AuthzVersion: initial}) == nil {
		t.Fatal("QS accepted old in-flight snapshot")
	}
	if loader.VerifySnapshot(ctx, &appauthz.Snapshot{AuthzVersion: target}) != nil {
		t.Fatal("fresh committed proof unavailable")
	}
	if unexpected.Load() {
		t.Fatal("unexpected terminal delivery")
	}
	if sub.Close() != nil {
		t.Fatal("actual QS subscriber close failed")
	}
	write(input.Receipt, map[string]any{"notification_received": true, "both_subjects_evicted": true, "old_snapshot_rejected": true, "primary_proof_verified": true, "subscriber_drained": true, "target_version": target})
}
