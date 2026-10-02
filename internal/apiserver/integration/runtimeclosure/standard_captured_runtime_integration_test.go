//go:build integration

package runtimeclosure

import (
	"context"
	"database/sql"
	"testing"
	"time"

	appeventing "github.com/FangcunMount/qs-server/internal/apiserver/application/eventing"
	"github.com/FangcunMount/qs-server/internal/apiserver/eventing/standardoutbox"
	eventsubsystem "github.com/FangcunMount/qs-server/internal/apiserver/eventing/subsystem"
	mongostandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/standardoutbox"
	mysqlstandard "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/standardoutbox"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/reliable-messaging/message"
	"github.com/FangcunMount/reliable-messaging/outbox"
	"github.com/FangcunMount/reliable-messaging/relay"
	sdkmongo "github.com/FangcunMount/reliable-messaging/storage/mongo"
	sdkmysql "github.com/FangcunMount/reliable-messaging/storage/mysql"
	sdktransport "github.com/FangcunMount/reliable-messaging/transport"
)

// The ordinary CI closure retains its captured transport boundary. Actual
// storage, original transactions and SDK Relay now match the current runtime;
// the separate real-NSQ closures prove physical delivery and FIN behavior.
type capturedSDKPublisher struct{ wire eventruntime.WirePublisher }

func (p capturedSDKPublisher) Publish(ctx context.Context, msg message.Message) sdktransport.Result {
	in := msg.Input()
	if err := p.wire.PublishWire(ctx, in.Destination, in.Payload); err != nil {
		return sdktransport.Result{Outcome: sdktransport.Unknown}
	}
	return sdktransport.Result{Outcome: sdktransport.Confirmed}
}

func newCapturedStandardEventSubsystem(t *testing.T, opts eventsubsystem.Options, db *sql.DB) (*eventsubsystem.Subsystem, runtimeClosureDelivery, error) {
	publisher := capturedSDKPublisher{wire: opts.WirePublisher}
	profiles := make(map[eventcatalog.OutboxProfile]eventsubsystem.StandardProfile)
	newProfile := func(name string, store outbox.Store, stager appeventing.EventStager, status appeventing.NamedOutboxStatusReader) (eventsubsystem.StandardProfile, error) {
		wake := standardoutbox.NewPostCommitWake()
		supervisor, err := standardoutbox.NewRelaySupervisor(standardoutbox.SupervisorOptions{
			Name: name, InitialBackoff: 50 * time.Millisecond, MaxBackoff: time.Second,
			NewRelay: func(observe relay.Observer) (standardoutbox.RelayRunner, error) {
				return relay.New(store, publisher, relay.Config{
					Concurrency: 1, PollInterval: 50 * time.Millisecond, Lease: 5 * time.Second, PublishTimeout: time.Second, WriteTimeout: time.Second,
					Wake: wake.Wake(), Retry: standardoutbox.SDKRetryPolicy(), Observe: observe,
				})
			},
		})
		if err != nil {
			return eventsubsystem.StandardProfile{}, err
		}
		return eventsubsystem.StandardProfile{Binding: appeventing.ProfileBinding{Stager: stager, PostCommit: wake}, Supervisor: supervisor,
			Drain: func(context.Context) error { return nil }, DrainTimeout: time.Second, Status: status}, nil
	}
	sqlStore, err := sdkmysql.New(db)
	if err != nil {
		return nil, nil, err
	}
	sqlStager, err := mysqlstandard.NewStager(opts.Catalog, eventruntime.SourceAPIServer)
	if err != nil {
		return nil, nil, err
	}
	sqlStatus, err := mysqlstandard.NewStatusReader(db)
	if err != nil {
		return nil, nil, err
	}
	profile, err := newProfile("assessment-mysql-outbox", sqlStore, sqlStager, appeventing.NamedOutboxStatusReader{Name: "assessment-mysql-outbox", Reader: sqlStatus})
	if err != nil {
		return nil, nil, err
	}
	profiles[eventcatalog.OutboxProfileAssessmentMySQL] = profile
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
	profile, err = newProfile("mongo-domain-events", mongoStore, mongoStager, appeventing.NamedOutboxStatusReader{Name: "mongo-domain-events", Reader: mongoStatus})
	if err != nil {
		return nil, nil, err
	}
	profiles[eventcatalog.OutboxProfileMongoDomain] = profile
	runtime, err := eventsubsystem.NewWithStandardProfiles(opts, profiles)
	return runtime, nil, err
}
