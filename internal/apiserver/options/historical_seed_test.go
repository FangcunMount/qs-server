package options

import "testing"

func TestHistoricalSeedEnabledRequiresInjectedSecret(t *testing.T) {
	t.Setenv(DefaultHistoricalSeedSecretEnv, "")
	opts := NewHistoricalSeedOptions()
	opts.Enabled = true
	opts.AllowedOrgIDs = []int64{1}

	if _, err := opts.Verifier(); err == nil {
		t.Fatal("Verifier() accepted enabled historical seed without its environment secret")
	}
}
