package options

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestProductionConfigEnablesCommittedAuthzVersionGuard(t *testing.T) {
	config := viper.New()
	config.SetConfigFile(filepath.Join("..", "..", "..", "configs", "apiserver.prod.yaml"))
	if err := config.ReadInConfig(); err != nil {
		t.Fatal(err)
	}

	loaded := NewOptions()
	if err := config.Unmarshal(loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.IAMOptions == nil || loaded.IAMOptions.AuthzVersionGuard == nil {
		t.Fatal("production IAM committed-version guard was not loaded")
	}
	guard := loaded.IAMOptions.AuthzVersionGuard
	if !guard.Enabled || guard.MaxAge != 10*time.Second || guard.PollInterval != 5*time.Second || guard.ReadTimeout != 2*time.Second {
		t.Fatalf("production IAM committed-version guard = %+v", guard)
	}
	if loaded.IAMOptions.AuthzSync == nil || loaded.IAMOptions.AuthzSync.EphemeralNSQ {
		t.Fatal("ephemeral NSQ must remain disabled for the guard rollout")
	}
	if errs := guard.Validate(); len(errs) != 0 {
		t.Fatalf("production IAM guard options invalid: %v", errs)
	}
}
