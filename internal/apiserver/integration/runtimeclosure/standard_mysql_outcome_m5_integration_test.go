//go:build integration && reliable_messaging_m4 && reliable_messaging_m5

package runtimeclosure

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/messaging"
	appeventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	mysqlstandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/standardoutbox"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	"github.com/FangcunMount/reliable-messaging/relay"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	sdknsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
	"github.com/nsqio/go-nsq"
)

type standardClosureResult struct {
	message   *messaging.Message
	eventType string
	brokerID  nsq.MessageID
	attempts  uint16
	err       error
}

type standardClosureDelivery struct {
	mu                   sync.RWMutex
	handlers             map[string]handlers.HandlerFunc
	results              chan standardClosureResult
	firstOutcomeBrokerID *nsq.MessageID
	lastOutcomeAttempts  uint16
}

func (d *standardClosureDelivery) SetHandlers(registered map[string]handlers.HandlerFunc) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers = registered
}

func (d *standardClosureDelivery) Wait(t *testing.T, eventType string) (*messaging.Message, error) {
	t.Helper()
	select {
	case got := <-d.results:
		if got.eventType != eventType || got.message == nil {
			t.Fatalf("standard MySQL NSQ event=%q message=%v, want %q", got.eventType, got.message, eventType)
		}
		if eventType == eventcatalog.EvaluationOutcomeCommitted {
			if d.firstOutcomeBrokerID == nil {
				if got.attempts != 1 {
					t.Fatalf("first Outcome broker attempt=%d, want 1", got.attempts)
				}
				brokerID := got.brokerID
				d.firstOutcomeBrokerID = &brokerID
			} else if got.brokerID != *d.firstOutcomeBrokerID || got.attempts <= d.lastOutcomeAttempts {
				t.Fatalf("Outcome was republished instead of NSQ redelivery: first broker ID=%s, current=%s, attempts=%d after %d", *d.firstOutcomeBrokerID, got.brokerID, got.attempts, d.lastOutcomeAttempts)
			}
			d.lastOutcomeAttempts = got.attempts
		}
		return got.message, got.err
	case <-t.Context().Done():
		t.Fatalf("standard MySQL NSQ event %q did not arrive: %v", eventType, t.Context().Err())
		return nil, t.Context().Err()
	}
}

// The same current-time business closure as the legacy baseline now selects
// the standard MySQL profile. The report response is hidden once after a real
// gRPC/Mongo commit; NSQ redelivers the original Outcome to the original
// Worker, which must reuse the one durable report and generated-report intent.
func TestM5StandardMySQLOutcomeToReportAcrossNSQ(t *testing.T) {
	if os.Getenv("RM_QS_M5_NSQ_TCP") != "nsqd:4150" {
		t.Fatal("disposable nsqd:4150 required")
	}
	runCurrentRuntimeClosure(t, newM5StandardMySQLEventSubsystem)
}

func newM5StandardMySQLEventSubsystem(t *testing.T, opts eventsubsystem.Options, sqlDB *sql.DB) (*eventsubsystem.Subsystem, runtimeClosureDelivery, error) {
	const topic = "qs.evaluation.lifecycle"
	address := os.Getenv("RM_QS_M5_NSQ_TCP")
	config := nsq.NewConfig()
	config.HeartbeatInterval, config.MsgTimeout = time.Second, 5*time.Second
	config.ReadTimeout, config.WriteTimeout = 3*time.Second, time.Second
	config.DefaultRequeueDelay = 200 * time.Millisecond
	consumer, err := nsq.NewConsumer(topic, "rm-m5-current-business-closure", config)
	if err != nil {
		return nil, nil, err
	}
	consumer.SetLogger(nil, nsq.LogLevelError)
	delivery := &standardClosureDelivery{results: make(chan standardClosureResult, 4)}
	consumer.AddHandler(nsq.HandlerFunc(func(raw *nsq.Message) error {
		decoded, recognized, decodeErr := messaging.DecodeMessagePayload(raw.Body)
		if decodeErr != nil || !recognized {
			if decodeErr == nil {
				decodeErr = fmt.Errorf("standard MySQL message has no original QS envelope")
			}
			return decodeErr
		}
		eventType := decoded.Metadata["event_type"]
		delivery.mu.RLock()
		handler := delivery.handlers[eventType]
		delivery.mu.RUnlock()
		if handler == nil {
			return fmt.Errorf("no current Worker handler for %s", eventType)
		}
		handleErr := handler(t.Context(), eventType, decoded.Payload)
		select {
		case delivery.results <- standardClosureResult{message: decoded, eventType: eventType, brokerID: raw.ID, attempts: raw.Attempts, err: handleErr}:
		case <-t.Context().Done():
		}
		return handleErr
	}))
	if err := consumer.ConnectToNSQD(address); err != nil {
		consumer.Stop()
		return nil, nil, err
	}
	t.Cleanup(func() {
		consumer.Stop()
		select {
		case <-consumer.StopChan:
		case <-time.After(5 * time.Second):
			t.Error("standard MySQL closure consumer did not stop")
		}
	})
	producer, err := nsq.NewProducer(address, config)
	if err != nil {
		return nil, nil, err
	}
	producer.SetLogger(nil, nsq.LogLevelError)
	if err := producer.Ping(); err != nil {
		producer.Stop()
		return nil, nil, err
	}
	publisher, err := sdknsq.New(producer, map[string]string{topic: topic}, 1)
	if err != nil {
		producer.Stop()
		return nil, nil, err
	}
	var drainOnce sync.Once
	var drainErr error
	drain := func(ctx context.Context) error {
		drainOnce.Do(func() {
			drainErr = publisher.Drain(ctx)
			producer.Stop()
		})
		return drainErr
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := drain(ctx); err != nil {
			t.Errorf("drain standard MySQL publisher: %v", err)
		}
	})
	store, err := sdkmysql.New(sqlDB)
	if err != nil {
		return nil, nil, err
	}
	stager, err := mysqlstandard.NewStager(opts.Catalog, eventruntime.SourceAPIServer)
	if err != nil {
		return nil, nil, err
	}
	status, err := mysqlstandard.NewStatusReader(sqlDB)
	if err != nil {
		return nil, nil, err
	}
	wake := standardoutbox.NewPostCommitWake()
	supervisor, err := standardoutbox.NewRelaySupervisor(standardoutbox.SupervisorOptions{
		Name: "assessment-mysql-outbox", InitialBackoff: 100 * time.Millisecond, MaxBackoff: time.Second,
		NewRelay: func(observe relay.Observer) (standardoutbox.RelayRunner, error) {
			return relay.New(store, publisher, relay.Config{
				Concurrency: 1, PollInterval: 50 * time.Millisecond, Lease: 5 * time.Second,
				PublishTimeout: 2 * time.Second, WriteTimeout: time.Second,
				Wake: wake.Wake(), Retry: standardoutbox.SDKRetryPolicy(), Observe: observe,
			})
		},
	})
	if err != nil {
		return nil, nil, err
	}
	subsystem, err := eventsubsystem.NewWithStandardProfiles(opts, map[eventcatalog.OutboxProfile]eventsubsystem.StandardProfile{
		eventcatalog.OutboxProfileAssessmentMySQL: {
			Binding:    appeventing.ProfileBinding{Stager: stager, PostCommit: wake},
			Supervisor: supervisor, Drain: drain, DrainTimeout: 5 * time.Second,
			Status: appeventing.NamedOutboxStatusReader{Name: "assessment-mysql-outbox", Reader: status},
		},
	})
	return subsystem, delivery, err
}
