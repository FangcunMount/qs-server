//go:build integration && reliable_messaging_m4 && reliable_messaging_m5

package runtimeclosure

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/messaging"
	appeventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	mongostandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/standardoutbox"
	mysqlstandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/standardoutbox"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/worker/handlers"
	"github.com/FangcunMount/reliable-messaging/relay"
	sdkmongo "github.com/FangcunMount/reliable-messaging/storage/mongo"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	sdknsq "github.com/FangcunMount/reliable-messaging/transport/nsq"
	"github.com/nsqio/go-nsq"
)

type standardClosureResult struct {
	message   *messaging.Message
	eventType string
	brokerID  nsq.MessageID
	attempts  uint16
	handledAt time.Time
	err       error
}

type standardClosureDelivery struct {
	mu                      sync.RWMutex
	handlers                map[string]handlers.HandlerFunc
	results                 chan standardClosureResult
	pending                 map[string][]standardClosureResult
	standardMongo           bool
	delayFirstFailure       bool
	firstEvaluationBrokerID *nsq.MessageID
	lastEvaluationAttempts  uint16
	firstOutcomeBrokerID    *nsq.MessageID
	lastOutcomeAttempts     uint16
	firstFailureBrokerID    *nsq.MessageID
	lastFailureAttempts     uint16
	lastFailureHandledAt    time.Time
}

func (d *standardClosureDelivery) SetHandlers(registered map[string]handlers.HandlerFunc) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers = registered
}

func (d *standardClosureDelivery) Covers(eventType string) bool {
	if d.standardMongo {
		return eventType == eventcatalog.AnswerSheetSubmitted || eventType == eventcatalog.InterpretationReportGenerated ||
			eventType == eventcatalog.EvaluationRequested || eventType == eventcatalog.EvaluationOutcomeCommitted ||
			(d.delayFirstFailure && (eventType == eventcatalog.EvaluationFailed || eventType == eventcatalog.EvaluationRetryRequested))
	}
	return eventType == eventcatalog.EvaluationRequested || eventType == eventcatalog.EvaluationOutcomeCommitted
}

func (d *standardClosureDelivery) UsesStandardMongo() bool { return d.standardMongo }

func (d *standardClosureDelivery) Wait(t *testing.T, eventType string) (*messaging.Message, error) {
	t.Helper()
	for {
		var got standardClosureResult
		if waiting := d.pending[eventType]; len(waiting) > 0 {
			got, d.pending[eventType] = waiting[0], waiting[1:]
		} else {
			select {
			case got = <-d.results:
			case <-t.Context().Done():
				t.Fatalf("standard NSQ event %q did not arrive: %v", eventType, t.Context().Err())
				return nil, t.Context().Err()
			}
			if got.eventType != eventType {
				d.pending[got.eventType] = append(d.pending[got.eventType], got)
				continue
			}
		}
		if got.message == nil {
			t.Fatalf("standard NSQ event=%q has no decoded message", got.eventType)
		}
		if eventType == eventcatalog.EvaluationRequested {
			if d.firstEvaluationBrokerID == nil {
				if got.attempts != 1 {
					t.Fatalf("first Evaluation broker attempt=%d, want 1", got.attempts)
				}
				brokerID := got.brokerID
				d.firstEvaluationBrokerID = &brokerID
			} else if got.brokerID != *d.firstEvaluationBrokerID || got.attempts <= d.lastEvaluationAttempts {
				t.Fatalf("Evaluation was republished instead of NSQ redelivery: first broker ID=%s, current=%s, attempts=%d after %d",
					*d.firstEvaluationBrokerID, got.brokerID, got.attempts, d.lastEvaluationAttempts)
			}
			d.lastEvaluationAttempts = got.attempts
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
		if eventType == eventcatalog.EvaluationFailed && d.delayFirstFailure {
			if d.firstFailureBrokerID == nil {
				if got.attempts != 1 {
					t.Fatalf("first Failure broker attempt=%d, want 1", got.attempts)
				}
				brokerID := got.brokerID
				d.firstFailureBrokerID = &brokerID
			} else if got.brokerID != *d.firstFailureBrokerID || got.attempts <= d.lastFailureAttempts {
				t.Fatalf("Failure was republished instead of NSQ redelivery: first broker ID=%s, current=%s, attempts=%d after %d", *d.firstFailureBrokerID, got.brokerID, got.attempts, d.lastFailureAttempts)
			}
			d.lastFailureAttempts = got.attempts
			d.lastFailureHandledAt = got.handledAt
		}
		return got.message, got.err
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

func TestM5DualStandardProfilesCurrentBusinessClosure(t *testing.T) {
	if os.Getenv("RM_QS_M5_NSQ_TCP") != "nsqd:4150" {
		t.Fatal("disposable nsqd:4150 required")
	}
	runCurrentRuntimeClosure(t, func(t *testing.T, opts eventsubsystem.Options, db *sql.DB) (*eventsubsystem.Subsystem, runtimeClosureDelivery, error) {
		return newM5StandardEventSubsystem(t, opts, db, true, false)
	})
}

func newM5StandardMySQLEventSubsystem(t *testing.T, opts eventsubsystem.Options, sqlDB *sql.DB) (*eventsubsystem.Subsystem, runtimeClosureDelivery, error) {
	return newM5StandardEventSubsystem(t, opts, sqlDB, false, false)
}

func newM5StandardEventSubsystem(t *testing.T, opts eventsubsystem.Options, sqlDB *sql.DB, standardMongo, delayFirstFailure bool) (*eventsubsystem.Subsystem, runtimeClosureDelivery, error) {
	const topic = "qs.evaluation.lifecycle"
	address := os.Getenv("RM_QS_M5_NSQ_TCP")
	config := nsq.NewConfig()
	config.HeartbeatInterval, config.MsgTimeout = time.Second, 5*time.Second
	config.ReadTimeout, config.WriteTimeout = 3*time.Second, time.Second
	config.DefaultRequeueDelay = 200 * time.Millisecond
	channel := "rm-m5-current-business-closure"
	if standardMongo {
		channel = "rm-m5-dual-business-closure"
	}
	consumer, err := nsq.NewConsumer(topic, channel, config)
	if err != nil {
		return nil, nil, err
	}
	consumer.SetLogger(nil, nsq.LogLevelError)
	delivery := &standardClosureDelivery{results: make(chan standardClosureResult, 16), pending: make(map[string][]standardClosureResult), standardMongo: standardMongo, delayFirstFailure: delayFirstFailure}
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
		if handleErr == nil && delayFirstFailure && eventType == eventcatalog.EvaluationFailed && raw.Attempts == 1 {
			// Keep the *same* broker message for a late physical redelivery.
			// The test asserts that the new report is durable before attempt 2.
			raw.DisableAutoResponse()
			raw.RequeueWithoutBackoff(60 * time.Second)
		}
		if handleErr == nil && eventType == eventcatalog.EvaluationRequested && raw.Attempts == 1 {
			handleErr = errors.New("controlled lost Evaluation consumer ACK")
		}
		select {
		case delivery.results <- standardClosureResult{message: decoded, eventType: eventType, brokerID: raw.ID, attempts: raw.Attempts, handledAt: time.Now(), err: handleErr}:
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
	maxInFlight := 1
	if standardMongo {
		maxInFlight = 2
	}
	publisher, err := sdknsq.New(producer, map[string]string{topic: topic}, maxInFlight)
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
	replacements := map[eventcatalog.OutboxProfile]eventsubsystem.StandardProfile{
		eventcatalog.OutboxProfileAssessmentMySQL: {
			Binding:    appeventing.ProfileBinding{Stager: stager, PostCommit: wake},
			Supervisor: supervisor, Drain: drain, DrainTimeout: 5 * time.Second,
			Status: appeventing.NamedOutboxStatusReader{Name: "assessment-mysql-outbox", Reader: status},
		},
	}
	if standardMongo {
		collection := opts.MongoDB.Collection("rm_outbox")
		if _, err := collection.Indexes().CreateMany(t.Context(), sdkmongo.Indexes()); err != nil {
			return nil, nil, err
		}
		mongoStore, err := sdkmongo.New(collection)
		if err != nil {
			return nil, nil, err
		}
		mongoStager, err := mongostandard.NewStager(collection, opts.Catalog, eventruntime.SourceAPIServer)
		if err != nil {
			return nil, nil, err
		}
		mongoStatus, err := mongostandard.NewStatusReader(collection)
		if err != nil {
			return nil, nil, err
		}
		mongoWake := standardoutbox.NewPostCommitWake()
		mongoSupervisor, err := standardoutbox.NewRelaySupervisor(standardoutbox.SupervisorOptions{
			Name: "mongo-domain-events", InitialBackoff: 100 * time.Millisecond, MaxBackoff: time.Second,
			NewRelay: func(observe relay.Observer) (standardoutbox.RelayRunner, error) {
				return relay.New(mongoStore, publisher, relay.Config{
					Concurrency: 1, PollInterval: 50 * time.Millisecond, Lease: 5 * time.Second,
					PublishTimeout: 2 * time.Second, WriteTimeout: time.Second,
					Wake: mongoWake.Wake(), Retry: standardoutbox.SDKRetryPolicy(), Observe: observe,
				})
			},
		})
		if err != nil {
			return nil, nil, err
		}
		replacements[eventcatalog.OutboxProfileMongoDomain] = eventsubsystem.StandardProfile{
			Binding:    appeventing.ProfileBinding{Stager: mongoStager, PostCommit: mongoWake},
			Supervisor: mongoSupervisor, Drain: drain, DrainTimeout: 5 * time.Second,
			Status: appeventing.NamedOutboxStatusReader{Name: "mongo-domain-events", Reader: mongoStatus},
		}
	}
	subsystem, err := eventsubsystem.NewWithStandardProfiles(opts, replacements)
	return subsystem, delivery, err
}
