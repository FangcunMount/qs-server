package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/reliable-messaging/transaction"
	"github.com/FangcunMount/reliable-messaging/transport/rocketmq"
)

func TestOwnerDrainRequiresIndependentReviewedAction(t *testing.T) {
	base := []string{"--mysql-database=qs", "--mongo-database=qs", "--owner-drain", "--owner-drain-config=/app/config.json", "--owner-drain-config-sha256=" + strings.Repeat("a", 64), "--owner-drain-operation=123-1", "--owner-drain-mysql-identity=" + strings.Repeat("b", 64), "--owner-drain-mongo-identity=" + strings.Repeat("c", 64)}
	if a, err := parseArguments(base); err != nil || !a.ownerDrain.enabled {
		t.Fatal(a, err)
	}
	for _, extra := range [][]string{{"--protocol-observation", "--timeout=30s"}, {"--apply", "--approval-file=/tmp/x", "--approval-sha256=" + strings.Repeat("a", 64)}, {"--owner-drain-operation=../reuse"}, {"--mongo-database="}, {"--owner-drain=false"}} {
		if _, err := parseArguments(append(append([]string{}, base...), extra...)); err == nil {
			t.Fatal("ambiguous drain accepted", extra)
		}
	}
}

func TestOwnerDrainPausesOnlyOriginalConfigAndRequiresBothOwners(t *testing.T) {
	open := true
	o := options.MessagingOptions{Enabled: true, Provider: "rocketmq", RocketMQ: &options.RocketMQOptions{PublishEnabled: &open, Transactions: map[string]options.TransactionOptions{}}}
	for _, owner := range []string{"qs.mysql", "qs.mongo"} {
		o.RocketMQ.Transactions[owner] = options.TransactionOptions{StoreID: owner + "-primary", WriterEpoch: "m3-1", RouteRevision: "m3-1"}
	}
	encode := func() ([]byte, string) {
		raw, err := json.Marshal(map[string]any{"messaging": o})
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(raw)
		return raw, hex.EncodeToString(h[:])
	}
	raw, digest := encode()
	paused, err := drainConfiguration(raw, digest)
	if err != nil || paused.RocketMQ.PublishingEnabled() || !o.RocketMQ.PublishingEnabled() {
		t.Fatal("pause changed original or failed", err)
	}
	if _, err = drainConfiguration(append(raw, ' '), digest); err == nil {
		t.Fatal("changed bytes accepted")
	}
	delete(o.RocketMQ.Transactions, "qs.mongo")
	raw, digest = encode()
	if _, err = drainConfiguration(raw, digest); err == nil {
		t.Fatal("missing owner accepted")
	}
}

func TestOwnerDrainPreservesProductionStringDurationsAndLiteralOwners(t *testing.T) {
	binding := `{"store_id":"original","writer_epoch":"m3-1","route_revision":"m3-1","limits":{"sends":4,"branches":4,"check_workers":2,"check_queue":16,"reconcile_batch":50,"check_timeout":"2s","rpc_timeout":"5s","lease":"30s","reconcile_interval":"1s"}}`
	raw := []byte(`{"messaging":{"enabled":true,"provider":"rocketmq","rocketmq":{"publish_enabled":true,"transactions":{"qs.mysql":` + binding + `,"qs.mongo":` + binding + `}}}}`)
	h := sha256.Sum256(raw)
	o, err := drainConfiguration(raw, hex.EncodeToString(h[:]))
	if err != nil {
		t.Fatal(err)
	}
	for _, owner := range []string{"qs.mysql", "qs.mongo"} {
		l, err := o.RocketMQ.Transactions[owner].ProtocolLimits(owner)
		if err != nil || l.CheckTimeout != 2*time.Second || l.RPCTimeout != 5*time.Second || l.CheckWorkers != 2 {
			t.Fatal(owner, l, err)
		}
	}
}

func TestOwnerDrainUnknownActiveOrUnresolvedCannotStop(t *testing.T) {
	base := drainOwnerReport{SDK: rocketmq.TransactionStats{Ready: true}, Protocol: transaction.ProtocolObservation{AsOf: time.Now().UTC()}, Protection: messagingruntime.ProtocolProtectionStats{Available: true, Fresh: true, SamplerRunning: true}}
	if !drainSampleSafe(base) {
		t.Fatal("known idle original owner rejected")
	}
	for _, change := range []func(*drainOwnerReport){
		func(r *drainOwnerReport) { r.SDK.Ready = false }, func(r *drainOwnerReport) { r.SDK.RunningChecks = 1 }, func(r *drainOwnerReport) { r.SDK.QueuedChecks = 1 }, func(r *drainOwnerReport) { r.SDK.ActiveSends = 1 }, func(r *drainOwnerReport) { r.SDK.Stopping = true },
		func(r *drainOwnerReport) { r.Protection.Fresh = false }, func(r *drainOwnerReport) { r.Protection.NativeLimitActive = true }, func(r *drainOwnerReport) { r.Protocol.AsOf = time.Time{} },
		func(r *drainOwnerReport) { r.Protocol.PreparingExecutions = 1 }, func(r *drainOwnerReport) { r.Protocol.ReservedExecutions = 1 }, func(r *drainOwnerReport) { r.Protocol.MetadataConflicts = 1 }, func(r *drainOwnerReport) { r.Protocol.ConfirmationGapBranches = 1 }, func(r *drainOwnerReport) { r.Protocol.UntrackedAbortedBranches = 1 }, func(r *drainOwnerReport) { r.Protocol.UnobservedCommittedBranches = 1 }, func(r *drainOwnerReport) { r.Protocol.UnagedConfirmationGapBranches = 1 }, func(r *drainOwnerReport) { r.Protocol.PreparedExpiryAgeSeconds = 1 }, func(r *drainOwnerReport) { r.Protocol.OldestConfirmationObservationAgeSeconds = 1 },
	} {
		r := base
		change(&r)
		if drainSampleSafe(r) {
			t.Fatal("unknown/active responsibility accepted")
		}
	}
}

func TestOwnerDrainStopRequestCannotBorrowOperationOrPublicFile(t *testing.T) {
	raw := []byte(`{"operation_id":"123-1","writers_stopped":true,"replacement_owner_ready":false,"receipt_sha256":"` + strings.Repeat("a", 64) + `"}`)
	if !drainStopRequest(raw, "123-1") || drainStopRequest(raw, "124-1") {
		t.Fatal("stop operation not exact")
	}
	p := filepath.Join(t.TempDir(), "stop.json")
	if err := os.WriteFile(p, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if !readDrainStop(p, "123-1") {
		t.Fatal("private exact request rejected")
	}
	if err := os.Chmod(p, 0644); err != nil {
		t.Fatal(err)
	}
	if readDrainStop(p, "123-1") {
		t.Fatal("public request accepted")
	}
	if err := os.Chmod(p, 0600); err != nil {
		t.Fatal(err)
	}
	alias := p + ".alias"
	if err := os.Symlink(p, alias); err != nil {
		t.Fatal(err)
	}
	if readDrainStop(alias, "123-1") {
		t.Fatal("symlink request accepted")
	}
}
