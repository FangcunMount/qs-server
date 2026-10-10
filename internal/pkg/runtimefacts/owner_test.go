package runtimefacts

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func testOwner(t *testing.T) *Owner {
	return testOwnerForComponent(t, "worker")
}

func testOwnerForComponent(t *testing.T, component string) *Owner {
	t.Helper()
	o, err := New(component, strings.Repeat("a", 40))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = o.Close() })
	return o
}

func testTransport(o *Owner, id, direction string) Transport {
	return Transport{ID: id, Provider: "nsq", Direction: direction, NSQDTCPAddresses: []string{"nsqd.internal:4150"}, NSQDHTTPAddresses: []string{"http://nsqd.internal:4151"}, NSQDPairs: []NSQDPair{{TCPAddress: "nsqd.internal:4150", HTTPAddress: "http://nsqd.internal:4151"}}, ClientID: o.ClientID(id), Hostname: o.Hostname()}
}

func TestProcessScopedClientIDs(t *testing.T) {
	first, second := testOwner(t), testOwner(t)
	firstID := first.ClientID("outbox")
	if firstID != first.ClientID("outbox") {
		t.Fatal("scope identity changed within process")
	}
	if first.ClientID("outbox") == first.ClientID("consumer") || first.ClientID("outbox") == second.ClientID("outbox") {
		t.Fatal("scope or process identity reused")
	}
	if first.nonce == second.nonce || !validHex(first.nonce, 64) {
		t.Fatal("nonce missing or reused")
	}
}

func TestUnboundDevelopmentSourceCannotQualify(t *testing.T) {
	for _, sha := range []string{"", "unknown", strings.Repeat("A", 40), strings.Repeat("0", 40), "secret-token"} {
		t.Run(sha, func(t *testing.T) {
			o, err := New("apiserver", sha)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = o.Close() }()
			if o.ClientID("publisher") == "" {
				t.Fatal("development identity unavailable")
			}
			s := o.Snapshot()
			if s.ObservationComplete || s.SourceSHA != "" || !contains(s.IncompleteReasons, "source_unbound") {
				t.Fatal("unbound source qualified")
			}
		})
	}
}

func TestTransportProjectionCopiesLoadedFields(t *testing.T) {
	o := testOwner(t)
	loaded := testTransport(o, "publisher", "publisher")
	loaded.LookupdAddresses = []string{"lookupd.internal:4161"}
	loaded.PublishTopics = []string{"topic.z", "topic.a"}
	if err := o.Declare(loaded); err != nil {
		t.Fatal(err)
	}
	if err := o.MarkStarted(loaded.ID); err != nil {
		t.Fatal(err)
	}
	loaded.LookupdAddresses[0] = "mutated:1"
	loaded.NSQDPairs[0].TCPAddress = "mutated:1"
	loaded.PublishTopics[0] = "mutated"
	first := o.Snapshot()
	if first.ObservationComplete || first.BrokerConnectionsVerified || first.Transports[0].State != "started" {
		t.Fatal("projection claimed broker or private-channel authority")
	}
	if first.Transports[0].LookupdAddresses[0] != "lookupd.internal:4161" || first.Transports[0].NSQDPairs[0].TCPAddress != "nsqd.internal:4150" || !reflect.DeepEqual(first.Transports[0].PublishTopics, []string{"topic.a", "topic.z"}) {
		t.Fatal("loaded configuration mutated or inferred")
	}
	first.Transports[0].NSQDTCPAddresses[0] = "mutated:1"
	if o.Snapshot().Transports[0].NSQDTCPAddresses[0] != "nsqd.internal:4150" {
		t.Fatal("snapshot aliases owner's configuration")
	}
}

func TestConsumerActualSubscriptionAndTerminalTransitions(t *testing.T) {
	for _, terminal := range []string{"stopped", "incomplete"} {
		t.Run(terminal, func(t *testing.T) {
			o := testOwner(t)
			transport := testTransport(o, "event-consumer", "consumer")
			if err := o.Declare(transport); err != nil {
				t.Fatal(err)
			}
			if err := o.MarkStarted(transport.ID); err == nil {
				t.Fatal("consumer started without subscription")
			}
			subscription := Subscription{Topic: "answersheet.submitted", Channel: "worker", FailureTopic: "answersheet.submitted.failed", FailureChannel: "worker-failure"}
			if err := o.MarkSubscribed(transport.ID, subscription); err != nil {
				t.Fatal(err)
			}
			if err := o.MarkStarted(transport.ID); err != nil {
				t.Fatal(err)
			}
			if got := o.Snapshot().Transports[0]; !reflect.DeepEqual(got.Subscriptions, []Subscription{subscription}) || got.State != "started" {
				t.Fatal("successful subscription not preserved")
			}
			if terminal == "stopped" {
				o.MarkStopped(transport.ID)
			} else {
				o.MarkIncomplete(transport.ID)
			}
			if o.MarkStarted(transport.ID) == nil || o.MarkSubscribed(transport.ID, Subscription{Topic: "another", Channel: "worker"}) == nil {
				t.Fatal("terminal transport restarted")
			}
			if got := o.Snapshot(); got.ObservationComplete || got.Transports[0].State != terminal {
				t.Fatal("terminal projection qualified or changed")
			}
		})
	}
}

func TestNoLocalMQRequiresExplicitExclusiveRole(t *testing.T) {
	o := testOwnerForComponent(t, "collection-server")
	if !contains(o.Snapshot().IncompleteReasons, "message_role_unregistered") {
		t.Fatal("empty registration treated as full coverage")
	}
	if err := o.NoLocalMQ(); err != nil {
		t.Fatal(err)
	}
	if got := o.Snapshot().Transports; len(got) != 1 || got[0].Direction != "no_local_mq" || got[0].Provider != "none" || got[0].State != "no_local_mq" || len(got[0].Subscriptions) != 0 {
		t.Fatal("no-local-MQ role not explicit")
	}
	if o.Declare(testTransport(o, "consumer", "consumer")) == nil {
		t.Fatal("no-local-MQ role mixed with consumer")
	}
	second := testOwnerForComponent(t, "collection-server")
	if second.Declare(testTransport(second, "publisher", "publisher")) != nil || second.NoLocalMQ() == nil {
		t.Fatal("publisher mixed with no-local-MQ role")
	}
}

func TestNoLocalMQRejectsWrongComponentWithoutEmptyCoverage(t *testing.T) {
	for _, component := range []string{"worker", "apiserver", "collection", "unknown"} {
		t.Run(component, func(t *testing.T) {
			o := testOwnerForComponent(t, component)
			if o.NoLocalMQ() == nil {
				t.Fatal("wrong component claimed no local MQ")
			}
			if o.Declare(Transport{ID: "manual-none", Provider: "none", Direction: "no_local_mq"}) == nil {
				t.Fatal("manual declaration bypassed component role")
			}
			got := o.Snapshot()
			if got.ObservationComplete || len(got.Transports) != 0 || !contains(got.IncompleteReasons, "no_local_mq_role_invalid") || !contains(got.IncompleteReasons, "message_role_unregistered") {
				t.Fatal("wrong role rejection was not sticky or empty role qualified")
			}
			transport := testTransport(o, "publisher", "publisher")
			if o.Declare(transport) != nil || o.MarkStarted(transport.ID) != nil {
				t.Fatal("actual transport could not be registered")
			}
			if got = o.Snapshot(); got.ObservationComplete || !contains(got.IncompleteReasons, "no_local_mq_role_invalid") {
				t.Fatal("later actual role erased invalid role conclusion")
			}
		})
	}
}

func TestLoadedConfigRejectsSecretsInferenceAndIdentityReuse(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Transport)
	}{
		{"credentials", func(t *Transport) { t.LookupdAddresses = []string{"http://user:secret@host:4161"} }},
		{"query", func(t *Transport) { t.NSQDHTTPAddresses = []string{"http://host:4151?token=secret"} }},
		{"path", func(t *Transport) { t.NSQDHTTPAddresses = []string{"http://host:4151/stats"} }},
		{"tcp-scheme", func(t *Transport) { t.NSQDTCPAddresses = []string{"tcp://host:4150"} }},
		{"address-missing", func(t *Transport) { t.NSQDTCPAddresses = nil }},
		{"pair-inferred", func(t *Transport) { t.NSQDPairs[0].HTTPAddress = "http://another:4151" }},
		{"unbound-client", func(t *Transport) { t.ClientID = "shared-client" }},
		{"different-host", func(t *Transport) { t.Hostname = "foreign-host" }},
		{"duplicate-address", func(t *Transport) { t.NSQDTCPAddresses = append(t.NSQDTCPAddresses, t.NSQDTCPAddresses[0]) }},
		{"consumer-publish-topics", func(t *Transport) { t.Direction = "consumer"; t.PublishTopics = []string{"topic"} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			o := testOwner(t)
			transport := testTransport(o, "publisher", "publisher")
			test.change(&transport)
			if o.Declare(transport) == nil {
				t.Fatal("invalid transport registered")
			}
			if len(o.Snapshot().Transports) != 0 {
				t.Fatal("invalid source projected")
			}
		})
	}
	o := testOwner(t)
	transport := testTransport(o, "publisher", "publisher")
	if o.Declare(transport) != nil {
		t.Fatal("valid transport rejected")
	}
	transport.ID = "another"
	if o.Declare(transport) == nil {
		t.Fatal("process transport identity reused")
	}
}

func TestUnknownTransitionCannotBeHidden(t *testing.T) {
	o := testOwner(t)
	o.MarkIncomplete("not-registered")
	if !contains(o.Snapshot().IncompleteReasons, "unregistered_transport_transition") {
		t.Fatal("missing registration hidden")
	}
}

func TestSnapshotFixedProjectionHasNoAuthorityFields(t *testing.T) {
	o := testOwnerForComponent(t, "collection-server")
	if err := o.NoLocalMQ(); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(o.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	var projection map[string]json.RawMessage
	if json.Unmarshal(raw, &projection) != nil {
		t.Fatal("invalid snapshot JSON")
	}
	expected := []string{"format_version", "component", "source_sha", "process_nonce", "hostname", "process", "observed_at", "lifecycle", "observation_complete", "incomplete_reasons", "broker_connections_verified", "transports"}
	if len(projection) != len(expected) {
		t.Fatal("unexpected projection fields")
	}
	for _, key := range expected {
		if _, ok := projection[key]; !ok {
			t.Fatal("missing projection field", key)
		}
	}
	for _, forbidden := range []string{"password", "dsn", "environment", "fence_verified", "writers_stopped", "drop_authorized", "execution_ready"} {
		if strings.Contains(string(raw), "\""+forbidden+"\"") {
			t.Fatal("unexpected authority or secret field")
		}
	}
}

func TestCloseStopsOwnerAndCannotRestart(t *testing.T) {
	o := testOwner(t)
	transport := testTransport(o, "publisher", "publisher")
	if o.Declare(transport) != nil || o.MarkStarted(transport.ID) != nil {
		t.Fatal("registration failed")
	}
	if o.Close() != nil {
		t.Fatal("close failed")
	}
	if o.Close() != nil {
		t.Fatal("idempotent close failed")
	}
	if o.Start() == nil || o.MarkStarted(transport.ID) == nil || o.Declare(testTransport(o, "next", "publisher")) == nil {
		t.Fatal("closed owner reused")
	}
	if got := o.Snapshot(); got.ObservationComplete || got.Lifecycle != "stopped" || got.Transports[0].State != "stopped" {
		t.Fatal("closed owner reported active")
	}
}

func TestConcurrentProjectionAndTransitions(t *testing.T) {
	o := testOwner(t)
	transport := testTransport(o, "publisher", "publisher")
	if o.Declare(transport) != nil {
		t.Fatal("registration failed")
	}
	var group sync.WaitGroup
	for n := 0; n < 8; n++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for i := 0; i < 32; i++ {
				_ = o.ClientID("scope")
				_ = o.Snapshot()
				_ = o.MarkStarted(transport.ID)
			}
		}()
	}
	group.Wait()
	o.MarkIncomplete(transport.ID)
	if o.Snapshot().ObservationComplete {
		t.Fatal("incomplete observation qualified")
	}
}
