package container

import (
	"testing"

	"github.com/FangcunMount/qs-server/internal/collection-server/application/personalitymodel"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/questionnaire"
	"github.com/FangcunMount/qs-server/internal/collection-server/application/scale"
	"github.com/FangcunMount/qs-server/internal/collection-server/options"
)

func TestCatalogL1CacheDisabledReturnsNil(t *testing.T) {
	t.Parallel()

	opts := options.NewOptions()
	opts.QuestionnaireCache.Enabled = false
	opts.ScaleCache.Enabled = false
	opts.PersonalityCache.Enabled = false

	if got := newCatalogL1Cache(opts, catalogKindQuestionnaire); got != nil {
		t.Fatalf("questionnaire cache = %T, want nil", got)
	}
	if got := newCatalogL1Cache(opts, catalogKindScale); got != nil {
		t.Fatalf("scale cache = %T, want nil", got)
	}
	if got := newCatalogL1Cache(opts, catalogKindPersonality); got != nil {
		t.Fatalf("personality cache = %T, want nil", got)
	}
}

func TestCatalogL1CacheEnabledBuildsTypedCaches(t *testing.T) {
	t.Parallel()

	opts := options.NewOptions()
	opts.QuestionnaireCache.Enabled = true
	opts.ScaleCache.Enabled = true
	opts.PersonalityCache.Enabled = true

	if _, ok := newCatalogL1Cache(opts, catalogKindQuestionnaire).(*questionnaire.LocalCache); !ok {
		t.Fatal("questionnaire cache type mismatch")
	}
	if _, ok := newCatalogL1Cache(opts, catalogKindScale).(*scale.LocalCatalogCache); !ok {
		t.Fatal("scale cache type mismatch")
	}
	if _, ok := newCatalogL1Cache(opts, catalogKindPersonality).(*personalitymodel.LocalCatalogCache); !ok {
		t.Fatal("personality cache type mismatch")
	}
}

func TestCatalogL1SingleflightEnabled(t *testing.T) {
	t.Parallel()

	opts := options.NewOptions()
	opts.ScaleCache.Singleflight = true
	opts.QuestionnaireCache.Singleflight = false

	if !catalogL1SingleflightEnabled(opts, catalogKindScale) {
		t.Fatal("scale singleflight should be enabled")
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
