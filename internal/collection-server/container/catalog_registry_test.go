package container

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/typologymodel"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
)

func TestCatalogL1CacheDisabledReturnsNil(t *testing.T) {
	t.Parallel()

	opts := options.NewOptions()
	opts.QuestionnaireCache.Enabled = false
	opts.ScaleCache.Enabled = false
	opts.TypologyCache.Enabled = false

	if got := newCatalogL1Cache(opts, catalogKindQuestionnaire); got != nil {
		t.Fatalf("questionnaire cache = %T, want nil", got)
	}
	if got := newCatalogL1Cache(opts, catalogKindTypology); got != nil {
		t.Fatalf("typology cache = %T, want nil", got)
	}
}

func TestCatalogL1CacheEnabledBuildsTypedCaches(t *testing.T) {
	t.Parallel()

	opts := options.NewOptions()
	opts.QuestionnaireCache.Enabled = true
	opts.ScaleCache.Enabled = true
	opts.TypologyCache.Enabled = true

	if _, ok := newCatalogL1Cache(opts, catalogKindQuestionnaire).(*questionnaire.LocalCache); !ok {
		t.Fatal("questionnaire cache type mismatch")
	}
	if _, ok := newCatalogL1Cache(opts, catalogKindTypology).(*typologymodel.LocalCatalogCache); !ok {
		t.Fatal("typology cache type mismatch")
	}
}

func TestCatalogL1SingleflightEnabled(t *testing.T) {
	t.Parallel()

	opts := options.NewOptions()
	opts.TypologyCache.Singleflight = true
	opts.QuestionnaireCache.Singleflight = false

	if !catalogL1SingleflightEnabled(opts, catalogKindTypology) {
		t.Fatal("typology singleflight should be enabled")
	}
	if catalogL1SingleflightEnabled(opts, catalogKindQuestionnaire) {
		t.Fatal("questionnaire singleflight should be disabled by default")
	}
}

func TestCatalogSignalEvictRequiresEnabledL1(t *testing.T) {
	t.Parallel()

	disabled := &options.CatalogL1CacheOptions{Enabled: false, SignalEvictEnabled: true}
	if catalogSignalEvictEnabled(disabled) {
		t.Fatal("signal evict should not run when L1 is disabled")
	}

	enabled := &options.CatalogL1CacheOptions{Enabled: true, SignalEvictEnabled: true}
	if !catalogSignalEvictEnabled(enabled) {
		t.Fatal("signal evict should run when L1 and evict are enabled")
	}
}
