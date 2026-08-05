package options

import (
	"testing"

	"github.com/spf13/pflag"
)

func TestConcurrencyOptionsResolvedPools(t *testing.T) {
	t.Parallel()

	t.Run("query uses canonical pool", func(t *testing.T) {
		t.Parallel()
		opts := &ConcurrencyOptions{MaxQueryConcurrency: 480}
		if got := opts.ResolvedQueryConcurrency(); got != 480 {
			t.Fatalf("ResolvedQueryConcurrency() = %d, want 480", got)
		}
		if got := opts.ResolvedSubmitConcurrency(); got != 96 {
			t.Fatalf("ResolvedSubmitConcurrency() = %d, want 96", got)
		}
	})

	t.Run("explicit split", func(t *testing.T) {
		t.Parallel()
		opts := &ConcurrencyOptions{
			MaxQueryConcurrency:   280,
			MaxCatalogConcurrency: 512,
			MaxSubmitConcurrency:  96,
			MaxWaitMs:             4000,
			CatalogMaxWaitMs:      800,
		}
		if got := opts.ResolvedQueryConcurrency(); got != 280 {
			t.Fatalf("ResolvedQueryConcurrency() = %d, want 280", got)
		}
		if got := opts.ResolvedCatalogConcurrency(); got != 512 {
			t.Fatalf("ResolvedCatalogConcurrency() = %d, want 512", got)
		}
		if got := opts.ResolvedSubmitConcurrency(); got != 96 {
			t.Fatalf("ResolvedSubmitConcurrency() = %d, want 96", got)
		}
		if got := opts.ResolvedCatalogMaxWait().Milliseconds(); got != 800 {
			t.Fatalf("ResolvedCatalogMaxWait() = %dms, want 800ms", got)
		}
	})
}

func TestOptionsRejectRemovedMaxConcurrency(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"max-concurrency", "max_concurrency"} {
		key := key
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			err := NewOptions().ValidateRawSettings(map[string]any{
				"concurrency": map[string]any{key: 10},
			})
			if err == nil || err.Error() != "concurrency.max-concurrency has been removed; use concurrency.max-query-concurrency" {
				t.Fatalf("expected removed max-concurrency error, got %v", err)
			}
		})
	}
}

func TestConcurrencyFlagsExposeOnlySplitPools(t *testing.T) {
	t.Parallel()

	opts := NewOptions()
	if got := opts.Concurrency.ResolvedQueryConcurrency(); got != 10 {
		t.Fatalf("default query concurrency = %d, want 10", got)
	}

	fs := pflag.NewFlagSet("concurrency", pflag.ContinueOnError)
	opts.Concurrency.AddFlags(fs)
	if fs.Lookup("concurrency.max-concurrency") != nil {
		t.Fatal("removed concurrency.max-concurrency flag is still registered")
	}
	if fs.Lookup("concurrency.max-query-concurrency") == nil {
		t.Fatal("canonical concurrency.max-query-concurrency flag is missing")
	}
}

func TestValidateCollectionConcurrencyRequiresPools(t *testing.T) {
	t.Parallel()

	if errs := validateCollectionConcurrency(nil); len(errs) == 0 {
		t.Fatal("expected error for nil concurrency")
	}
	if errs := validateCollectionConcurrency(&ConcurrencyOptions{}); len(errs) == 0 {
		t.Fatal("expected error for empty concurrency")
	}
	if errs := validateCollectionConcurrency(&ConcurrencyOptions{
		MaxQueryConcurrency:   280,
		MaxCatalogConcurrency: 512,
		MaxSubmitConcurrency:  96,
	}); len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
}
