package process

import (
	"strings"
	"testing"
	"time"

	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
	workerconfig "github.com/FangcunMount/qs-server/internal/worker/config"
	workeroptions "github.com/FangcunMount/qs-server/internal/worker/options"
)

func TestRetiredWorkerModeCannotEnterRuntime(t *testing.T) {
	for _, kind := range []string{"nsq", "empty", "ordinary_writer", "owner_on_worker"} {
		t.Run(kind, func(t *testing.T) {
			o := workeroptions.NewOptions()
			off := false
			o.Messaging.Provider = "rocketmq"
			o.Messaging.RocketMQ = &genericoptions.RocketMQOptions{PublishEnabled: &off}
			switch kind {
			case "nsq":
				o.Messaging.Provider = "nsq"
			case "empty":
				o.Messaging.Provider = ""
			case "ordinary_writer":
				o.Messaging.RocketMQ.PublishEnabled = nil
			case "owner_on_worker":
				o.Messaging.RocketMQ.Transactions = map[string]genericoptions.TransactionOptions{"qs.mysql": {}}
			}
			s := &server{config: &workerconfig.Config{Options: o}}
			_, err := s.initializeRuntime(resourceOutput{}, containerOutput{})
			if err == nil || !strings.Contains(err.Error(), "final consume-only RocketMQ worker required") {
				t.Fatalf("retired worker accepted: %v", err)
			}
		})
	}
}

func TestFinalWorkerKeepsSharedConsumptionAndProcessingBudget(t *testing.T) {
	o := workeroptions.NewOptions()
	off := false
	o.Worker.Concurrency = 48
	o.Messaging.NSQMessageTimeout = 8*time.Minute + 30*time.Second
	o.Messaging.Provider = "rocketmq"
	o.Messaging.RocketMQ = &genericoptions.RocketMQOptions{PublishEnabled: &off}
	s := &server{config: &workerconfig.Config{Options: o}}
	if err := s.validateFinalMessaging(); err != nil {
		t.Fatal(err)
	}
	if s.workerMaxInFlight() != 48 || s.loadedSubscriberConfig().NSQMessageTimeout != o.Messaging.NSQMessageTimeout {
		t.Fatal("retirement changed the current consumption/deadline budget")
	}
}
