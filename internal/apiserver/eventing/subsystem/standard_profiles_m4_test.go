//go:build reliable_messaging_m4

package subsystem

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/FangcunMount/component-base/pkg/event"
	"github.com/FangcunMount/component-base/pkg/messaging"
	appEventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	outboxport "github.com/FangcunMount/qs-server/internal/apiserver/port/outbox"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventobservability "github.com/FangcunMount/qs-server/internal/pkg/eventing/observe"
	"github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/reliable-messaging/relay"
	_ "github.com/go-sql-driver/mysql"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type candidateStager struct{}

func (candidateStager) Stage(context.Context, ...event.DomainEvent) error { return nil }

type candidatePostCommit struct{}

func (candidatePostCommit) AfterCommit(context.Context, []event.DomainEvent, time.Time) {}

type candidateStatusReader struct{}

func (candidateStatusReader) OutboxStatusSnapshot(_ context.Context, now time.Time) (outboxport.StatusSnapshot, error) {
	return outboxport.StatusSnapshot{Store: "standard-mongo", GeneratedAt: now,
		Buckets: []outboxport.StatusBucket{{Status: "pending", Count: 2}}}, nil
}

type candidateTypeStatusReader struct{ candidateStatusReader }

func (candidateTypeStatusReader) OutboxStatusByEventType(context.Context, time.Time) ([]outboxport.EventTypeStatusBucket, error) {
	return nil, nil
}

type candidateStatusObserver struct {
	eventobservability.NopObserver
	statuses     chan eventobservability.OutboxStatusEvent
	typeStatuses chan eventobservability.OutboxEventTypeStatusEvent
}

func (o candidateStatusObserver) ObserveOutboxEventTypeStatus(_ context.Context, evt eventobservability.OutboxEventTypeStatusEvent) {
	select {
	case o.typeStatuses <- evt:
	default:
	}
}

func (o candidateStatusObserver) ObserveOutboxStatus(_ context.Context, evt eventobservability.OutboxStatusEvent) {
	select {
	case o.statuses <- evt:
	default:
	}
}

type candidateRunner func(context.Context) error

func (run candidateRunner) Run(ctx context.Context) error { return run(ctx) }

func TestStandardProfileReplacesWholeLegacyRuntimeBeforeStart(t *testing.T) {
	// An unconnected client proves selection does not construct the legacy
	// Mongo store, whose constructor creates indexes over the old collection.
	client, err := mongo.NewClient(options.Client().ApplyURI("mongodb://127.0.0.1:1"))
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var calls []string
	record := func(call string) {
		mu.Lock()
		calls = append(calls, call)
		mu.Unlock()
	}
	started := make(chan struct{})
	statusEvents := make(chan eventobservability.OutboxStatusEvent, 1)
	typeStatusEvents := make(chan eventobservability.OutboxEventTypeStatusEvent, 16)
	stager := candidateStager{}
	postCommit := candidatePostCommit{}
	supervisor, err := standardoutbox.NewRelaySupervisor(standardoutbox.SupervisorOptions{
		Name: "mongo-domain-events", InitialBackoff: time.Millisecond, MaxBackoff: time.Second,
		NewRelay: func(observe relay.Observer) (standardoutbox.RelayRunner, error) {
			return candidateRunner(func(ctx context.Context) error {
				record("run.start")
				observe(relay.Event{Kind: "scan_succeeded"})
				close(started)
				<-ctx.Done()
				record("run.stop")
				return nil
			}), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewWithStandardProfiles(Options{
		Catalog: loadCatalog(t), MongoDB: client.Database("candidate"),
		PublisherMode: eventruntime.PublishModeMQ, MQPublisher: fakePublisher{},
		Observer:  candidateStatusObserver{statuses: statusEvents, typeStatuses: typeStatusEvents},
		Consumers: map[string]ConsumerOptions{hotRankConsumerID: {Enabled: false}},
	}, map[eventcatalog.OutboxProfile]StandardProfile{
		eventcatalog.OutboxProfileMongoDomain: {
			Binding:    appEventing.ProfileBinding{Stager: stager, PostCommit: postCommit},
			Supervisor: supervisor,
			Drain:      func(context.Context) error { record("drain"); return nil }, DrainTimeout: time.Second,
			Status: appEventing.NamedOutboxStatusReader{Name: "mongo-domain-events", Reader: candidateTypeStatusReader{}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	profile := s.Profile(eventcatalog.OutboxProfileMongoDomain)
	if profile.Stager != stager || profile.PostCommit != postCommit {
		t.Fatal("standard profile did not replace the legacy writer and post-commit path")
	}
	if runtime := s.profiles[eventcatalog.OutboxProfileMongoDomain]; runtime.relay != nil || runtime.immediate != nil || runtime.reconciler != nil || runtime.readyIndex != nil {
		t.Fatalf("legacy executor survived profile replacement: %+v", runtime)
	}
	if err := s.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("candidate runner did not start")
	}
	select {
	case observed := <-statusEvents:
		if observed.Store != "standard-mongo" || observed.Status != "pending" || observed.Count != 2 {
			t.Fatalf("SDK profile backlog metric = %+v", observed)
		}
	case <-time.After(time.Second):
		t.Fatal("SDK profile did not report its backlog after starting")
	}
	select {
	case observed := <-typeStatusEvents:
		if observed.Store != "standard-mongo" || observed.EventType == "" || observed.Status != "pending" || observed.Count != 0 {
			t.Fatalf("SDK profile did not seed an empty event-type bucket: %+v", observed)
		}
	case <-time.After(time.Second):
		t.Fatal("SDK profile did not seed event-type backlog labels after starting")
	}
	status, err := s.StatusService().GetStatus(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, profile := range status.Profiles {
		if profile.Name == eventcatalog.OutboxProfileMongoDomain {
			if !profile.Running || !profile.RelayEnabled || profile.ImmediateEnabled || profile.ReconcilerEnabled ||
				profile.RelayKind != "sdk" || profile.ScanHealthy == nil || !*profile.ScanHealthy {
				t.Fatalf("candidate status is inaccurate: %+v", profile)
			}
		}
	}
	if len(status.Outboxes) != 1 || status.Outboxes[0].Store != "standard-mongo" || status.Outboxes[0].Degraded {
		t.Fatalf("candidate status still reads legacy outbox: %+v", status.Outboxes)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	got := append([]string(nil), calls...)
	mu.Unlock()
	if !reflect.DeepEqual(got, []string{"run.start", "run.stop", "drain"}) {
		t.Fatalf("candidate close order = %v", got)
	}
	stopped := s.runtimeStatusSnapshot().Profiles[eventcatalog.OutboxProfileMongoDomain]
	if stopped.Running || stopped.ScanHealthy == nil || *stopped.ScanHealthy {
		t.Fatalf("stopped candidate reported healthy: %+v", stopped)
	}
}

func TestMongoOnlyStandardProfileKeepsLegacyAssessmentAndHotRankConsumer(t *testing.T) {
	client, err := mongo.NewClient(options.Client().ApplyURI("mongodb://127.0.0.1:1"))
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := sql.Open("mysql", "root@tcp(127.0.0.1:1)/candidate?parseTime=true")
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	mysqlDB, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}),
		&gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := standardoutbox.NewRelaySupervisor(standardoutbox.SupervisorOptions{
		Name: "mongo-domain-events", InitialBackoff: time.Millisecond, MaxBackoff: time.Second,
		NewRelay: func(relay.Observer) (standardoutbox.RelayRunner, error) {
			return candidateRunner(func(ctx context.Context) error { <-ctx.Done(); return nil }), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	subscriber := &fakeSubscriber{}
	s, err := NewWithStandardProfiles(Options{
		Catalog: loadCatalog(t), MongoDB: client.Database("candidate"), MySQLDB: mysqlDB,
		PublisherMode: eventruntime.PublishModeMQ, MQPublisher: fakePublisher{},
		SubscriberFactory: func() (messaging.Subscriber, error) { return subscriber, nil },
		Consumers: map[string]ConsumerOptions{hotRankConsumerID: {
			Enabled: true, Channel: "qs-apiserver-modelcatalog-hot-rank-v1",
		}},
	}, map[eventcatalog.OutboxProfile]StandardProfile{
		eventcatalog.OutboxProfileMongoDomain: {
			Binding:    appEventing.ProfileBinding{Stager: candidateStager{}, PostCommit: candidatePostCommit{}},
			Supervisor: supervisor, Drain: func(context.Context) error { return nil }, DrainTimeout: time.Second,
			Status: appEventing.NamedOutboxStatusReader{Name: "mongo-domain-events", Reader: candidateStatusReader{}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	mongoProfile := s.profiles[eventcatalog.OutboxProfileMongoDomain]
	if mongoProfile.run == nil || mongoProfile.relay != nil || mongoProfile.immediate != nil || mongoProfile.reconciler != nil {
		t.Fatalf("Mongo replacement retained a legacy runner: %+v", mongoProfile)
	}
	assessmentProfile := s.profiles[eventcatalog.OutboxProfileAssessmentMySQL]
	if assessmentProfile.run != nil || assessmentProfile.relay == nil || assessmentProfile.immediate == nil || assessmentProfile.reconciler == nil {
		t.Fatalf("unselected MySQL profile was replaced: %+v", assessmentProfile)
	}
	if err := s.RegisterConsumer(hotRankConsumerID, func(context.Context, string, []byte) error { return nil }); err != nil {
		t.Fatal(err)
	}
	// This test needs no live MySQL: the existing relay's presence is asserted
	// above, while the start below verifies the independent subscriber wiring.
	assessmentProfile.relay = nil
	assessmentProfile.reconciler = nil
	if err := s.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if subscriber.topic != "qs.evaluation.lifecycle" || subscriber.channel != "qs-apiserver-modelcatalog-hot-rank-v1" || subscriber.handler == nil {
		t.Fatalf("hot-rank subscription = topic %q channel %q handler %v", subscriber.topic, subscriber.channel, subscriber.handler != nil)
	}
}

func TestStandardProfileFailsClosedWhenIncomplete(t *testing.T) {
	client, err := mongo.NewClient(options.Client().ApplyURI("mongodb://127.0.0.1:1"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewWithStandardProfiles(Options{
		Catalog: loadCatalog(t), MongoDB: client.Database("candidate"),
		PublisherMode: eventruntime.PublishModeMQ, MQPublisher: fakePublisher{},
	}, map[eventcatalog.OutboxProfile]StandardProfile{
		eventcatalog.OutboxProfileMongoDomain: {
			Binding: appEventing.ProfileBinding{Stager: candidateStager{}, PostCommit: candidatePostCommit{}},
			Status:  appEventing.NamedOutboxStatusReader{Name: "mongo-domain-events", Reader: candidateStatusReader{}},
		},
	})
	if err == nil {
		t.Fatal("incomplete standard profile was accepted")
	}
}

func TestStandardProfileDrainHasBoundedContext(t *testing.T) {
	s := &Subsystem{
		profiles: map[eventcatalog.OutboxProfile]*profileRuntime{
			eventcatalog.OutboxProfileMongoDomain: {
				name: "mongo-domain-events", drainTimeout: 10 * time.Millisecond,
				drain: func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() },
			},
		},
		consumers: map[string]*consumerRuntime{}, closeDone: make(chan struct{}),
	}
	if err := s.Close(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close() error = %v, want bounded drain deadline", err)
	}
}
