package subsystem

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	bridge "github.com/FangcunMount/qs-server/internal/apiserver/application/aibridge"
	mongoTx "github.com/FangcunMount/qs-server/internal/apiserver/infra/mongo/eventtransaction"
	mysqlTx "github.com/FangcunMount/qs-server/internal/apiserver/infra/mysql/eventtransaction"
	eventcatalog "github.com/FangcunMount/qs-server/internal/pkg/eventing/catalog"
	eventruntime "github.com/FangcunMount/qs-server/internal/pkg/eventing/runtime"
	"github.com/FangcunMount/qs-server/internal/pkg/messagingruntime"
	"github.com/FangcunMount/qs-server/internal/pkg/options"
	"github.com/FangcunMount/reliable-messaging/transport"
)

type transactionState struct {
	mysql    *mysqlTx.Committer
	mongo    *mongoTx.Owner
	runtimes []*messagingruntime.TransactionRuntime
}

func transactionTopics(catalog *eventcatalog.Catalog, kinds []string) ([]string, error) {
	seen := map[string]bool{}
	for _, kind := range kinds {
		topic, ok := catalog.GetTopicForEvent(kind)
		if !ok || topic == "" {
			return nil, fmt.Errorf("required transaction event route absent: %s", kind)
		}
		seen[topic] = true
	}
	topics := make([]string, 0, len(seen))
	for topic := range seen {
		topics = append(topics, topic)
	}
	sort.Strings(topics)
	return topics, nil
}

func registeredTransactionTopics(o *options.MessagingOptions, owner string, required []string) ([]string, error) {
	config, err := o.RocketMQ.Load(nil, 1, 5*time.Second)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, route := range config.Routes {
		if route.Owner == owner && route.MessageType == transport.TransactionMessageType && route.RouteRevision == o.RocketMQ.Transactions[owner].RouteRevision {
			seen[route.Topic] = true
		}
	}
	for _, topic := range required {
		if !seen[topic] {
			return nil, fmt.Errorf("required %s transaction topic absent: %s", owner, topic)
		}
	}
	topics := make([]string, 0, len(seen))
	for topic := range seen {
		topics = append(topics, topic)
	}
	sort.Strings(topics)
	return topics, nil
}

// NewWithTransactionOwners is inert. The process installs schema explicitly,
// then starts this subsystem and its one shared runtime per original store.
// No Outbox profile or Relay is created for the selected transaction mode.
func NewWithTransactionOwners(opts Options, o *options.MessagingOptions) (*Subsystem, error) {
	if o == nil || !o.Enabled || o.Provider != "rocketmq" || o.RocketMQ == nil || opts.Catalog == nil ||
		opts.MySQLDB == nil || opts.MongoDB == nil || opts.PublisherMode != eventruntime.PublishModeMQ || opts.WirePublisher == nil {
		return nil, errors.New("full original QS databases, explicit RocketMQ transaction configuration and NORMAL client required")
	}
	for _, owner := range []string{"qs.mysql", "qs.mongo"} {
		if _, ok := o.RocketMQ.Transactions[owner]; !ok {
			return nil, fmt.Errorf("required host transaction owner absent: %s", owner)
		}
	}
	mysqlTopics, err := transactionTopics(opts.Catalog, []string{eventcatalog.EvaluationRequested, eventcatalog.EvaluationRetryRequested,
		eventcatalog.EvaluationOutcomeCommitted, eventcatalog.EvaluationFailed, eventcatalog.TaskOpenedReminderRequested})
	if err != nil {
		return nil, err
	}
	mysqlTopics = append(mysqlTopics, bridge.CommandsTopic, bridge.AcksTopic)
	mysqlTopics, err = registeredTransactionTopics(o, "qs.mysql", mysqlTopics)
	if err != nil {
		return nil, err
	}
	mongoTopics, err := transactionTopics(opts.Catalog, []string{eventcatalog.AnswerSheetSubmitted, eventcatalog.InterpretationReportGenerated,
		eventcatalog.InterpretationReportFailed, eventcatalog.InterpretationRetryRequested})
	if err != nil {
		return nil, err
	}
	mongoTopics, err = registeredTransactionTopics(o, "qs.mongo", mongoTopics)
	if err != nil {
		return nil, err
	}
	mysqlOwner, err := mysqlTx.New(opts.MySQLDB, o, opts.Catalog, mysqlTopics)
	if err != nil {
		return nil, err
	}
	mongoOwner, err := mongoTx.New(opts.MongoDB, o, mongoTopics)
	if err != nil {
		return nil, err
	}
	s, err := newBase(opts)
	if err != nil {
		return nil, err
	}
	s.publisher, err = eventruntime.NewBestEffortPublisherWithOptions(eventruntime.RoutingPublisherOptions{
		Catalog: opts.Catalog, WirePublisher: opts.WirePublisher, Observer: opts.Observer, Source: eventruntime.SourceAPIServer})
	if err != nil {
		return nil, err
	}
	s.transactions = &transactionState{mysql: mysqlOwner, mongo: mongoOwner,
		runtimes: []*messagingruntime.TransactionRuntime{mysqlOwner.Runtime(), mongoOwner.Runtime()}}
	for _, runtime := range s.transactions.runtimes {
		if err = runtime.HoldStartupAdmission(); err != nil {
			return nil, err
		}
	}
	s.buildConsumers(opts.Consumers)
	return s, nil
}

func (s *Subsystem) HasTransactionOwners() bool { return s != nil && s.transactions != nil }
func (s *Subsystem) MySQLTransactionOwner() *mysqlTx.Committer {
	if !s.HasTransactionOwners() {
		return nil
	}
	return s.transactions.mysql
}
func (s *Subsystem) MongoTransactionOwner() *mongoTx.Owner {
	if !s.HasTransactionOwners() {
		return nil
	}
	return s.transactions.mongo
}

func (s *Subsystem) startTransactionOwners(ctx context.Context) error {
	if !s.HasTransactionOwners() {
		return nil
	}
	for _, runtime := range s.transactions.runtimes {
		if err := runtime.Start(ctx); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("transaction host closed during startup")
	}
	for _, runtime := range s.transactions.runtimes {
		if err := runtime.ReleaseStartupAdmission(); err != nil {
			return err
		}
	}
	return nil
}

// Caller holds s.mu. Pausing the host gate does not wait for SDK I/O and keeps
// Checker running. The owned closer later seals/drains the underlying senders.
func (s *Subsystem) pauseTransactionAdmission() {
	if !s.HasTransactionOwners() {
		return
	}
	for _, runtime := range s.transactions.runtimes {
		_ = runtime.SetAdmission(false)
	}
}

func (s *Subsystem) closeTransactionOwners() error {
	if !s.HasTransactionOwners() {
		return nil
	}
	var failures []error
	for i := len(s.transactions.runtimes) - 1; i >= 0; i-- {
		// This single owned coordinator retains dependencies until real work
		// completes. CloseContext/Wait bound each caller's wait independently.
		if err := s.transactions.runtimes[i].Close(context.Background()); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

// StopAdmission closes only the host write gate. Checkers and owned clients
// remain available while original business consumers finish their work.
func (s *Subsystem) StopAdmission() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pauseTransactionAdmission()
}
