package cache

import (
	"context"
	"testing"

	"github.com/FangcunMount/qs-server/internal/collection-server/options"
)

func TestSubsystemBuildsConfiguredTypedCaches(t *testing.T) {
	opts := options.NewOptions()
	opts.Cache.Capabilities.Catalog.Questionnaire.Enabled = true
	opts.Cache.Capabilities.Catalog.Questionnaire.Singleflight = false
	opts.Cache.Capabilities.Catalog.Typology.Enabled = true
	opts.Cache.Capabilities.Catalog.Typology.Singleflight = true

	s := NewSubsystem(opts, nil)
	if s.Questionnaire() == nil {
		t.Fatal("questionnaire cache = nil, want configured cache")
	}
	if s.Typology() == nil {
		t.Fatal("typology cache = nil, want configured cache")
	}
	if s.QuestionnaireSingleflight() {
		t.Fatal("questionnaire singleflight = true, want false")
	}
	if !s.TypologySingleflight() {
		t.Fatal("typology singleflight = false, want true")
	}
	entries := s.EffectiveRegistry().Snapshot()
	if len(entries) != 2 || entries[0].Capability != "catalog.questionnaire" || entries[1].Capability != "catalog.typology" {
		t.Fatalf("effective registry = %#v", entries)
	}
}

func TestSubsystemDisabledCachesStayNil(t *testing.T) {
	opts := options.NewOptions()
	opts.Cache.Capabilities.Catalog.Questionnaire.Enabled = false
	opts.Cache.Capabilities.Catalog.Typology.Enabled = false

	s := NewSubsystem(opts, nil)
	if s.Questionnaire() != nil || s.Typology() != nil {
		t.Fatal("disabled cache was constructed")
	}
}

func TestSubsystemStartCloseAreIdempotent(t *testing.T) {
	s := NewSubsystem(options.NewOptions(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	firstCancel := s.cancel
	if firstCancel == nil {
		t.Fatal("Start() did not install lifecycle cancel")
	}
	if err := s.Start(ctx); err != nil {
		t.Fatalf("second Start() error = %v", err)
	}
	if s.cancel == nil {
		t.Fatal("second Start() cleared lifecycle cancel")
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if s.started || s.cancel != nil {
		t.Fatalf("closed lifecycle state = started:%v cancel:%v", s.started, s.cancel != nil)
	}
}

func TestSignalEvictRequiresEnabledCache(t *testing.T) {
	if signalEvict(&options.CatalogL1CacheOptions{Enabled: false, SignalEvictEnabled: true}) {
		t.Fatal("signal eviction enabled for disabled cache")
	}
	if !signalEvict(&options.CatalogL1CacheOptions{Enabled: true, SignalEvictEnabled: true}) {
		t.Fatal("signal eviction disabled for enabled cache")
	}
}
