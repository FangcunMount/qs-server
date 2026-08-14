package cachegovernance

import (
	"context"
	"errors"
	"testing"
	"time"

	componenterrors "github.com/FangcunMount/component-base/pkg/errors"
	cachemodel "github.com/FangcunMount/qs-server/internal/apiserver/cache/governance/model"
	sharedcache "github.com/FangcunMount/qs-server/internal/pkg/cache"
	"github.com/FangcunMount/qs-server/internal/pkg/code"
)

func TestPolicyReloaderPublishesCASAndPreservesSnapshotOnFailure(t *testing.T) {
	current := sharedcache.EffectiveCapability{Capability: "statistics.query", Enabled: true, Policy: sharedcache.Policy{TTL: time.Minute}}
	registry := sharedcache.NewRegistry(current)
	candidate := current
	candidate.Policy.TTL = 2 * time.Minute
	source := sharedcache.PolicySource{Component: "qs-apiserver", SchemaVersion: "1.0", Path: "/tmp/cache.yaml", PolicySHA256: "hash-2"}
	reloader := NewPolicyReloader("test", registry, func(context.Context) ([]sharedcache.EffectiveCapability, sharedcache.PolicySource, error) {
		return []sharedcache.EffectiveCapability{candidate}, source, nil
	})

	result, err := reloader.ReloadPolicy(context.Background(), 7, cachemodel.CachePolicyReloadRequest{ExpectedVersion: 1})
	if err != nil {
		t.Fatalf("ReloadPolicy() error = %v", err)
	}
	if !result.Changed || result.PreviousVersion != 1 || result.CurrentVersion != 2 {
		t.Fatalf("ReloadPolicy() = %#v", result)
	}
	if len(result.ChangedCapabilities) != 1 || result.ChangedCapabilities[0] != "statistics.query" {
		t.Fatalf("ChangedCapabilities = %#v", result.ChangedCapabilities)
	}
	if effective, _ := registry.Resolve("statistics.query"); effective.Policy.TTL != 2*time.Minute {
		t.Fatalf("effective TTL = %s", effective.Policy.TTL)
	}

	_, err = reloader.ReloadPolicy(context.Background(), 7, cachemodel.CachePolicyReloadRequest{ExpectedVersion: 1})
	if err == nil || !componenterrors.IsCode(err, code.ErrConflict) {
		t.Fatalf("stale ReloadPolicy() error = %v, want conflict", err)
	}
	if registry.Version() != 2 {
		t.Fatalf("version after conflict = %d, want 2", registry.Version())
	}
}

func TestPolicyReloaderNoopAndLoaderFailureDoNotBumpVersion(t *testing.T) {
	entry := sharedcache.EffectiveCapability{Capability: "plan.detail", Enabled: true, Policy: sharedcache.Policy{TTL: time.Hour}}
	source := sharedcache.PolicySource{Component: "qs-apiserver", SchemaVersion: "1.0", Path: "config.yaml", PolicySHA256: "hash"}
	registry := sharedcache.NewRegistryWithSource(source, entry)
	reloader := NewPolicyReloader("test", registry, func(context.Context) ([]sharedcache.EffectiveCapability, sharedcache.PolicySource, error) {
		return []sharedcache.EffectiveCapability{entry}, source, nil
	})
	result, err := reloader.ReloadPolicy(context.Background(), 1, cachemodel.CachePolicyReloadRequest{ExpectedVersion: 1})
	if err != nil || result.Changed || registry.Version() != 1 {
		t.Fatalf("no-op reload result=%#v err=%v version=%d", result, err, registry.Version())
	}

	loadErr := errors.New("cannot read config")
	reloader.loader = func(context.Context) ([]sharedcache.EffectiveCapability, sharedcache.PolicySource, error) {
		return nil, sharedcache.PolicySource{}, loadErr
	}
	if _, err := reloader.ReloadPolicy(context.Background(), 1, cachemodel.CachePolicyReloadRequest{ExpectedVersion: 1}); !errors.Is(err, loadErr) {
		t.Fatalf("loader failure = %v", err)
	}
	if registry.Version() != 1 {
		t.Fatalf("version after loader failure = %d", registry.Version())
	}
	status := reloader.ReloadStatus()
	if status.LastFailureAt.IsZero() || status.LastError == "" {
		t.Fatalf("reload status = %#v", status)
	}
}

func TestStatusServiceProjectsEffectiveRegistryLayers(t *testing.T) {
	entry := sharedcache.EffectiveCapability{
		Capability: "plan.detail", Owner: "plan", Kind: sharedcache.KindCache, Layer: sharedcache.LayerL2,
		Family: "object_view", Enabled: true, CatalogVersion: "v2", Source: "cache.capabilities.plan.detail",
		Layers: sharedcache.PolicyLayers{SpecDefault: sharedcache.Policy{TTL: time.Hour}, Override: sharedcache.Policy{TTL: 2 * time.Hour}},
		Policy: sharedcache.Policy{TTL: 2 * time.Hour, Compress: sharedcache.PolicySwitchDisabled, Singleflight: sharedcache.PolicySwitchEnabled, Negative: sharedcache.PolicySwitchDisabled},
	}
	source := sharedcache.PolicySource{Component: "qs-apiserver", SchemaVersion: "1.0", Path: "/app/configs/cache/apiserver.prod.yaml", PolicySHA256: "abc"}
	registry := sharedcache.NewRegistryWithSource(source, entry)
	service := NewStatusService("apiserver", nil, nil, nil, registry)
	status, err := service.GetStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.EffectiveRegistry == nil || status.EffectiveRegistry.SnapshotVersion != 1 || status.EffectiveRegistry.CatalogVersion != "v2" {
		t.Fatalf("effective registry = %#v", status.EffectiveRegistry)
	}
	if status.EffectiveRegistry.PolicySource == nil || status.EffectiveRegistry.PolicySource.PolicySHA256 != "abc" {
		t.Fatalf("policy source = %#v", status.EffectiveRegistry.PolicySource)
	}
	capability := status.EffectiveRegistry.Capabilities[0]
	if capability.Override.TTL != "2h0m0s" || capability.Effective.Singleflight != "enabled" {
		t.Fatalf("capability policy view = %#v", capability)
	}
}
