package options

import "testing"

func TestConcurrencyOptionsResolvedPools(t *testing.T) {
	t.Parallel()

	t.Run("query falls back to max concurrency", func(t *testing.T) {
		t.Parallel()
		opts := &ConcurrencyOptions{MaxConcurrency: 480}
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
			MaxQueryConcurrency:  400,
			MaxSubmitConcurrency: 96,
		}
		if got := opts.ResolvedQueryConcurrency(); got != 400 {
			t.Fatalf("ResolvedQueryConcurrency() = %d, want 400", got)
		}
		if got := opts.ResolvedSubmitConcurrency(); got != 96 {
			t.Fatalf("ResolvedSubmitConcurrency() = %d, want 96", got)
		}
	})
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
		MaxQueryConcurrency:  400,
		MaxSubmitConcurrency: 96,
	}); len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
}
