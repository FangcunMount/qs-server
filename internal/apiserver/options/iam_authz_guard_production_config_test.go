package options

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestFinalProductionConfigPairsRocketMQWithCommittedVersionGuard(t *testing.T) {
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
	sync := loaded.IAMOptions.AuthzSync
	if sync == nil || !sync.Enabled || sync.Provider != "rocketmq" || sync.EphemeralNSQ {
		t.Fatalf("final production authorization subscription differs: %+v", sync)
	}
	if errs := guard.Validate(); len(errs) != 0 {
		t.Fatalf("production IAM guard options invalid: %v", errs)
	}
	if errs := sync.Validate(); len(errs) == 0 {
		t.Fatal("unbound production endpoint must fail closed")
	}
	sync.RocketMQ.Endpoint = "broker.invalid:8081"
	if errs := sync.Validate(); len(errs) != 0 {
		t.Fatalf("production IAM authorization sync options invalid: %v", errs)
	}
}
