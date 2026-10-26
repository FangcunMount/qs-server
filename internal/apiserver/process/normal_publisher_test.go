package process

import (
	"errors"
	"testing"

	apiserverconfig "github.com/FangcunMount/qs-server/internal/apiserver/config"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	apiserveroptions "github.com/FangcunMount/qs-server/internal/apiserver/options"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	genericoptions "github.com/FangcunMount/qs-server/internal/pkg/options"
)

func TestMixedTransactionHostSelectsDeferredNormalFactoryAndNeverLegacyOutboxPublisher(t *testing.T) {
	cfg := &apiserverconfig.Config{Options: apiserveroptions.NewOptions()}
	cfg.MessagingOptions.Enabled = true
	cfg.MessagingOptions.Provider = "rocketmq"
	cfg.MessagingOptions.RocketMQ = &genericoptions.RocketMQOptions{Transactions: map[string]genericoptions.TransactionOptions{"qs.mysql": {}, "qs.mongo": {}}}
	deps := (&server{config: cfg}).buildMQPublisherDeps()
	if deps.newNormalPublisher == nil {
		t.Fatal("transaction host selected legacy publisher")
	}
}

func TestDeferredNormalPublisherUsesOriginalCatalogAndClosesOnLaterStartupFailure(t *testing.T) {
	catalog := eventcatalog.NewCatalog(nil)
	closed, calls := 0, 0
	wire := &fakeWirePublisher{onClose: func() { closed++ }}
	wanted := errors.New("original subsystem startup failed")
	_, err := prepareResources(resourceStageDeps{
		loadEventCatalog: func() (*eventcatalog.Catalog, error) { return catalog, nil },
		mqPublisher: mqPublisherStageDeps{enabled: true, provider: "rocketmq", newNormalPublisher: func(got *eventcatalog.Catalog) (wirePublisherResource, error) {
			calls++
			if got != catalog {
				t.Fatal("normal factory reloaded or replaced the original catalog")
			}
			return wire, nil
		}},
		eventSubsystem: eventSubsystemResourceDeps{newSubsystem: func(got eventsubsystem.Options) (*eventsubsystem.Subsystem, error) {
			if got.WirePublisher != wire || got.Catalog != catalog {
				t.Fatal("NORMAL binding lost at original subsystem root")
			}
			return nil, wanted
		}},
	})
	if !errors.Is(err, wanted) || calls != 1 || closed != 1 {
		t.Fatal("normal startup failure lost lifecycle ownership", err, calls, closed)
	}
}

func TestInvalidCatalogCannotStartDeferredNormalClient(t *testing.T) {
	calls := 0
	_, err := prepareResources(resourceStageDeps{
		loadEventCatalog: func() (*eventcatalog.Catalog, error) { return nil, errors.New("catalog unavailable") },
		mqPublisher:      mqPublisherStageDeps{enabled: true, provider: "rocketmq", newNormalPublisher: func(*eventcatalog.Catalog) (wirePublisherResource, error) { calls++; return &fakeWirePublisher{}, nil }},
	})
	if err == nil || calls != 0 {
		t.Fatal("invalid catalog started a NORMAL client", err, calls)
	}
}
