package process

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FangcunMount/qs-server/pkg/app"
	"github.com/spf13/viper"

	apiconfig "github.com/FangcunMount/qs-server/internal/apiserver/config"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	apioptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
)

func TestDevelopmentConfigSelectsFinalTransactionOwners(t *testing.T) {
	v := viper.New()
	v.SetConfigFile(filepath.Join("..", "..", "..", "configs", "apiserver.dev.yaml"))
	if err := v.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	cfg := &apiconfig.Config{Options: apioptions.NewOptions()}
	if err := app.DecodeOptions(v, cfg.Options); err != nil {
		t.Fatal(err)
	}
	if err := validateFinalMessaging(cfg); err != nil {
		t.Fatal(err)
	}
	if err := cfg.MessagingOptions.RocketMQ.Validate(); err != nil {
		t.Fatal(err)
	}
}

func finalMessageConfig() *apiconfig.Config {
	cfg := &apiconfig.Config{Options: apioptions.NewOptions()}
	cfg.MessagingOptions.Enabled = true
	cfg.MessagingOptions.Provider = "rocketmq"
	cfg.MessagingOptions.RocketMQ = &genericoptions.RocketMQOptions{Transactions: map[string]genericoptions.TransactionOptions{"qs.mysql": {}, "qs.mongo": {}}}
	return cfg
}

func TestRetiredMessageModesFailBeforeDatabaseOrClientAcquisition(t *testing.T) {
	for _, kind := range []string{"nsq", "empty", "ordinary_rocketmq", "one_owner", "extra_owner", "old_profile", "disabled"} {
		t.Run(kind, func(t *testing.T) {
			cfg := finalMessageConfig()
			switch kind {
			case "nsq":
				cfg.MessagingOptions.Provider = "nsq"
			case "empty":
				cfg.MessagingOptions.Provider = ""
			case "ordinary_rocketmq":
				cfg.MessagingOptions.RocketMQ.Transactions = nil
			case "one_owner":
				delete(cfg.MessagingOptions.RocketMQ.Transactions, "qs.mongo")
			case "extra_owner":
				cfg.MessagingOptions.RocketMQ.Transactions["ai.mysql"] = genericoptions.TransactionOptions{}
			case "old_profile":
				cfg.Eventing.StandardOutbox.Mongo = true
			case "disabled":
				cfg.MessagingOptions.Enabled = false
			}
			deps := (&server{config: cfg}).buildResourceStageDeps()
			touched := false
			deps.database.initialize = func() error { touched = true; return errors.New("database must not open") }
			deps.database.close = func() error { touched = true; return nil }
			_, err := prepareResources(deps)
			if err == nil || !strings.Contains(err.Error(), "final RocketMQ transaction owners required") || touched {
				t.Fatalf("retired mode reached dependencies: error=%v touched=%t", err, touched)
			}
			_, err = configuredEventSubsystem(cfg)(eventsubsystem.Options{})
			if err == nil || !strings.Contains(err.Error(), "final RocketMQ transaction owners required") {
				t.Fatalf("retired mode reached subsystem construction: %v", err)
			}
		})
	}
}

func TestFinalMessagingSelectsOnlyNormalAndDualOwnerConstructors(t *testing.T) {
	cfg := finalMessageConfig()
	if err := validateFinalMessaging(cfg); err != nil {
		t.Fatal(err)
	}
	host := &server{config: cfg}
	if host.buildMQPublisherDeps().newNormalPublisher == nil || host.buildEventSubsystemResourceDeps().buildSDKSubscriberFactory == nil {
		t.Fatal("final production factories absent")
	}
	_, err := configuredEventSubsystem(cfg)(eventsubsystem.Options{})
	if err == nil || !strings.Contains(err.Error(), "full original QS databases") {
		t.Fatalf("final owner constructor was not selected: %v", err)
	}
}
